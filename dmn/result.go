package dmn

import (
	"context"
	"fmt"

	"github.com/pblumer/temis/internal/boxed"
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
	// Decisions holds every decision evaluated to produce the result. Until DRG
	// chaining (WP-28) this mirrors Outputs.
	Decisions map[string]any
	// Diags holds runtime diagnostics (e.g. a null produced by a recoverable
	// error). Spec-conformant null results are not errors.
	Diags Diagnostics
	// Trace is the structured explanation of this evaluation, present only when
	// the call requested it via WithTrace; nil otherwise.
	Trace *Trace
}

// EvalOption tunes a single Evaluate call.
type EvalOption func(*evalConfig)

type evalConfig struct {
	trace  bool
	strict bool
}

// WithTrace makes Evaluate attach a structured explanation (which rules matched
// and why) to Result.Trace. It is opt-in: without it, evaluation takes the
// allocation-free path and Result.Trace stays nil (ADR-0013, WP-51).
func WithTrace() EvalOption {
	return func(c *evalConfig) { c.trace = true }
}

// WithStrictInput makes Evaluate validate the input against the decision's
// declared schema first and fail with an *InputError if it does not conform —
// instead of silently coercing a wrong-typed or misnamed value into a null or a
// non-match (ADR-0013, WP-52). Without it, evaluation is lenient as before.
func WithStrictInput() EvalOption {
	return func(c *evalConfig) { c.strict = true }
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
func (c *CompiledDecision) Evaluate(ctx context.Context, in Input, opts ...EvalOption) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if c.expr == nil {
		return Result{}, fmt.Errorf("dmn: decision %q has no executable logic", c.name)
	}

	var cfg evalConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	if cfg.strict {
		if probs := c.ValidateInput(in); len(probs) > 0 {
			return Result{}, &InputError{Problems: probs}
		}
	}

	vals, err := inputToValues(in)
	if err != nil {
		return Result{}, err
	}

	scope := c.env.NewScope(vals)
	var rec *boxed.Recorder
	if cfg.trace {
		rec = boxed.NewRecorder()
		scope = scope.WithTrace(rec)
	}

	out, err := c.expr(scope)
	if err != nil {
		return Result{}, fmt.Errorf("dmn: evaluate decision %q: %w", c.name, err)
	}

	result := fromValue(out)
	res := Result{
		Outputs:   map[string]any{c.name: result},
		Decisions: map[string]any{c.name: result},
	}
	if cfg.trace {
		res.Trace = traceFromRecorder(rec)
	}
	return res, nil
}
