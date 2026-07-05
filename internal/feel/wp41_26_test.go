package feel

import (
	"testing"

	"github.com/pblumer/temis/internal/value"
)

// TestNamedArgOmitsParams covers WP-41.26: a named call may omit optional
// parameters (they default to null) rather than being rejected on arity — e.g.
// is(value1: x) reduces to is(x, null) (TCK 0103).
func TestNamedArgOmitsParams(t *testing.T) {
	if got := evalStr(t, `is(value1: @"2021-02-13")`, nil); got != value.False {
		t.Errorf(`is(value1: @date) = %s, want false`, got.String())
	}
	if got := evalStr(t, `is(value1: 1, value2: 1)`, nil); got != value.True {
		t.Errorf("is(1, 1) = %s, want true", got.String())
	}
}

// TestConditionalNonBoolean covers WP-41.26: a genuine non-boolean `if` condition
// yields null, while false and null take the else branch (TCK 1150).
func TestConditionalNonBoolean(t *testing.T) {
	if got := evalStr(t, `if "abc" then 1 else 2`, nil); !value.IsNull(got) {
		t.Errorf(`if "abc" then 1 else 2 = %s, want null`, got.String())
	}
	if got := evalStr(t, `if null then 1 else 2`, nil); got.String() != "2" {
		t.Errorf("if null then 1 else 2 = %s, want 2 (else)", got.String())
	}
	if got := evalStr(t, `if true then 1 else 2`, nil); got.String() != "1" {
		t.Errorf("if true = %s, want 1", got.String())
	}
}
