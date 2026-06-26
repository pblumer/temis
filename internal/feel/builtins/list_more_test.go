package builtins

import (
	"testing"

	"github.com/pblumer/temis/internal/value"
)

// eqList asserts a builtin returns a list FEEL-equal to want.
func eqList(t *testing.T, name string, args []value.Value, want value.Value) {
	t.Helper()
	got := call(t, name, args...)
	if value.Equal(got, want) != value.True {
		t.Errorf("%s = %s, want %s", name, got, want.String())
	}
}

func TestListAllAny(t *testing.T) {
	run(t, []tc{
		{name: "all", args: []value.Value{list(value.True, value.True)}, want: "true"},
		{name: "all", args: []value.Value{list(value.True, value.False)}, want: "false"},
		{name: "all", args: []value.Value{list()}, want: "true"},
		{name: "all", args: []value.Value{list(value.True, value.Null)}, wantNull: true},
		{name: "all", args: []value.Value{list(value.False, value.Null)}, want: "false"},
		{name: "any", args: []value.Value{list(value.False, value.False)}, want: "false"},
		{name: "any", args: []value.Value{list(value.False, value.True)}, want: "true"},
		{name: "any", args: []value.Value{list()}, want: "false"},
		{name: "any", args: []value.Value{list(value.False, value.Null)}, wantNull: true},
		{name: "any", args: []value.Value{list(value.True, value.Null)}, want: "true"},
	})
}

func TestListShapeBuiltins(t *testing.T) {
	l123 := list(num("1"), num("2"), num("3"))
	eqList(t, "sublist", []value.Value{l123, num("2")}, value.NewList(num("2"), num("3")))
	eqList(t, "sublist", []value.Value{l123, num("1"), num("2")}, value.NewList(num("1"), num("2")))
	eqList(t, "sublist", []value.Value{l123, num("-1")}, value.NewList(num("3")))
	eqList(t, "append", []value.Value{l123, num("4"), num("5")}, value.NewList(num("1"), num("2"), num("3"), num("4"), num("5")))
	eqList(t, "concatenate", []value.Value{list(num("1")), list(num("2"), num("3"))}, l123)
	eqList(t, "insert before", []value.Value{l123, num("2"), num("9")}, value.NewList(num("1"), num("9"), num("2"), num("3")))
	eqList(t, "remove", []value.Value{l123, num("2")}, value.NewList(num("1"), num("3")))
	eqList(t, "reverse", []value.Value{l123}, value.NewList(num("3"), num("2"), num("1")))
	eqList(t, "index of", []value.Value{list(num("1"), num("2"), num("1")), num("1")}, value.NewList(num("1"), num("3")))
	eqList(t, "union", []value.Value{list(num("1"), num("2")), list(num("2"), num("3"))}, l123)
	eqList(t, "distinct values", []value.Value{list(num("1"), num("2"), num("1"), num("3"))}, l123)
	eqList(t, "flatten", []value.Value{list(num("1"), list(num("2"), list(num("3"))))}, l123)

	// null propagation / bounds
	run(t, []tc{
		{name: "append", args: []value.Value{num("1"), num("2")}, wantNull: true},
		{name: "remove", args: []value.Value{l123, num("9")}, wantNull: true},
		{name: "insert before", args: []value.Value{l123, num("0"), num("9")}, wantNull: true},
		{name: "sublist", args: []value.Value{l123, num("0")}, wantNull: true},
	})
}

func TestListStatBuiltins(t *testing.T) {
	run(t, []tc{
		{name: "product", args: []value.Value{list(num("2"), num("3"), num("4"))}, want: "24"},
		{name: "product", args: []value.Value{num("2"), num("3")}, want: "6"},
		{name: "product", args: []value.Value{list()}, wantNull: true},
		{name: "product", args: []value.Value{list(num("2"), str("x"))}, wantNull: true},
		{name: "median", args: []value.Value{list(num("3"), num("1"), num("2"))}, want: "2"},
		{name: "median", args: []value.Value{list(num("1"), num("2"), num("3"), num("4"))}, want: "2.5"},
		{name: "median", args: []value.Value{list()}, wantNull: true},
		{name: "stddev", args: []value.Value{list(num("2"), num("4"), num("4"), num("4"), num("5"), num("5"), num("7"), num("9"))}, want: "2.138089935299395077476427847038028"},
		{name: "stddev", args: []value.Value{list(num("5"))}, wantNull: true},
	})
	// mode returns a (sorted) list.
	eqList(t, "mode", []value.Value{list(num("6"), num("3"), num("9"), num("6"), num("6"))}, value.NewList(num("6")))
	eqList(t, "mode", []value.Value{list(num("1"), num("2"), num("3"), num("3"), num("1"))}, value.NewList(num("1"), num("3")))
	eqList(t, "mode", []value.Value{list()}, value.NewList())
}
