package feel

import (
	"sort"
	"testing"

	"github.com/pblumer/temis/internal/value"
)

// evalStr compiles and evaluates src against the given variables, failing on any
// compile or eval error.
func evalStr(t *testing.T, src string, vars map[string]value.Value) value.Value {
	t.Helper()
	env := envFor(vars)
	ce, err := CompileString(src, env)
	if err != nil {
		t.Fatalf("compile %q: %v", src, err)
	}
	v, err := ce(env.NewScope(vars))
	if err != nil {
		t.Fatalf("eval %q: %v", src, err)
	}
	return v
}

func envFor(vars map[string]value.Value) *Env {
	names := make([]string, 0, len(vars))
	for k := range vars {
		names = append(names, k)
	}
	sort.Strings(names)
	return NewEnv(names...)
}

func TestEvalArithmetic(t *testing.T) {
	cases := map[string]string{
		"1 + 2 * 3":   "7",
		"(1 + 2) * 3": "9",
		"10 / 4":      "2.5",
		"2 ** 10":     "1024",
		"-5 + 1":      "-4",
		"2 ** 3 ** 2": "512", // right-assoc: 2**(3**2)
	}
	for src, want := range cases {
		if got := evalStr(t, src, nil); got.String() != want {
			t.Errorf("%q = %s, want %s", src, got, want)
		}
	}
}

func TestEvalDivisionByZeroIsNull(t *testing.T) {
	if got := evalStr(t, "1 / 0", nil); !value.IsNull(got) {
		t.Errorf("1/0 = %s, want null", got)
	}
	if got := evalStr(t, "1 + null", nil); !value.IsNull(got) {
		t.Errorf("1 + null = %s, want null", got)
	}
}

func TestEvalComparisons(t *testing.T) {
	cases := map[string]string{
		"1 < 2":     "true",
		"2 <= 2":    "true",
		"3 > 4":     "false",
		"3 = 3":     "true",
		"3 != 4":    "true",
		`"a" < "b"`: "true",
	}
	for src, want := range cases {
		if got := evalStr(t, src, nil); got.String() != want {
			t.Errorf("%q = %s, want %s", src, got, want)
		}
	}
	// Incomparable operands yield null.
	if got := evalStr(t, `1 < "a"`, nil); !value.IsNull(got) {
		t.Errorf(`1 < "a" = %s, want null`, got)
	}
}

func TestEvalThreeValuedLogic(t *testing.T) {
	cases := map[string]string{
		"true and true":  "true",
		"true and false": "false",
		"false and null": "false", // false dominates
		"true and null":  "null",
		"true or false":  "true",
		"null or true":   "true", // true dominates
		"false or false": "false",
		"null or false":  "null",
		"null and null":  "null",
	}
	for src, want := range cases {
		got := evalStr(t, src, nil)
		if got.String() != want {
			t.Errorf("%q = %s, want %s", src, got, want)
		}
	}
}

func TestEvalIf(t *testing.T) {
	cases := map[string]string{
		`if 1 < 2 then "y" else "n"`: "y",
		`if 2 < 1 then "y" else "n"`: "n",
		`if null then 1 else 2`:      "2", // non-true condition takes else
	}
	for src, want := range cases {
		if got := evalStr(t, src, nil); got.String() != want {
			t.Errorf("%q = %s, want %s", src, got, want)
		}
	}
}

func TestEvalVariables(t *testing.T) {
	vars := map[string]value.Value{
		"x": value.MustNumber("3"),
		"y": value.MustNumber("4"),
	}
	if got := evalStr(t, "x + y", vars); got.String() != "7" {
		t.Errorf("x + y = %s, want 7", got)
	}
	if got := evalStr(t, "x * y - 2", vars); got.String() != "10" {
		t.Errorf("x * y - 2 = %s, want 10", got)
	}
}

func TestEvalBetweenAndIn(t *testing.T) {
	cases := map[string]string{
		"5 between 1 and 10": "true",
		"5 between 6 and 10": "false",
		"2 in (1, 2, 3)":     "true",
		"5 in (1, 2, 3)":     "false",
		"5 in [1..10]":       "true",
		"10 in [1..10)":      "false", // open upper bound excludes 10
		"1 in [1..10)":       "true",
	}
	for src, want := range cases {
		if got := evalStr(t, src, nil); got.String() != want {
			t.Errorf("%q = %s, want %s", src, got, want)
		}
	}
}

