package builtins

import (
	"testing"

	"github.com/pblumer/temis/internal/value"
)

// TestListReplaceAndIsAndNumber covers the DMN-1.5 builtins added for TCK
// conformance (WP-41): list replace (position + match-function forms), is, and
// the multi-argument number form.
func TestListReplaceAndIsAndNumber(t *testing.T) {
	run(t, []tc{
		// list replace by position (1-indexed; negative from the end).
		{"list replace", []value.Value{list(num("1"), num("2"), num("3")), num("2"), num("9")}, "[1, 9, 3]", false},
		{"list replace", []value.Value{list(num("1"), num("2"), num("3")), num("-1"), num("9")}, "[1, 2, 9]", false},
		// out-of-range position and non-list → null.
		{"list replace", []value.Value{list(num("1")), num("5"), num("9")}, "", true},
		{"list replace", []value.Value{str("x"), num("1"), num("9")}, "", true},

		// number with grouping + decimal separators.
		{"number", []value.Value{str("1.000,5"), str("."), str(",")}, "1000.5", false},
		{"number", []value.Value{str("1 000.5"), str(" "), str(".")}, "1000.5", false},
		{"number", []value.Value{str("nope")}, "", true},

		// is: value AND type equality.
		{"is", []value.Value{num("1"), num("1")}, "true", false},
		{"is", []value.Value{num("1"), str("1")}, "false", false},
		{"is", []value.Value{value.Null, value.Null}, "true", false},
		{"is", []value.Value{num("1"), value.Null}, "false", false},
	})

	// list replace with a match function replaces every matching element.
	fn := &value.Function{
		Name:  "match",
		Arity: 2,
		Call: func(args []value.Value) (value.Value, error) {
			return value.BoolOf(value.Equal(args[0], value.MustNumber("2")) == value.True), nil
		},
	}
	got := call(t, "list replace", list(num("2"), num("3"), num("2")), fn, num("9"))
	if got.String() != "[9, 3, 9]" {
		t.Errorf("list replace(match) = %s, want [9, 3, 9]", got)
	}
}
