package feel

import (
	"testing"

	"github.com/pblumer/temis/internal/value"
)

// TestAggregateNamedListArg covers WP-41.24: variadic aggregates accept the
// single-collection form as a named "list" argument (TCK 0059/0062).
func TestAggregateNamedListArg(t *testing.T) {
	cases := map[string]string{
		"all(list: [true, false, true])": "false",
		"any(list: [false, true])":       "true",
		"sum(list: [1, 2, 3])":           "6",
		"count(list: [1, 2, 3])":         "3",
		"mode(list: [6, 3, 9, 6, 6])[1]": "6",
	}
	for src, want := range cases {
		if got := evalStr(t, src, nil); got.String() != want {
			t.Errorf("%s = %s, want %s", src, got.String(), want)
		}
	}
}

// TestBuiltinAsFunctionValue covers WP-41.24: a bare built-in name lifts to a
// first-class function value that can be passed and applied (TCK 0092).
func TestBuiltinAsFunctionValue(t *testing.T) {
	// Reference `abs` as a value, then apply it.
	got := evalStr(t, "(function(f) f(-5))(abs)", nil)
	if got.String() != "5" {
		t.Errorf("applying abs as a value = %s, want 5", got.String())
	}
	if v := evalStr(t, "abs instance of function<>->Any", nil); v != value.True {
		t.Errorf("abs instance of function = %s, want true", v.String())
	}
}
