package feel

import (
	"testing"

	"github.com/pblumer/temis/internal/value"
)

// TestDottedNames covers WP-41.25: a variable whose name contains a dot (e.g. a
// BKM formal parameter "Person.Gender") assembles as one name when the oracle
// knows it, while ordinary path access "a.b" (no such flat name) still navigates.
func TestDottedNames(t *testing.T) {
	vars := map[string]value.Value{
		"Person.Gender": value.Str("Male"),
		"a":             value.NewContext().Put("b", value.MustNumber("7")),
	}
	if got := evalStr(t, "Person.Gender", vars); got.String() != "Male" {
		t.Errorf(`"Person.Gender" = %s, want Male`, got.String())
	}
	// a.b is still path access into the context (no flat variable "a.b").
	if got := evalStr(t, "a.b", vars); got.String() != "7" {
		t.Errorf("a.b path access = %s, want 7", got.String())
	}
}

// TestUnaryTestMembership covers WP-41.25: a decision-table unary test whose value
// is a list is a membership test, while a scalar reduces to equality.
func TestUnaryTestMembership(t *testing.T) {
	env := NewEnv("?", "xs")
	scope := func(input value.Value, xs value.Value) *Scope {
		return env.NewScope(map[string]value.Value{"?": input, "xs": xs})
	}
	test, err := CompileUnaryTest("xs", env)
	if err != nil {
		t.Fatal(err)
	}
	list := value.NewList(value.Str("cough"), value.Str("fever"))
	ok, _ := Matches(test, scope(value.Str("cough"), list))
	if !ok {
		t.Error(`unary test "xs" with "cough" in the list should match`)
	}
	ok, _ = Matches(test, scope(value.Str("x"), list))
	if ok {
		t.Error(`unary test "xs" with "x" not in the list should not match`)
	}
}
