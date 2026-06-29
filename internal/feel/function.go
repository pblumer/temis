package feel

import (
	"fmt"

	"github.com/pblumer/temis/internal/value"
)

// Func is a user-defined FEEL function: a business knowledge model's
// encapsulated logic, or a function(...) literal compiled as a named callable.
//
// Body is compiled against an Env whose trailing slots are the formal Params (in
// order); calling the function runs Body in a scope whose those slots hold the
// argument values. Body is assigned after compilation so a function may
// reference itself (recursion) or sibling functions (mutual recursion) by name.
type Func struct {
	Name   string
	Params []string
	Body   CompiledExpr
}

// call runs f with the given positional argument values, taking the recursion
// budget from the caller scope s. Missing arguments default to null and surplus
// arguments are ignored, so the body always sees exactly len(Params) slots.
func (f *Func) call(s *Scope, args []value.Value) (value.Value, error) {
	st := s.st
	if st != nil {
		if st.depth >= st.maxDepth {
			return nil, fmt.Errorf("feel: call depth limit %d exceeded calling %s", st.maxDepth, f.label())
		}
		st.depth++
		defer func() { st.depth-- }()
	}
	if f.Body == nil {
		return nil, fmt.Errorf("feel: function %s has no body", f.label())
	}
	vars := make([]value.Value, len(f.Params))
	for i := range vars {
		if i < len(args) {
			vars[i] = args[i]
		} else {
			vars[i] = value.Null
		}
	}
	return f.Body(&Scope{vars: vars, st: st})
}

// asValue lifts f into a first-class FEEL function value bound to the recursion
// budget of scope s, so a named function can be passed to higher-order built-ins
// or stored in a variable.
func (f *Func) asValue(s *Scope) value.Value {
	return &value.Function{
		Name:  f.Name,
		Arity: len(f.Params),
		Call:  func(args []value.Value) (value.Value, error) { return f.call(s, args) },
	}
}

func (f *Func) label() string {
	if f.Name != "" {
		return fmt.Sprintf("%q", f.Name)
	}
	return "anonymous function"
}

// CallFunc returns a CompiledExpr that calls the statically known function f with
// the given compiled arguments (already arranged in f's parameter order), under
// the recursion-depth limit. It is the entry point boxed invocations use to call
// a business knowledge model.
func CallFunc(f *Func, args []CompiledExpr) CompiledExpr {
	return func(s *Scope) (value.Value, error) {
		vals, err := evalArgs(args, s)
		if err != nil {
			return nil, err
		}
		return f.call(s, vals)
	}
}

// CallValue returns a CompiledExpr that evaluates callee to a function value and
// calls it with the compiled positional arguments. A callee that is not a
// function yields null.
func CallValue(callee CompiledExpr, args []CompiledExpr) CompiledExpr {
	return func(s *Scope) (value.Value, error) {
		fv, err := callee(s)
		if err != nil {
			return nil, err
		}
		fn, ok := fv.(*value.Function)
		if !ok {
			return value.Null, nil
		}
		vals, err := evalArgs(args, s)
		if err != nil {
			return nil, err
		}
		return fn.Call(vals)
	}
}

// funcNames is a NameSet over user-function names, letting the parser assemble
// multi-word function names (e.g. a BKM named "Rate Table") the same way it does
// for multi-word built-ins.
type funcNames map[string]*Func

func (f funcNames) Has(name string) bool {
	_, ok := f[name]
	return ok
}

// unionNames is a NameSet that reports a name known if any member set knows it.
type unionNames []NameSet

func (u unionNames) Has(name string) bool {
	for _, s := range u {
		if s != nil && s.Has(name) {
			return true
		}
	}
	return false
}
