package dmn

import (
	"context"

	"github.com/pblumer/temis/internal/feel"
)

// CompiledExpression is a standalone compiled FEEL expression: the engine's FEEL
// evaluator exposed on its own, without a surrounding decision. It is immutable
// and safe to evaluate concurrently. Compile once with CompileExpression, then
// Evaluate repeatedly against different inputs.
//
// It exists so higher layers can use FEEL as a small expression language over a
// named context — notably the decision-flow layer (package flow, ADR-0026), whose
// step-input mappings are FEEL expressions over the flow's inputs and earlier
// steps' outputs.
type CompiledExpression struct {
	env  *feel.Env
	expr feel.CompiledExpr
	refs []string
}

// CompileExpression parses and compiles a FEEL expression that may reference the
// given variable names. A name the expression uses that is not in names is a
// compile error ("unknown variable"), so the caller declares the context up front
// — exactly the names it will supply to Evaluate.
//
// The expression is compiled with the engine's default configuration: the full
// FEEL built-in library and standard value semantics (ADR-0003/0007). now() and
// today() read the process clock and are therefore not deterministic here; pass a
// fixed value as an input instead when determinism matters.
func CompileExpression(expr string, names ...string) (*CompiledExpression, error) {
	env := feel.NewEnv(names...)
	ce, refs, err := feel.CompileStringRefs(expr, env)
	if err != nil {
		return nil, err
	}
	return &CompiledExpression{env: env, expr: ce, refs: refs}, nil
}

// References returns the subset of the declared names the expression actually
// uses, sorted. It lets a caller learn an expression's dependencies (e.g. to
// order a graph of expressions) without re-parsing it.
func (c *CompiledExpression) References() []string {
	out := make([]string, len(c.refs))
	copy(out, c.refs)
	return out
}

// Evaluate runs the expression against in (variable name → Go value, converted to
// FEEL values as documented on CompiledDecision.Evaluate) and returns the result
// converted back to Go. A name declared at compile time but absent from in
// evaluates to null. A spec-conformant FEEL null (type mismatch, division by
// zero, …) is returned as a nil value, not an error.
func (c *CompiledExpression) Evaluate(ctx context.Context, in Input) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	vals, err := inputToValues(in)
	if err != nil {
		return nil, err
	}
	out, err := c.expr(c.env.NewScope(vals))
	if err != nil {
		return nil, err
	}
	return fromValue(out), nil
}