func TestEvalLiterals(t *testing.T) {
	cases := map[string]string{
		"[1, 2, 3]":     "[1, 2, 3]",
		"{a: 1, b: 2}":  "{a: 1, b: 2}",
		"[1..10]":       "[1..10]",
		`@"2024-01-31"`: "2024-01-31",
		`@"P1Y2M"`:      "P1Y2M",
		`@"12:30:00"`:   "12:30:00",
	}
	for src, want := range cases {
		if got := evalStr(t, src, nil); got.String() != want {
			t.Errorf("%q = %s, want %s", src, got, want)
		}
	}
}

func TestEvalPath(t *testing.T) {
	rec := value.NewContext().Put("x", value.MustNumber("5")).Put("y", value.MustNumber("9"))
	vars := map[string]value.Value{"rec": rec}
	if got := evalStr(t, "rec.x + rec.y", vars); got.String() != "14" {
		t.Errorf("rec.x + rec.y = %s, want 14", got)
	}
	// Path on a non-context yields null.
	if got := evalStr(t, "rec.missing", vars); !value.IsNull(got) {
		t.Errorf("rec.missing = %s, want null", got)
	}
}

func TestCompileErrors(t *testing.T) {
	cases := []struct {
		src       string
		line, col int
	}{
		{"x + 1", 1, 1},                     // unknown variable
		{"f(1)", 1, 1},                      // unknown function, at the name position
		{"function(x) x + y", 1, 17},        // function body references unknown variable
		{"some x in xs satisfies x", 1, 11}, // unknown domain variable "xs"
	}
	for _, c := range cases {
		_, err := CompileString(c.src, NewEnv())
		if err == nil {
			t.Errorf("Compile(%q) = nil error, want error", c.src)
			continue
		}
		ce, ok := err.(*CompileError)
		if !ok {
			t.Errorf("Compile(%q) error type %T, want *CompileError", c.src, err)
			continue
		}
		if ce.Line != c.line || ce.Col != c.col {
			t.Errorf("Compile(%q) error at %d:%d, want %d:%d (%s)", c.src, ce.Line, ce.Col, c.line, c.col, ce.Msg)
		}
	}
}

func TestEnvScopeBoundary(t *testing.T) {
	env := NewEnv("a", "b")
	// A name absent from the input map becomes null.
	scope := env.NewScope(map[string]value.Value{"a": value.MustNumber("1")})
	ce, err := CompileString("b", env)
	if err != nil {
		t.Fatal(err)
	}
	v, _ := ce(scope)
	if !value.IsNull(v) {
		t.Errorf("absent variable b = %s, want null", v)
	}
	if env.Names()[0] != "a" || env.Names()[1] != "b" {
		t.Errorf("env slot order = %v, want [a b]", env.Names())
	}
}

func TestCompileErrorMessageAndExtras(t *testing.T) {
	_, err := CompileString("x", NewEnv())
	ce := err.(*CompileError)
	if ce.Error() != "1:1: "+ce.Msg {
		t.Errorf("Error() = %q", ce.Error())
	}

	// datetime @-literal and >= comparison.
	if got := evalStr(t, `@"2024-01-31T12:30:00"`, nil); got.String() != "2024-01-31T12:30:00" {
		t.Errorf("datetime literal = %s", got)
	}
	if got := evalStr(t, "3 >= 3", nil); got.String() != "true" {
		t.Errorf("3 >= 3 = %s, want true", got)
	}
	// between with incomparable bounds yields null.
	if got := evalStr(t, `5 between "a" and 10`, nil); !value.IsNull(got) {
		t.Errorf(`5 between "a" and 10 = %s, want null`, got)
	}

	// invalid temporal literal is a compile error.
	if _, err := CompileString(`@"not-temporal"`, NewEnv()); err == nil {
		t.Error("invalid @-literal should fail to compile")
	}
	// a syntax error surfaces as a *ParseError, not a *CompileError.
	if _, err := CompileString("1 +", NewEnv()); err == nil {
		t.Error("syntax error should propagate")
	} else if _, ok := err.(*ParseError); !ok {
		t.Errorf("expected *ParseError, got %T", err)
	}
}

func BenchmarkCompile(b *testing.B) {
	env := NewEnv("a", "b", "c")
	expr, err := Parse("a + b * 2 - c")
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Compile(expr, env); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEval(b *testing.B) {
	env := NewEnv("a", "b", "c")
	ce, err := CompileString("a + b * 2 - c", env)
	if err != nil {
		b.Fatal(err)
	}
	scope := env.NewScope(map[string]value.Value{
		"a": value.MustNumber("1"),
		"b": value.MustNumber("2"),
		"c": value.MustNumber("3"),
	})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ce(scope); err != nil {
			b.Fatal(err)
		}
	}
}
