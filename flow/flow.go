package flow

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Flow is a parsed, structurally-validated decision flow. It is immutable after
// Compile and safe to evaluate concurrently (evaluation holds no state on the
// Flow). Model-aware validation happens in Validate/Evaluate, which need a
// Resolver.
type Flow struct {
	desc    Descriptor
	stepIdx map[string]int       // step id → index into desc.Steps (valid steps only)
	order   []int                // topological evaluation order (indices into desc.Steps)
	inputs  map[string]InputDecl // declared flow inputs, by name
	diags   Diagnostics          // structural diagnostics from Compile

	// feelNames is the variable namespace a FEEL mapping compiles against: every
	// declared flow input plus every step id.
	feelNames []string
	// compiledIn holds each step's compiled input wirings, keyed by step index
	// then target input name. compiledOut holds the flow's output wirings by name.
	compiledIn  map[int]map[string]mapping
	compiledOut map[string]mapping
}

// Name returns the flow's declared identifier.
func (f *Flow) Name() string { return f.desc.Flow }

// Diagnostics returns the structural diagnostics found at Compile time (bad
// shape, duplicate/cyclic steps, unresolved references). Model-aware problems
// are reported by Validate.
func (f *Flow) Diagnostics() Diagnostics { return f.diags }

// Compile parses a JSON flow descriptor and runs structural validation that does
// not need the models: shape, unique/complete steps, reference well-formedness
// and acyclicity. Malformed JSON is an error (no Flow); structural faults are
// returned as diagnostics on an inspectable Flow, and Evaluate refuses to run a
// flow that carries them.
func Compile(data []byte) (*Flow, Diagnostics, error) {
	var d Descriptor
	if err := json.Unmarshal(data, &d); err != nil {
		diags := Diagnostics{{Code: CodeMalformed, Message: "cannot parse flow descriptor: " + err.Error()}}
		return nil, diags, fmt.Errorf("flow: %w", err)
	}

	f := &Flow{desc: d, stepIdx: make(map[string]int), inputs: make(map[string]InputDecl)}
	for _, in := range d.Inputs {
		if in.Name != "" {
			f.inputs[in.Name] = in
		}
	}

	var diags Diagnostics
	if len(d.Steps) == 0 {
		diags = append(diags, Diagnostic{Code: CodeNoSteps, Message: "flow has no steps"})
	}
	for i := range d.Steps {
		s := d.Steps[i]
		if s.ID == "" {
			diags = append(diags, Diagnostic{Code: CodeMissingField, Message: fmt.Sprintf("step at index %d has no id", i)})
			continue
		}
		if strings.Contains(s.ID, ".") {
			diags = append(diags, Diagnostic{Code: CodeMissingField, Step: s.ID, Message: "step id must not contain '.'"})
			continue
		}
		if _, dup := f.stepIdx[s.ID]; dup {
			diags = append(diags, Diagnostic{Code: CodeDuplicateStep, Step: s.ID, Message: "duplicate step id"})
			continue
		}
		f.stepIdx[s.ID] = i
		if s.Model == "" {
			diags = append(diags, Diagnostic{Code: CodeMissingField, Step: s.ID, Message: "step has no model"})
		}
		if s.Decision == "" {
			diags = append(diags, Diagnostic{Code: CodeMissingField, Step: s.ID, Message: "step has no decision"})
		}
	}

	// The FEEL namespace is every declared input plus every step id (sorted for a
	// deterministic slot layout). Mappings are then compiled against it.
	f.feelNames = make([]string, 0, len(f.inputs)+len(f.stepIdx))
	for name := range f.inputs {
		f.feelNames = append(f.feelNames, name)
	}
	f.feelNames = append(f.feelNames, f.sortedIDs()...)
	sort.Strings(f.feelNames)

	diags = append(diags, f.compileMappings()...)

	order, cyclic := f.topoOrder()
	if cyclic {
		diags = append(diags, Diagnostic{Code: CodeCycle, Message: "flow steps form a reference cycle"})
	}
	f.order = order
	f.diags = diags
	return f, diags, nil
}

// compileMappings classifies and compiles every step-input and output wiring,
// storing the results and returning any diagnostics (bad references, FEEL
// expressions that do not compile).
func (f *Flow) compileMappings() Diagnostics {
	var diags Diagnostics
	f.compiledIn = make(map[int]map[string]mapping, len(f.stepIdx))
	for _, id := range f.sortedIDs() {
		idx := f.stepIdx[id]
		s := f.desc.Steps[idx]
		if len(s.In) == 0 {
			continue
		}
		m := make(map[string]mapping, len(s.In))
		for target, ref := range s.In {
			cm, ds := f.classify(id, "input "+strconv.Quote(target), ref)
			diags = append(diags, ds...)
			m[target] = cm
		}
		f.compiledIn[idx] = m
	}
	f.compiledOut = make(map[string]mapping, len(f.desc.Output))
	for name, ref := range f.desc.Output {
		cm, ds := f.classify("", "output "+strconv.Quote(name), ref)
		diags = append(diags, ds...)
		f.compiledOut[name] = cm
	}
	return diags
}

// topoOrder returns a deterministic topological order of the valid steps and
// whether the dependency graph is cyclic (Kahn's algorithm; ties broken by id).
// A step depends on every step its input mappings reference — whether by a plain
// "stepID.output" reference or by naming the step inside a FEEL expression.
func (f *Flow) topoOrder() (order []int, cyclic bool) {
	ids := f.sortedIDs()
	indeg := make(map[string]int, len(ids))
	adj := make(map[string][]string, len(ids))
	for _, id := range ids {
		indeg[id] = 0
	}
	for _, id := range ids {
		idx := f.stepIdx[id]
		seen := make(map[string]bool)
		for _, m := range f.compiledIn[idx] {
			for _, dep := range m.stepDeps() {
				if dep == id || seen[dep] {
					continue
				}
				seen[dep] = true
				adj[dep] = append(adj[dep], id) // dep must run before id
				indeg[id]++
			}
		}
	}

	var queue []string
	for _, id := range ids {
		if indeg[id] == 0 {
			queue = append(queue, id)
		}
	}
	for len(queue) > 0 {
		sort.Strings(queue)
		id := queue[0]
		queue = queue[1:]
		order = append(order, f.stepIdx[id])
		for _, nb := range adj[id] {
			indeg[nb]--
			if indeg[nb] == 0 {
				queue = append(queue, nb)
			}
		}
	}
	return order, len(order) != len(ids)
}

// parseStepRef splits "stepID.key" into its parts, reporting ok only when the
// prefix names a known step. A reference without a '.' — or whose prefix is not a
// step id — is a flow-input reference, not a step reference.
func (f *Flow) parseStepRef(ref string) (stepID, key string, ok bool) {
	if i := strings.IndexByte(ref, '.'); i >= 0 {
		if _, isStep := f.stepIdx[ref[:i]]; isStep {
			return ref[:i], ref[i+1:], true
		}
	}
	return "", "", false
}

// sortedIDs returns the valid step ids in deterministic order.
func (f *Flow) sortedIDs() []string {
	ids := make([]string, 0, len(f.stepIdx))
	for id := range f.stepIdx {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
