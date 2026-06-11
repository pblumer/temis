package feel

import (
	"testing"

	"github.com/pblumer/temis/internal/value"
)

// matchTest compiles a unary-test cell and reports whether input matches it,
// with optional extra decision variables in scope.
func matchTest(t *testing.T, src string, input value.Value, extra map[string]value.Value) bool {
	t.Helper()
	names := []string{InputVar}
	for k := range extra {
		names = append(names, k)
	}
	env := NewEnv(names...)
	ce, err := CompileUnaryTest(src, env)
	if err != nil {
		t.Fatalf("compile unary test %q: %v", src, err)
	}
	vars := map[string]value.Value{InputVar: input}
	for k, v := range extra {
		vars[k] = v
	}
	ok, err := Matches(ce, env.NewScope(vars))
	if err != nil {
		t.Fatalf("match %q: %v", src, err)
	}
	return ok
}

func num(s string) value.Value { return value.MustNumber(s) }
func str(s string) value.Value { return value.Str(s) }

func TestUnaryTestValues(t *testing.T) {
	cases := []struct {
		src   string
		input value.Value
		want  bool
	}{
		{`"Spring"`, str("Spring"), true},
		{`"Spring"`, str("Fall"), false},
		{"42", num("42"), true},
		{"42", num("7"), false},
		{"-5", num("-5"), true}, // equality with negative number, not the dash
		{"-5", num("5"), false},
	}
	for _, c := range cases {
		if got := matchTest(t, c.src, c.input, nil); got != c.want {
			t.Errorf("%q matches %s = %v, want %v", c.src, c.input, got, c.want)
		}
	}
}

func TestUnaryTestIntervals(t *testing.T) {
	cases := []struct {
		src   string
		input string
		want  bool
	}{
		{"[1..10]", "5", true},
		{"[1..10]", "1", true},
		{"[1..10]", "10", true},
		{"[1..10]", "11", false},
		{"[1..10)", "10", false}, // open upper bound
		{"(0..100]", "0", false}, // open lower bound
		{"(0..100]", "100", true},
	}
	for _, c := range cases {
		if got := matchTest(t, c.src, num(c.input), nil); got != c.want {
			t.Errorf("%q matches %s = %v, want %v", c.src, c.input, got, c.want)
		}
	}
}

func TestUnaryTestEnumeration(t *testing.T) {
	if !matchTest(t, "1, 2, 3", num("2"), nil) {
		t.Error("2 should match 1, 2, 3")
	}
	if matchTest(t, "1, 2, 3", num("5"), nil) {
		t.Error("5 should not match 1, 2, 3")
	}
	if !matchTest(t, `"A", "B"`, str("B"), nil) {
		t.Error(`"B" should match "A", "B"`)
	}
	if matchTest(t, "[1..5], >= 100", num("100"), nil) == false {
		t.Error("100 should match [1..5], >= 100")
	}
	if !matchTest(t, "[1..5], >= 100", num("3"), nil) {
		t.Error("3 should match [1..5], >= 100")
	}
	if matchTest(t, "[1..5], >= 100", num("50"), nil) {
		t.Error("50 should not match [1..5], >= 100")
	}
}

func TestUnaryTestDashAndEmpty(t *testing.T) {
	if !matchTest(t, "-", num("999"), nil) {
		t.Error("dash should always match")
	}
	if !matchTest(t, "", str("anything"), nil) {
		t.Error("empty cell should always match")
	}
}

func TestUnaryTestOperators(t *testing.T) {
	cases := []struct {
		src   string
		input string
		want  bool
	}{
		{"< 18", "17", true},
		{"< 18", "18", false},
		{"<= 18", "18", true},
		{"> 5", "6", true},
		{"> 5", "5", false},
		{">= 5", "5", true},
	}
	for _, c := range cases {
		if got := matchTest(t, c.src, num(c.input), nil); got != c.want {
			t.Errorf("%q matches %s = %v, want %v", c.src, c.input, got, c.want)
		}
	}
}

func TestUnaryTestReferencesOtherInput(t *testing.T) {
	extra := map[string]value.Value{"limit": num("100")}
	if !matchTest(t, "< limit", num("50"), extra) {
		t.Error("50 should be < limit(100)")
	}
	if matchTest(t, "< limit", num("150"), extra) {
		t.Error("150 should not be < limit(100)")
	}
}

func TestUnaryTestNegation(t *testing.T) {
	if matchTest(t, "not(1, 2)", num("1"), nil) {
		t.Error("not(1, 2) should reject 1")
	}
	if !matchTest(t, "not(1, 2)", num("3"), nil) {
		t.Error("not(1, 2) should accept 3")
	}
	if matchTest(t, "not(< 5)", num("3"), nil) {
		t.Error("not(< 5) should reject 3")
	}
	if !matchTest(t, "not(< 5)", num("10"), nil) {
		t.Error("not(< 5) should accept 10")
	}
}

func TestUnaryTestExplicitInput(t *testing.T) {
	if !matchTest(t, "? > 5", num("6"), nil) {
		t.Error("? > 5 should accept 6")
	}
	if !matchTest(t, `? != "x"`, str("y"), nil) {
		t.Error(`? != "x" should accept "y"`)
	}
}

func TestUnaryTestExplicitInputForms(t *testing.T) {
	cases := []struct {
		src   string
		input value.Value
		want  bool
	}{
		{"? in (1, 2, 3)", num("2"), true},
		{"? in (1, 2, 3)", num("9"), false},
		{"if ? > 0 then true else false", num("5"), true},
		{"? between 1 and 10", num("5"), true},
		{"? between 1 and 10", num("50"), false},
		{`string length(?) >= 3`, str("abcd"), true},
		{`string length(?) >= 3`, str("ab"), false},
		{"count([?, 1]) > 0", num("9"), true}, // ? inside a list argument
	}
	for _, c := range cases {
		if got := matchTest(t, c.src, c.input, nil); got != c.want {
			t.Errorf("%q matches %s = %v, want %v", c.src, c.input, got, c.want)
		}
	}
}

func TestUnaryTestNullInput(t *testing.T) {
	// A null input matches nothing (comparisons and equality with null are not a match).
	if matchTest(t, "< 5", value.Null, nil) {
		t.Error("null should not match < 5")
	}
	if matchTest(t, `"Spring"`, value.Null, nil) {
		t.Error(`null should not match "Spring"`)
	}
	// ... except the catch-all dash.
	if !matchTest(t, "-", value.Null, nil) {
		t.Error("null should match the dash")
	}
}

func TestUnaryTestCompileErrors(t *testing.T) {
	env := NewEnv(InputVar)
	for _, src := range []string{"<", "[1..", "not(", "1 2"} {
		if _, err := CompileUnaryTest(src, env); err == nil {
			t.Errorf("CompileUnaryTest(%q) = nil error, want error", src)
		}
	}
}
