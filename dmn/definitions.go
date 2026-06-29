package dmn

import (
	"fmt"

	"github.com/pblumer/temis/internal/feel"
	"github.com/pblumer/temis/internal/model"
)

// Definitions is a compiled DMN model: the set of decisions a document declares,
// each ready to evaluate. It is immutable after Compile and safe to share.
type Definitions struct {
	model         *model.Definitions
	byID          map[string]*CompiledDecision
	byName        map[string]*CompiledDecision
	order         []*CompiledDecision
	servicesByID  map[string]*CompiledService
	servicesByNam map[string]*CompiledService
	serviceOrder  []*CompiledService
}

// CompiledDecision is a single decision's compiled logic. It is immutable and
// thread-safe, so one instance may be evaluated concurrently any number of
// times against different inputs.
type CompiledDecision struct {
	id, name string
	env      *feel.Env
	expr     feel.CompiledExpr // nil when the decision has no executable logic
	inputs   []InputField      // declared input schema, for self-description and validation
	// constraints holds the resolved structural type and allowed-values matcher
	// per input name, enforced by strict validation (WP-31).
	constraints map[string]*inputConstraint
	// requires are the decisions this one consumes directly; the evaluator runs
	// them first and feeds their results in by name (WP-28). Resolved after all
	// decisions compile.
	requires []*CompiledDecision
	// reqInputs are the resolved names of the decision's required input data
	// (input data only, not required decisions). Evaluate fails hard when any is
	// absent from the supplied Input.
	reqInputs []string
	// limits are the resource bounds enforced for an evaluation rooted at this
	// decision (WP-34), resolved from the engine configuration at compile time.
	limits feel.Limits
}

// Decision returns the compiled decision identified by idOrName. It is an error
// if no such decision exists, or if it exists but has no executable logic.
func (d *Definitions) Decision(idOrName string) (*CompiledDecision, error) {
	cd, ok := d.byID[idOrName]
	if !ok {
		cd, ok = d.byName[idOrName]
	}
	if !ok {
		return nil, fmt.Errorf("dmn: no decision %q", idOrName)
	}
	if cd.expr == nil {
		return nil, fmt.Errorf("dmn: decision %q has no executable logic", idOrName)
	}
	return cd, nil
}

// ModelName returns the DMN definitions' name (the editable model name), or ""
// when the document declares none.
func (d *Definitions) ModelName() string { return d.model.Name }

// Name returns the decision's name.
func (c *CompiledDecision) Name() string { return c.name }

// ID returns the decision's identifier.
func (c *CompiledDecision) ID() string { return c.id }

// ModelIndex lists a model's evaluable decisions and its input data, by name,
// for tooling and discovery.
type ModelIndex struct {
	Decisions []string
	Inputs    []string
}

// Index returns the names of the model's decisions and input data. Only
// decisions with executable logic are listed.
func (d *Definitions) Index() ModelIndex {
	idx := ModelIndex{}
	for _, cd := range d.order {
		if cd.expr != nil && cd.name != "" {
			idx.Decisions = append(idx.Decisions, cd.name)
		}
	}
	for _, in := range d.model.InputData {
		if in.Name != "" {
			idx.Inputs = append(idx.Inputs, in.Name)
		}
	}
	return idx
}
