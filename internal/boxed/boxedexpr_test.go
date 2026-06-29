package boxed

import (
	"strings"
	"testing"

	"github.com/pblumer/temis/internal/feel"
	"github.com/pblumer/temis/internal/model"
	"github.com/pblumer/temis/internal/value"
)

func lit(text string) *model.LiteralExpression { return &model.LiteralExpression{Text: text} }

func evalExpr(t *testing.T, expr model.Expression, env *feel.Env, funcs map[string]*feel.Func, in map[string]value.Value) (value.Value, error) {
	t.Helper()
	ce, err := Compile(expr, env, funcs)
	if err != nil {
		return nil, err
	}
	return ce(env.NewScope(in))
}

func mustFunc(t *testing.T, name string, params []string, body string, funcs map[string]*feel.Func) *feel.Func {
	t.Helper()
	f := &feel.Func{Name: name, Params: params}
	if funcs != nil {
		funcs[name] = f
	}
	ce, err := feel.CompileStringWith(body, feel.NewEnv(params...), funcs)
	if err != nil {
		t.Fatalf("compile func %q body: %v", name, err)
	}
	f.Body = ce
	return f
}

func TestContextResultCellSequential(t *testing.T) {
	// base = x * 2; doubled = base + 1; result = doubled
	ctx := &model.ContextExpr{Entries: []model.ContextEntry{
		{Name: "base", Value: lit("x * 2")},
		{Name: "doubled", Value: lit("base + 1")},
		{Value: lit("doubled")},
	}}
	got, err := evalExpr(t, ctx, feel.NewEnv("x"), nil, map[string]value.Value{"x": value.MustNumber("5")})
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "11" {
		t.Errorf("context result = %s, want 11", got)
	}
}

func TestContextWithoutResultCellIsContext(t *testing.T) {
	ctx := &model.ContextExpr{Entries: []model.ContextEntry{
		{Name: "a", Value: lit("1")},
		{Name: "b", Value: lit("a + 1")},
	}}
	got, err := evalExpr(t, ctx, feel.NewEnv(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	c, ok := got.(*value.Context)
	if !ok {
		t.Fatalf("context value type %T, want *value.Context", got)
	}
	if b, _ := c.Get("b"); b.String() != "2" {
		t.Errorf("context b = %s, want 2", b)
	}
}

func TestContextResultCellMustBeLast(t *testing.T) {
	ctx := &model.ContextExpr{Entries: []model.ContextEntry{
		{Value: lit("1")}, // result cell, not last
		{Name: "a", Value: lit("2")},
	}}
	if _, err := Compile(ctx, feel.NewEnv(), nil); err == nil {
		t.Error("result cell before the last entry should be a compile error")
	}
}

func TestInvocationNamedBinding(t *testing.T) {
	funcs := map[string]*feel.Func{}
	mustFunc(t, "rate", []string{"total"}, "if total > 1000 then 0.2 else 0.1", funcs)

	inv := &model.Invocation{
		Called: lit("rate"),
		Bindings: []model.Binding{
			{Parameter: "total", Value: lit("x")},
		},
	}
	got, err := evalExpr(t, inv, feel.NewEnv("x"), funcs, map[string]value.Value{"x": value.MustNumber("1500")})
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "0.2" {
		t.Errorf("rate(1500) = %s, want 0.2", got)
	}
}

func TestInvocationUnknownParameter(t *testing.T) {
	funcs := map[string]*feel.Func{}
	mustFunc(t, "rate", []string{"total"}, "total", funcs)
	inv := &model.Invocation{
		Called:   lit("rate"),
		Bindings: []model.Binding{{Parameter: "wrong", Value: lit("1")}},
	}
	if _, err := Compile(inv, feel.NewEnv(), funcs); err == nil {
		t.Error("binding an unknown parameter should be a compile error")
	}
}

func TestInvocationCalleeNotAFunction(t *testing.T) {
	// Called expression is a number, not a function: the result is null.
	inv := &model.Invocation{Called: lit("42")}
	got, err := evalExpr(t, inv, feel.NewEnv(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !value.IsNull(got) {
		t.Errorf("invoking a non-function = %s, want null", got)
	}
}

func TestInvocationOfInlineFunction(t *testing.T) {
	// The called expression is a function definition, not a named BKM, so
	// arguments bind positionally in their listed order.
	inv := &model.Invocation{
		Called: &model.FunctionDef{
			Parameters: []model.FunctionParam{{Name: "a"}},
			Body:       lit("a * 2"),
		},
		Bindings: []model.Binding{{Value: lit("3")}},
	}
	got, err := evalExpr(t, inv, feel.NewEnv(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "6" {
		t.Errorf("inline invocation = %s, want 6", got)
	}
}

func TestInvocationCompileErrors(t *testing.T) {
	funcs := map[string]*feel.Func{}
	mustFunc(t, "rate", []string{"total"}, "total", funcs)
	cases := map[string]*model.Invocation{
		"bad callee":      {Called: lit("1 +")},
		"bad general arg": {Called: &model.FunctionDef{Body: lit("1")}, Bindings: []model.Binding{{Value: lit("2 *")}}},
		"bad named arg":   {Called: lit("rate"), Bindings: []model.Binding{{Parameter: "total", Value: lit("2 *")}}},
	}
	for name, inv := range cases {
		if _, err := Compile(inv, feel.NewEnv(), funcs); err == nil {
			t.Errorf("%s: expected a compile error", name)
		}
	}
}

func TestFunctionDefinitionBodyCompileError(t *testing.T) {
	fn := &model.FunctionDef{Parameters: []model.FunctionParam{{Name: "y"}}, Body: lit("y +")}
	if _, err := Compile(fn, feel.NewEnv(), nil); err == nil {
		t.Error("malformed function body should be a compile error")
	}
}

func TestFunctionDefinitionAsLogic(t *testing.T) {
	fn := &model.FunctionDef{
		Parameters: []model.FunctionParam{{Name: "y"}},
		Body:       lit("y + 1"),
	}
	got, err := evalExpr(t, fn, feel.NewEnv(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	f, ok := got.(*value.Function)
	if !ok {
		t.Fatalf("function definition value type %T, want *value.Function", got)
	}
	res, err := f.Call([]value.Value{value.MustNumber("4")})
	if err != nil {
		t.Fatal(err)
	}
	if res.String() != "5" {
		t.Errorf("(function(y) y+1)(4) = %s, want 5", res)
	}
}

func TestFunctionDefinitionNonFEELKind(t *testing.T) {
	fn := &model.FunctionDef{Kind: "Java", Body: lit("x")}
	if _, err := Compile(fn, feel.NewEnv(), nil); err == nil {
		t.Error("a non-FEEL function kind should not be executable")
	}
}

func TestCompileNil(t *testing.T) {
	if _, err := Compile(nil, feel.NewEnv(), nil); err == nil {
		t.Error("Compile(nil) should error")
	}
}

func TestCompileTableErrorPropagatesThroughDispatch(t *testing.T) {
	dt := mkTable(model.HitUnique, model.AggNone, []string{"1 +"}, []string{"o"},
		r{[]string{"-"}, []string{"1"}})
	if _, err := Compile(dt, feel.NewEnv(), nil); err == nil || !strings.Contains(err.Error(), "input") {
		t.Errorf("dispatch should surface table compile error, got %v", err)
	}
}
