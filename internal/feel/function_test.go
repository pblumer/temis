package feel

import (
	"strings"
	"testing"

	"github.com/pblumer/temis/internal/value"
)

func evalWith(t *testing.T, src string, env *Env, funcs map[string]*Func, in map[string]value.Value) (value.Value, error) {
	t.Helper()
	ce, err := CompileStringWith(src, env, funcs)
	if err != nil {
		t.Fatalf("compile %q: %v", src, err)
	}
	return ce(env.NewScope(in))
}

func TestFunctionLiteralCall(t *testing.T) {
	got, err := evalWith(t, "(function(x) x + 1)(4)", NewEnv(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "5" {
		t.Errorf("(function(x) x+1)(4) = %s, want 5", got)
	}
}

func TestFunctionClosesOverEnclosingScope(t *testing.T) {
	// The literal captures `base` from the enclosing scope at definition time.
	env := NewEnv("base")
	got, err := evalWith(t, "(function(x) x + base)(5)", env, nil,
		map[string]value.Value{"base": value.NumberFromInt64(10)})
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "15" {
		t.Errorf("closure over base = %s, want 15", got)
	}
}

func TestNamedFunctionRecursion(t *testing.T) {
	funcs := map[string]*Func{}
	fact := &Func{Name: "fact", Params: []string{"n"}}
	funcs["fact"] = fact
	body, err := CompileStringWith("if n <= 1 then 1 else n * fact(n - 1)", NewEnv("n"), funcs)
	if err != nil {
		t.Fatalf("compile body: %v", err)
	}
	fact.Body = body

	got, err := evalWith(t, "fact(x)", NewEnv("x"), funcs,
		map[string]value.Value{"x": value.NumberFromInt64(5)})
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "120" {
		t.Errorf("fact(5) = %s, want 120", got)
	}
}

func TestRecursionDepthLimit(t *testing.T) {
	funcs := map[string]*Func{}
	loop := &Func{Name: "loop", Params: []string{"n"}}
	funcs["loop"] = loop
	body, err := CompileStringWith("loop(n + 1)", NewEnv("n"), funcs)
	if err != nil {
		t.Fatalf("compile body: %v", err)
	}
	loop.Body = body

	_, err = evalWith(t, "loop(x)", NewEnv("x"), funcs,
		map[string]value.Value{"x": value.NumberFromInt64(0)})
	if err == nil {
		t.Fatal("unbounded recursion: got nil error, want depth-limit error")
	}
	if !strings.Contains(err.Error(), "depth limit") {
		t.Errorf("error = %v, want a depth-limit error", err)
	}
}

func TestFunctionValueReference(t *testing.T) {
	// Referencing a named function (not calling it) yields a function value.
	funcs := map[string]*Func{}
	double := &Func{Name: "double", Params: []string{"x"}}
	funcs["double"] = double
	body, _ := CompileStringWith("x * 2", NewEnv("x"), funcs)
	double.Body = body

	v, err := evalWith(t, "double", NewEnv(), funcs, nil)
	if err != nil {
		t.Fatal(err)
	}
	fn, ok := v.(*value.Function)
	if !ok {
		t.Fatalf("reference to double = %T, want *value.Function", v)
	}
	got, err := fn.Call([]value.Value{value.NumberFromInt64(21)})
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "42" {
		t.Errorf("double(21) = %s, want 42", got)
	}
}

func TestMultiWordFunctionName(t *testing.T) {
	// A space-containing function name assembles via the parser's name oracle.
	funcs := map[string]*Func{}
	rt := &Func{Name: "rate table", Params: []string{"x"}}
	funcs["rate table"] = rt
	body, _ := CompileStringWith("x + 1", NewEnv("x"), funcs)
	rt.Body = body

	got, err := evalWith(t, "rate table(x)", NewEnv("x"), funcs,
		map[string]value.Value{"x": value.NumberFromInt64(9)})
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "10" {
		t.Errorf("rate table(9) = %s, want 10", got)
	}
}

func TestFuncCallArgumentErrors(t *testing.T) {
	funcs := map[string]*Func{}
	sub := &Func{Name: "sub", Params: []string{"a", "b"}}
	funcs["sub"] = sub
	body, _ := CompileStringWith("a - b", NewEnv("a", "b"), funcs)
	sub.Body = body

	// FEEL: a user-function call with the wrong arity or unknown/mixed named
	// parameters compiles and evaluates to null (total-function semantics), rather
	// than failing to compile.
	for _, src := range []string{
		"sub(1, b: 2)", // mixed positional and named
		"sub(1, 2, 3)", // too many arguments
		"sub(c: 1)",    // unknown parameter
	} {
		env := NewEnv()
		ce, err := CompileStringWith(src, env, funcs)
		if err != nil {
			t.Errorf("%q: Compile = %v, want no error", src, err)
			continue
		}
		got, err := ce(env.NewScope(nil))
		if err != nil {
			t.Errorf("%q: eval error %v", src, err)
			continue
		}
		if !value.IsNull(got) {
			t.Errorf("%q = %s, want null", src, got)
		}
	}
}

func TestValueCallErrors(t *testing.T) {
	// Named arguments are rejected when the callee is not a statically known func.
	if _, err := CompileString("(function(x) x)(y: 1)", NewEnv()); err == nil {
		t.Error("named args on a value call should be a compile error")
	}
	// Calling a variable that holds a non-function yields null at runtime.
	got, err := evalWith(t, "n(1)", NewEnv("n"), nil, map[string]value.Value{"n": value.NumberFromInt64(5)})
	if err != nil {
		t.Fatal(err)
	}
	if !value.IsNull(got) {
		t.Errorf("calling a non-function variable = %s, want null", got)
	}
}

func TestCallHelpers(t *testing.T) {
	// CallFunc and CallValue are the entry points boxed invocations build on.
	funcs := map[string]*Func{}
	add := &Func{Name: "add", Params: []string{"a", "b"}}
	funcs["add"] = add
	body, _ := CompileStringWith("a + b", NewEnv("a", "b"), funcs)
	add.Body = body

	env := NewEnv()
	args := []CompiledExpr{
		func(*Scope) (value.Value, error) { return value.NumberFromInt64(3), nil },
		func(*Scope) (value.Value, error) { return value.NumberFromInt64(4), nil },
	}
	scope := env.NewScope(nil)

	got, err := CallFunc(add, args)(scope)
	if err != nil || got.String() != "7" {
		t.Errorf("CallFunc add(3,4) = %v, %v, want 7", got, err)
	}

	callee := func(s *Scope) (value.Value, error) { return add.asValue(s), nil }
	got, err = CallValue(callee, args)(scope)
	if err != nil || got.String() != "7" {
		t.Errorf("CallValue add(3,4) = %v, %v, want 7", got, err)
	}

	// CallValue on a non-function callee yields null.
	notFn := func(*Scope) (value.Value, error) { return value.NumberFromInt64(1), nil }
	got, err = CallValue(notFn, nil)(scope)
	if err != nil || !value.IsNull(got) {
		t.Errorf("CallValue on non-function = %v, %v, want null", got, err)
	}
}

func TestNullExpr(t *testing.T) {
	got, err := NullExpr(NewEnv().NewScope(nil))
	if err != nil || !value.IsNull(got) {
		t.Errorf("NullExpr = %v, %v, want null", got, err)
	}
}

func TestNamedArgumentsToFunction(t *testing.T) {
	funcs := map[string]*Func{}
	sub := &Func{Name: "sub", Params: []string{"a", "b"}}
	funcs["sub"] = sub
	body, _ := CompileStringWith("a - b", NewEnv("a", "b"), funcs)
	sub.Body = body

	got, err := evalWith(t, "sub(b: 3, a: 10)", NewEnv(), funcs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "7" {
		t.Errorf("sub(b:3, a:10) = %s, want 7", got)
	}
}
