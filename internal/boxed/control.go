package boxed

import (
	"fmt"

	"github.com/pblumer/temis/internal/feel"
	"github.com/pblumer/temis/internal/model"
)

// compileConditional compiles a boxed if/then/else. A missing else branch
// evaluates to null.
func compileConditional(c *model.Conditional, env *feel.Env, funcs map[string]*feel.Func) (feel.CompiledExpr, error) {
	cond, err := Compile(c.If, env, funcs)
	if err != nil {
		return nil, fmt.Errorf("conditional if: %w", err)
	}
	then, err := Compile(c.Then, env, funcs)
	if err != nil {
		return nil, fmt.Errorf("conditional then: %w", err)
	}
	els, err := compileOrNull(c.Else, env, funcs)
	if err != nil {
		return nil, fmt.Errorf("conditional else: %w", err)
	}
	return feel.IfThenElse(cond, then, els), nil
}

// compileFor compiles a boxed iterator into a list comprehension. The iterator
// variable is bound for the return expression.
func compileFor(f *model.ForExpr, env *feel.Env, funcs map[string]*feel.Func) (feel.CompiledExpr, error) {
	if f.IteratorVariable == "" {
		return nil, fmt.Errorf("for: missing iterator variable")
	}
	coll, err := Compile(f.In, env, funcs)
	if err != nil {
		return nil, fmt.Errorf("for in: %w", err)
	}
	body, err := Compile(f.Return, env.Append(f.IteratorVariable), funcs)
	if err != nil {
		return nil, fmt.Errorf("for return: %w", err)
	}
	return feel.ForOne(coll, body), nil
}

// compileQuantified compiles a boxed some/every. The iterator variable is bound
// for the satisfies expression.
func compileQuantified(q *model.Quantified, env *feel.Env, funcs map[string]*feel.Func) (feel.CompiledExpr, error) {
	if q.IteratorVariable == "" {
		return nil, fmt.Errorf("%s: missing iterator variable", q.Kind)
	}
	coll, err := Compile(q.In, env, funcs)
	if err != nil {
		return nil, fmt.Errorf("%s in: %w", q.Kind, err)
	}
	pred, err := Compile(q.Satisfies, env.Append(q.IteratorVariable), funcs)
	if err != nil {
		return nil, fmt.Errorf("%s satisfies: %w", q.Kind, err)
	}
	return feel.QuantifyOne(q.Kind == "some", coll, pred), nil
}

// compileFilter compiles a boxed filter. The match predicate must be a literal
// FEEL expression; it sees each element as the implicit variable item (with the
// element's context keys resolving directly).
func compileFilter(f *model.FilterExpr, env *feel.Env, funcs map[string]*feel.Func) (feel.CompiledExpr, error) {
	coll, err := Compile(f.In, env, funcs)
	if err != nil {
		return nil, fmt.Errorf("filter in: %w", err)
	}
	le, ok := f.Match.(*model.LiteralExpression)
	if !ok {
		return nil, fmt.Errorf("filter match must be a literal expression, got %T", f.Match)
	}
	return feel.BoxedFilter(coll, le.Text, env, funcs)
}

// compileOrNull compiles expr, or yields null when expr is absent.
func compileOrNull(expr model.Expression, env *feel.Env, funcs map[string]*feel.Func) (feel.CompiledExpr, error) {
	if expr == nil {
		return feel.NullExpr, nil
	}
	return Compile(expr, env, funcs)
}
