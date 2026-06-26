package builtins

import (
	"testing"

	"github.com/pblumer/temis/internal/value"
)

func TestSortBuiltin(t *testing.T) {
	l := list(num("3"), num("1"), num("2"))
	// natural ascending
	eqList(t, "sort", []value.Value{l}, value.NewList(num("1"), num("2"), num("3")))

	// descending via a precedes function a > b
	desc := &value.Function{Name: "desc", Arity: 2, Call: func(args []value.Value) (value.Value, error) {
		c, ok := value.Compare(args[0], args[1])
		return value.BoolOf(ok && c > 0), nil
	}}
	eqList(t, "sort", []value.Value{l, desc}, value.NewList(num("3"), num("2"), num("1")))

	if got := call(t, "sort", num("1")); !value.IsNull(got) {
		t.Errorf("sort(non-list) = %s, want null", got)
	}
}

// rng builds a numeric range [lo..hi] with the given closed flags.
func rng(loClosed bool, lo, hi string, hiClosed bool) value.Range {
	return value.Range{LowClosed: loClosed, Low: num(lo), High: num(hi), HighClosed: hiClosed}
}

func TestRangeBuiltins(t *testing.T) {
	closed := func(lo, hi string) value.Range { return rng(true, lo, hi, true) }

	run(t, []tc{
		// before / after — points and ranges
		{name: "before", args: []value.Value{num("1"), num("10")}, want: "true"},
		{name: "before", args: []value.Value{num("10"), num("1")}, want: "false"},
		{name: "after", args: []value.Value{num("10"), num("1")}, want: "true"},
		{name: "before", args: []value.Value{num("1"), closed("5", "8")}, want: "true"},
		{name: "before", args: []value.Value{num("5"), closed("5", "8")}, want: "false"},
		{name: "before", args: []value.Value{num("5"), rng(false, "5", "8", true)}, want: "true"},
		{name: "before", args: []value.Value{closed("1", "5"), closed("6", "8")}, want: "true"},
		{name: "before", args: []value.Value{closed("1", "5"), closed("5", "8")}, want: "false"},

		// meets / met by
		{name: "meets", args: []value.Value{closed("1", "5"), closed("5", "8")}, want: "true"},
		{name: "meets", args: []value.Value{rng(true, "1", "5", false), closed("5", "8")}, want: "false"},
		{name: "met by", args: []value.Value{closed("5", "8"), closed("1", "5")}, want: "true"},

		// overlaps
		{name: "overlaps", args: []value.Value{closed("1", "5"), closed("3", "8")}, want: "true"},
		{name: "overlaps", args: []value.Value{closed("1", "5"), closed("6", "8")}, want: "false"},
		{name: "overlaps before", args: []value.Value{closed("1", "5"), closed("3", "8")}, want: "true"},
		{name: "overlaps after", args: []value.Value{closed("3", "8"), closed("1", "5")}, want: "true"},

		// includes / during
		{name: "includes", args: []value.Value{closed("1", "10"), num("5")}, want: "true"},
		{name: "includes", args: []value.Value{closed("1", "10"), num("10")}, want: "true"},
		{name: "includes", args: []value.Value{rng(true, "1", "10", false), num("10")}, want: "false"},
		{name: "during", args: []value.Value{num("5"), closed("1", "10")}, want: "true"},

		// starts / started by
		{name: "starts", args: []value.Value{num("1"), closed("1", "10")}, want: "true"},
		{name: "starts", args: []value.Value{num("1"), rng(false, "1", "10", true)}, want: "false"},
		{name: "started by", args: []value.Value{closed("1", "10"), num("1")}, want: "true"},

		// finishes / finished by
		{name: "finishes", args: []value.Value{num("10"), closed("1", "10")}, want: "true"},
		{name: "finishes", args: []value.Value{num("10"), rng(true, "1", "10", false)}, want: "false"},
		{name: "finished by", args: []value.Value{closed("1", "10"), num("10")}, want: "true"},

		// coincides
		{name: "coincides", args: []value.Value{num("5"), num("5")}, want: "true"},
		{name: "coincides", args: []value.Value{closed("1", "5"), closed("1", "5")}, want: "true"},
		{name: "coincides", args: []value.Value{closed("1", "5"), rng(true, "1", "5", false)}, want: "false"},

		// null propagation
		{name: "before", args: []value.Value{value.Null, num("1")}, wantNull: true},
		{name: "meets", args: []value.Value{num("1"), num("2")}, wantNull: true}, // points: not defined for meets
	})
}
