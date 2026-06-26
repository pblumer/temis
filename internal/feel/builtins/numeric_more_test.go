package builtins

import (
	"testing"

	"github.com/pblumer/temis/internal/value"
)

func TestNumericRoundingBuiltins(t *testing.T) {
	run(t, []tc{
		{name: "decimal", args: []value.Value{num("3.14159"), num("2")}, want: "3.14"},
		{name: "decimal", args: []value.Value{num("2.5"), num("0")}, want: "2"}, // half-even
		{name: "decimal", args: []value.Value{str("x"), num("2")}, wantNull: true},
		{name: "round up", args: []value.Value{num("1.121"), num("2")}, want: "1.13"},
		{name: "round down", args: []value.Value{num("1.126"), num("2")}, want: "1.12"},
		{name: "round half up", args: []value.Value{num("5.5"), num("0")}, want: "6"},
		{name: "round half down", args: []value.Value{num("5.5"), num("0")}, want: "5"},

		{name: "floor", args: []value.Value{num("2.7")}, want: "2"},
		{name: "floor", args: []value.Value{num("-2.1")}, want: "-3"},
		{name: "floor", args: []value.Value{num("1.567"), num("2")}, want: "1.56"},
		{name: "ceiling", args: []value.Value{num("1.561"), num("2")}, want: "1.57"},

		{name: "modulo", args: []value.Value{num("12"), num("5")}, want: "2"},
		{name: "modulo", args: []value.Value{num("-12"), num("5")}, want: "3"},
		{name: "modulo", args: []value.Value{num("1"), num("0")}, wantNull: true},

		{name: "sqrt", args: []value.Value{num("16")}, want: "4"},
		{name: "sqrt", args: []value.Value{num("-1")}, wantNull: true},
		{name: "log", args: []value.Value{num("1")}, want: "0"},
		{name: "log", args: []value.Value{num("0")}, wantNull: true},
		{name: "exp", args: []value.Value{num("0")}, want: "1"},

		{name: "even", args: []value.Value{num("2")}, want: "true"},
		{name: "even", args: []value.Value{num("3")}, want: "false"},
		{name: "even", args: []value.Value{num("2.5")}, wantNull: true},
		{name: "odd", args: []value.Value{num("3")}, want: "true"},
		{name: "odd", args: []value.Value{num("2")}, want: "false"},
	})
}
