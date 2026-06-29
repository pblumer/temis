package boxed

import (
	"fmt"
	"strings"

	"github.com/pblumer/temis/internal/feel"
	"github.com/pblumer/temis/internal/model"
)

// compileInvocation compiles a boxed invocation. The common case — the called
// expression is a literal naming a business knowledge model — binds each
// argument to the model's formal parameter by name, so missing parameters
// default to null and order is irrelevant. Otherwise the called expression is
// evaluated to a function value and the arguments are passed in binding order.
func compileInvocation(inv *model.Invocation, env *feel.Env, funcs map[string]*feel.Func) (feel.CompiledExpr, error) {
	if name := calledName(inv.Called); name != "" {
		if f, ok := funcs[name]; ok {
			return compileNamedInvocation(name, f, inv, env, funcs)
		}
	}

	callee, err := Compile(inv.Called, env, funcs)
	if err != nil {
		return nil, fmt.Errorf("invocation callee: %w", err)
	}
	args := make([]feel.CompiledExpr, len(inv.Bindings))
	for i, b := range inv.Bindings {
		ce, err := Compile(b.Value, env, funcs)
		if err != nil {
			return nil, fmt.Errorf("invocation binding %q: %w", b.Parameter, err)
		}
		args[i] = ce
	}
	return feel.CallValue(callee, args), nil
}

// compileNamedInvocation binds the invocation's arguments into f's parameter
// order, reporting a binding to an unknown parameter as an error.
func compileNamedInvocation(name string, f *feel.Func, inv *model.Invocation, env *feel.Env, funcs map[string]*feel.Func) (feel.CompiledExpr, error) {
	args := make([]feel.CompiledExpr, len(f.Params))
	for i := range args {
		args[i] = feel.NullExpr
	}
	for _, b := range inv.Bindings {
		idx := indexOf(f.Params, b.Parameter)
		if idx < 0 {
			return nil, fmt.Errorf("invocation of %q: no parameter %q", name, b.Parameter)
		}
		ce, err := Compile(b.Value, env, funcs)
		if err != nil {
			return nil, fmt.Errorf("invocation of %q binding %q: %w", name, b.Parameter, err)
		}
		args[idx] = ce
	}
	return feel.CallFunc(f, args), nil
}

// calledName returns the function name an invocation's called expression refers
// to when it is a literal expression holding a plain name, else "".
func calledName(e model.Expression) string {
	if le, ok := e.(*model.LiteralExpression); ok {
		return strings.TrimSpace(le.Text)
	}
	return ""
}
