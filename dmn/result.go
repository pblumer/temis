package dmn

import (
	"context"
	"fmt"

	"github.com/pblumer/temis/internal/value"
)

// Input is an evaluation context: variable name → Go value. Keys are input-data
// or required-decision names; values are converted to FEEL values per the
// mapping documented on Evaluate. Names the model does not reference are
// ignored; referenced names absent from the map evaluate to FEEL null.
type Input map[string]any

// Result is the outcome of evaluating a decision.
type Result struct {
	// Outputs holds the requested decision's result, keyed by decision name.
	Outputs map[string]any
	// Decisions holds every decision evaluated to produce the result, keyed by
	// name: the requested decision plus each required decision the evaluator ran
	// for it (WP-28). A required value supplied directly in the input is used as
	// given and is not re-evaluated, so it does not appear here.
	Decisions map[string]any
	// Diags holds runtime diagnostics (e.g. a null produced by a recoverable
	// error). Spec-conformant null results are not errors.
	Diags Diagnostics
}

// Evaluate runs the decision against in and returns its result. Compilation has
// already happened, so this is the cheap, repeatable phase.
//
// Go inputs convert to FEEL values as follows: nil→null, bool→boolean, the
// integer and floating-point kinds→number (decimal; float inputs may lose
// precision — prefer string or integer for exact amounts), string→string,
// time.Time→date and time, []any→list, map[string]any→context. A value already
// of the engine's internal value type is passed through.
//
// FEEL results convert back to Go with numbers rendered as their exact decimal
// string (ADR-0007), booleans as bool, strings as string, temporal values and
// ranges as their canonical FEEL string, lists as []any and contexts as
// map[string]any. A spec-conformant null becomes nil and is not an error; only
// genuine runtime failures (a context cancellation, an exhausted limit, a
// UNIQUE table with multiple matches) return a non-nil error.
func (c *CompiledDecision) Evaluate(ctx context.Context, in Input) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if c.expr == nil {
		return Result{}, fmt.Errorf("dmn: decision %q has no executable logic", c.name)
	}

	base, err := inputToValues(in)
	if err != nil {
		return Result{}, err
	}

	decisions := map[string]any{}
	cache := make(map[*CompiledDecision]value.Value)
	visiting := make(map[*CompiledDecision]bool)

	var eval func(d *CompiledDecision) (value.Value, error)
	eval = func(d *CompiledDecision) (value.Value, error) {
		if v, ok := cache[d]; ok {
			return v, nil
		}
		if visiting[d] {
			return nil, fmt.Errorf("dmn: dependency cycle at decision %q", label(d))
		}
		if d.expr == nil {
			return nil, fmt.Errorf("dmn: required decision %q has no executable logic", label(d))
		}
		visiting[d] = true

		// The decision evaluates against the input data plus its required
		// decisions' results, injected by name. A required value the caller
		// supplied directly is honoured as-is rather than recomputed.
		vals := make(map[string]value.Value, len(base)+len(d.requires))
		for k, v := range base {
			vals[k] = v
		}
		for _, req := range d.requires {
			if v, ok := base[req.name]; ok && req.name != "" {
				vals[req.name] = v
				continue
			}
			rv, err := eval(req)
			if err != nil {
				return nil, err
			}
			vals[req.name] = rv
		}

		out, err := d.expr(d.env.NewScope(vals))
		if err != nil {
			return nil, fmt.Errorf("dmn: evaluate decision %q: %w", d.name, err)
		}
		visiting[d] = false
		cache[d] = out
		if d.name != "" {
			decisions[d.name] = fromValue(out)
		}
		return out, nil
	}

	out, err := eval(c)
	if err != nil {
		return Result{}, err
	}

	return Result{
		Outputs:   map[string]any{c.name: fromValue(out)},
		Decisions: decisions,
	}, nil
}
