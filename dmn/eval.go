package dmn

import (
	"fmt"

	"github.com/pblumer/temis/internal/boxed"
	"github.com/pblumer/temis/internal/value"
)

// evaluator runs a decision graph for one Evaluate call: it memoises each
// decision's result (so a decision reached by several paths runs once), guards
// against dependency cycles, and records every decision it evaluates. It is not
// safe for concurrent use; one is created per call.
type evaluator struct {
	base      map[string]value.Value // input data and caller-supplied results
	cache     map[*CompiledDecision]value.Value
	visiting  map[*CompiledDecision]bool
	decisions map[string]any  // evaluated decisions, by name, for the Result
	boundary  map[string]bool // decision names the evaluator must not compute (service inputs)
	// rec, when set, collects a decision trace shared across the whole graph, so
	// every decision table the evaluation touches records into one explanation
	// (WP-51). nil disables tracing.
	rec *boxed.Recorder
}

func newEvaluator(base map[string]value.Value) *evaluator {
	return &evaluator{
		base:      base,
		cache:     make(map[*CompiledDecision]value.Value),
		visiting:  make(map[*CompiledDecision]bool),
		decisions: map[string]any{},
	}
}

// eval returns d's result, evaluating its required decisions first and feeding
// them in by name. A required value present in the input is used as-is rather
// than recomputed, which both bounds a decision service at its inputs and lets a
// caller short-circuit a sub-decision.
func (e *evaluator) eval(d *CompiledDecision) (value.Value, error) {
	if v, ok := e.cache[d]; ok {
		return v, nil
	}
	if e.visiting[d] {
		return nil, fmt.Errorf("dmn: dependency cycle at decision %q", label(d))
	}
	if d.expr == nil {
		return nil, fmt.Errorf("dmn: required decision %q has no executable logic", label(d))
	}
	e.visiting[d] = true

	vals := make(map[string]value.Value, len(e.base)+len(d.requires))
	for k, v := range e.base {
		vals[k] = v
	}
	for _, req := range d.requires {
		if v, ok := e.base[req.name]; ok && req.name != "" {
			vals[req.name] = v
			continue
		}
		// A service input decision is a boundary: it is supplied, never computed,
		// so an unsupplied one is null rather than evaluated.
		if e.boundary[req.name] && req.name != "" {
			vals[req.name] = value.Null
			continue
		}
		rv, err := e.eval(req)
		if err != nil {
			return nil, err
		}
		vals[req.name] = rv
	}

	scope := d.env.NewScope(vals)
	if e.rec != nil {
		scope = scope.WithTrace(e.rec)
	}
	out, err := d.expr(scope)
	if err != nil {
		return nil, fmt.Errorf("dmn: evaluate decision %q: %w", d.name, err)
	}
	e.visiting[d] = false
	e.cache[d] = out
	if d.name != "" {
		e.decisions[d.name] = fromValue(out)
	}
	return out, nil
}
