package dmn

import (
	"context"
	"errors"
	"strconv"

	"github.com/pblumer/temis/internal/boxed"
)

// Input is an evaluation context: variable name → Go value. Keys are input-data
// or required-decision names; values are converted to FEEL values per the
// mapping documented on Evaluate. Names the model does not reference are
// ignored. A referenced required input data name that is absent is a caller
// error (Evaluate returns an EvalError with CodeMissingInput, not a silent
// null); other referenced names absent from the map evaluate to FEEL null.
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
// map[string]any.
//
// Evaluate is hard (fail-fast): it returns a non-nil error — always an
// *EvalError, classifiable via its Code — in exactly these cases:
//
//   - CodeNotExecutable: the decision did not compile to executable logic.
//     The caller is expected to check diags.HasErrors() after Compile; reaching
//     here is a caller bug, not a data case, and is not masked as a null.
//   - CodeMissingInput: a required input data value the model references is
//     absent from in. Also a caller bug; not masked as a null.
//   - CodeUniqueMultiple: a UNIQUE hit-policy table matched more than one rule
//     (classified from a typed cause in internal/boxed).
//   - CodeRuntime: the context was cancelled or its deadline passed, or the
//     expression failed at runtime in a way not yet exposed as a typed cause.
//     CodeLimitExceeded is reserved for the resource-limit path (ADR-0008): once
//     limits are wired and their breach is typed, those failures move from
//     CodeRuntime to CodeLimitExceeded.
//
// A spec-conformant FEEL null (a runtime type mismatch, division by zero, …) is
// NOT an error: it becomes a nil output in Result, optionally with a
// warning/info diagnostic in Result.Diags.
func (c *CompiledDecision) Evaluate(ctx context.Context, in Input) (Result, error) {
	if err := ctx.Err(); err != nil {
		// Cancellation/deadline; deliberately CodeRuntime, not a limit breach.
		return Result{}, &EvalError{
			Code:       CodeRuntime,
			DecisionID: c.id,
			Message:    "context error before evaluation",
			Err:        err,
		}
	}
	if c.expr == nil {
		return Result{}, &EvalError{
			Code:       CodeNotExecutable,
			DecisionID: c.id,
			Message:    "decision has no executable logic; check Compile diagnostics (diags.HasErrors()) before evaluating",
		}
	}

	for _, name := range c.reqInputs {
		if _, ok := in[name]; !ok {
			return Result{}, &EvalError{
				Code:       CodeMissingInput,
				DecisionID: c.id,
				Message:    "missing required input " + strconv.Quote(name),
			}
		}
	}

	vals, err := inputToValues(in)
	if err != nil {
		return Result{}, &EvalError{
			Code:       CodeRuntime,
			DecisionID: c.id,
			Message:    "converting inputs",
			Err:        err,
		}
	}

	out, err := c.expr(c.env.NewScope(vals))
	if err != nil {
		return Result{}, c.classifyRuntime(err)
	}

	result := fromValue(out)
	return Result{
		Outputs:   map[string]any{c.name: result},
		Decisions: map[string]any{c.name: result},
	}, nil
}

// classifyRuntime maps a runtime error from the compiled expression to a typed
// EvalError. Causes that are exposed as typed errors get a specific code; the
// rest fall back to the honest CodeRuntime placeholder rather than claiming a
// more precise classification the cause does not support.
func (c *CompiledDecision) classifyRuntime(err error) *EvalError {
	var mm *boxed.MultipleMatchError
	if errors.As(err, &mm) {
		return &EvalError{
			Code:       CodeUniqueMultiple,
			DecisionID: c.id,
			Message:    "UNIQUE hit policy matched multiple rules",
			Err:        err,
		}
	}
	return &EvalError{
		Code:       CodeRuntime,
		DecisionID: c.id,
		Message:    "evaluating decision",
		Err:        err,
	}
}
