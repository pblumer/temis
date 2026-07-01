package flow

import (
	"fmt"
	"strings"

	"github.com/pblumer/temis/dmn"
)

// mappingKind classifies how a step-input or output wiring is resolved.
type mappingKind int

const (
	// mStep is a plain reference to an earlier step's output, "stepID.output".
	mStep mappingKind = iota
	// mInput is a plain reference to a flow input by name.
	mInput
	// mFeel is a full FEEL expression over the flow inputs and step outputs.
	mFeel
)

// mapping is a compiled wiring. Plain references (mStep, mInput) are resolved
// directly and stay backward-compatible with the reference-only form (WP-90); a
// mFeel mapping is an arbitrary FEEL expression (ADR-0026 / full-FEEL mappings),
// compiled once against the flow's inputs and step ids.
type mapping struct {
	raw    string
	kind   mappingKind
	stepID string                  // mStep
	key    string                  // mStep
	name   string                  // mInput
	expr   *dmn.CompiledExpression // mFeel
	deps   []string                // mFeel: the step ids the expression references
}

// stepDeps returns the step ids this mapping depends on (for ordering).
func (m mapping) stepDeps() []string {
	switch m.kind {
	case mStep:
		return []string{m.stepID}
	case mFeel:
		return m.deps
	default:
		return nil
	}
}

// isStepID reports whether name is a valid step id in this flow.
func (f *Flow) isStepID(name string) bool {
	_, ok := f.stepIdx[name]
	return ok
}

// classify compiles one mapping string into a mapping, returning any diagnostics.
// A mapping is resolved directly when it is a plain reference — an earlier step's
// "stepID.output", or exactly a declared flow-input name — which keeps the common,
// reference-only case allocation-light and backward-compatible (WP-90). Anything
// else is a full FEEL expression compiled against {declared inputs} ∪ {step ids};
// referencing a name that is neither is a compile error (FLOW_MAPPING_INVALID),
// so declaring an input is required to use it in an expression.
func (f *Flow) classify(step, where, ref string) (mapping, Diagnostics) {
	ref = strings.TrimSpace(ref)
	if stepID, key, ok := f.parseStepRef(ref); ok {
		return mapping{raw: ref, kind: mStep, stepID: stepID, key: key}, nil
	}
	if _, ok := f.inputs[ref]; ok {
		return mapping{raw: ref, kind: mInput, name: ref}, nil
	}
	ce, err := dmn.CompileExpression(ref, f.feelNames...)
	if err != nil {
		return mapping{raw: ref, kind: mFeel}, Diagnostics{{Code: CodeMappingInvalid, Step: step, Message: fmt.Sprintf("%s: invalid FEEL mapping %q: %v", where, ref, err)}}
	}
	var deps []string
	for _, r := range ce.References() {
		if f.isStepID(r) {
			deps = append(deps, r)
		}
	}
	return mapping{raw: ref, kind: mFeel, expr: ce, deps: deps}, nil
}
