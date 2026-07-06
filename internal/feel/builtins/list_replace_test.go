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
		// out-of-range position → null.
		{"list replace", []value.Value{list(num("1")), num("5"), num("9")}, "", true},
		// a non-list argument coerces to a one-element list (FEEL singleton coercion).
		{"list replace", []value.Value{str("x"), num("1"), num("9")}, "[9]", false},
		// a non-integer position truncates toward zero.
		{"list replace", []value.Value{list(num("1"), num("2"), num("3")), num("2.5"), num("9")}, "[1, 9, 3]", false},

		// number with grouping + decimal separators.
		{"number", []value.Value{str("1.000,5"), str("."), str(",")}, "1000.5", false},
		{"number", []value.Value{str("1 000.5"), str(" "), str(".")}, "1000.5", false},
		{"number", []value.Value{str("nope")}, "", true},

		// is: value AND type equality.
		{"is", []value.Value{num("1"), num("1")}, "true", false},
		{"is", []value.Value{num("1"), str("1")}, "false", false},
		{"is", []value.Value{value.Null, value.Null}, "true", false},
		{"is", []value.Value{num("1"), value.Null}, "false", false},
		// is on temporals compares representation, not the instant: two times that
		// are the same instant but rendered differently are not `is`.
		{"is", []value.Value{mustTime("23:00:50"), mustTime("23:00:50Z")}, "false", false},
		{"is", []value.Value{mustTime("20:00:50+00:00"), mustTime("21:00:50+01:00")}, "false", false},
		{"is", []value.Value{mustTime("10:30:00Z"), mustTime("10:30:00Z")}, "true", false},
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

func mustTime(s string) value.Value {
	t, err := value.ParseTime(s)
	if err != nil {
		panic(err)
	}
	return t
}
