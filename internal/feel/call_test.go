package feel

import (
	"testing"

	"github.com/pblumer/temis/internal/value"
)

func TestEvalBuiltinCalls(t *testing.T) {
	cases := map[string]string{
		"count([1, 2, 3])":             "3",
		"sum(1, 2, 3)":                 "6",
		"sum([10, 20, 30])":            "60",
		"mean([2, 4, 6])":              "4",
		"min([3, 1, 2])":               "1",
		"max([3, 1, 2])":               "3",
		`string length("hello")`:       "5",
		`upper case("abc")`:            "ABC",
		`starts with("foobar", "foo")`: "true",
		`ends with("foobar", "bar")`:   "true",
		`ends with("foobar", "foo")`:   "false",
		`contains("foobar", "oob")`:    "true",
		`substring("foobar", 2, 3)`:    "oob",
		"not(true)":                    "false",
		"floor(2.7)":                   "2",
		"ceiling(2.1)":                 "3",
		"abs(-5)":                      "5",
		`abs(duration("-P1D"))`:        "P1DT0H0M0S", // abs on a days-time duration
		`abs(duration("-P1Y"))`:        "P1Y0M",      // abs on a years-months duration
		`number("3.14")`:               "3.14",
		`list contains([1, 2, 3], 2)`:  "true",
		// `in` with a parenthesised operator test (single element, no comma)
		"10 in (= 10)":   "true",
		"10 in (!= 10)":  "false",
		`"a" in (= "a")`: "true",
	}
	for src, want := range cases {
		if got := evalStr(t, src, nil); got.String() != want {
			t.Errorf("%q = %s, want %s", src, got, want)
		}
	}
}

func TestEvalBuiltinNullPropagation(t *testing.T) {
	for _, src := range []string{
		`string length(42)`,
		`sum([1, "a"])`,
		`starts with(1, "x")`,
	} {
		if got := evalStr(t, src, nil); !value.IsNull(got) {
			t.Errorf("%q = %s, want null", src, got)
		}
	}
}

func TestEvalNamedArguments(t *testing.T) {
	// Named arguments, including a multi-word parameter name.
	src := `substring(string: "foobar", start position: 2, length: 3)`
	if got := evalStr(t, src, nil); got.String() != "oob" {
		t.Errorf("%q = %s, want oob", src, got)
	}
}

func TestEvalNestedCalls(t *testing.T) {
	if got := evalStr(t, `sum([string length("ab"), string length("cde")])`, nil); got.String() != "5" {
		t.Errorf("nested call = %s, want 5", got)
	}
}

// TestCallOfNonFunctionYieldsNull: invoking an unknown name or a non-function
// callee is a total-function null, not a compile error, so the decision stays
// executable (WP-41.17, TCK 1131).
func TestCallOfNonFunctionYieldsNull(t *testing.T) {
	for _, src := range []string{
		"bogus(1)", // unknown function with an argument
		"1(2)",     // a number callee with an argument
	} {
		if _, err := CompileString(src, NewEnv()); err != nil {
			t.Errorf("Compile(%q) = %v, want no error", src, err)
			continue
		}
		if got := evalStr(t, src, nil); !value.IsNull(got) {
			t.Errorf("eval(%q) = %s, want null", src, got)
		}
	}
}

// TestInvalidInvocationYieldsNull covers FEEL's total-function semantics: a
// syntactically valid call with the wrong arity or unknown/mixed named parameters
// compiles and evaluates to null (it does not make the decision non-executable).
func TestInvalidInvocationYieldsNull(t *testing.T) {
	for _, src := range []string{
		"count()",                        // too few arguments
		`substring("a", 1, 2, 3)`,        // too many arguments
		`substring("foobar", length: 3)`, // mixed positional and named
		`upper case(value: "x")`,         // unknown parameter name
		`count(x: [1, 2])`,               // named args on a no-param builtin
	} {
		if _, err := CompileString(src, NewEnv()); err != nil {
			t.Errorf("Compile(%q) = %v, want no error", src, err)
			continue
		}
		if got := evalStr(t, src, nil); !value.IsNull(got) {
			t.Errorf("eval(%q) = %s, want null", src, got)
		}
	}
}
