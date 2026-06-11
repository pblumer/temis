package builtins

import (
	"testing"

	"github.com/pblumer/temis/internal/value"
)

func call(t *testing.T, name string, args ...value.Value) value.Value {
	t.Helper()
	b, ok := Default().Lookup(name)
	if !ok {
		t.Fatalf("builtin %q not registered", name)
	}
	v, err := b.Fn(args)
	if err != nil {
		t.Fatalf("%s returned error: %v", name, err)
	}
	return v
}

func num(s string) value.Value           { return value.MustNumber(s) }
func str(s string) value.Value           { return value.Str(s) }
func list(vs ...value.Value) value.Value { return value.NewList(vs...) }

// each case checks one builtin; want is the expected FEEL string, or "" with
// wantNull to assert null.
type tc struct {
	name     string
	args     []value.Value
	want     string
	wantNull bool
}

func run(t *testing.T, cases []tc) {
	t.Helper()
	for _, c := range cases {
		got := call(t, c.name, c.args...)
		if c.wantNull {
			if !value.IsNull(got) {
				t.Errorf("%s(%v) = %s, want null", c.name, c.args, got)
			}
			continue
		}
		if value.IsNull(got) || got.String() != c.want {
			t.Errorf("%s(%v) = %s, want %s", c.name, c.args, got, c.want)
		}
	}
}

func TestBoolean(t *testing.T) {
	run(t, []tc{
		{name: "not", args: []value.Value{value.True}, want: "false"},
		{name: "not", args: []value.Value{value.False}, want: "true"},
		{name: "not", args: []value.Value{num("5")}, wantNull: true},
	})
}

func TestListBuiltins(t *testing.T) {
	run(t, []tc{
		{name: "count", args: []value.Value{list(num("1"), num("2"), num("3"))}, want: "3"},
		{name: "count", args: []value.Value{num("1"), num("2"), num("3")}, want: "3"},
		{name: "count", args: []value.Value{list()}, want: "0"},
		{name: "sum", args: []value.Value{list(num("1"), num("2"), num("3"))}, want: "6"},
		{name: "sum", args: []value.Value{list()}, wantNull: true},
		{name: "sum", args: []value.Value{list(num("1"), str("a"))}, wantNull: true},
		{name: "mean", args: []value.Value{list(num("2"), num("4"))}, want: "3"},
		{name: "mean", args: []value.Value{list()}, wantNull: true},
		{name: "min", args: []value.Value{list(num("3"), num("1"), num("2"))}, want: "1"},
		{name: "max", args: []value.Value{list(num("3"), num("1"), num("2"))}, want: "3"},
		{name: "min", args: []value.Value{list()}, wantNull: true},
		{name: "min", args: []value.Value{list(num("1"), str("a"))}, wantNull: true},
		{name: "list contains", args: []value.Value{list(num("1"), num("2")), num("2")}, want: "true"},
		{name: "list contains", args: []value.Value{list(num("1"), num("2")), num("5")}, want: "false"},
		{name: "list contains", args: []value.Value{num("1"), num("2")}, wantNull: true},
	})
}

func TestStringBuiltins(t *testing.T) {
	run(t, []tc{
		{name: "string length", args: []value.Value{str("abcé")}, want: "4"},
		{name: "string length", args: []value.Value{num("5")}, wantNull: true},
		{name: "upper case", args: []value.Value{str("aBc")}, want: "ABC"},
		{name: "lower case", args: []value.Value{str("aBc")}, want: "abc"},
		{name: "upper case", args: []value.Value{num("5")}, wantNull: true},
		{name: "contains", args: []value.Value{str("foobar"), str("oob")}, want: "true"},
		{name: "contains", args: []value.Value{str("foobar"), str("xyz")}, want: "false"},
		{name: "contains", args: []value.Value{num("1"), str("x")}, wantNull: true},
		{name: "starts with", args: []value.Value{str("foobar"), str("foo")}, want: "true"},
		{name: "ends with", args: []value.Value{str("foobar"), str("bar")}, want: "true"},
		{name: "substring", args: []value.Value{str("foobar"), num("2")}, want: "oobar"},
		{name: "substring", args: []value.Value{str("foobar"), num("2"), num("3")}, want: "oob"},
		{name: "substring", args: []value.Value{str("foobar"), num("-2"), num("1")}, want: "a"},
		{name: "substring", args: []value.Value{str("foobar"), num("0")}, wantNull: true},
		{name: "substring", args: []value.Value{num("5"), num("1")}, wantNull: true},
	})
}

func TestConversionBuiltins(t *testing.T) {
	dt, _ := value.ParseDateTime("2024-01-31T12:30:00")
	run(t, []tc{
		{name: "number", args: []value.Value{str("3.14")}, want: "3.14"},
		{name: "number", args: []value.Value{num("5")}, want: "5"},
		{name: "number", args: []value.Value{str("xyz")}, wantNull: true},
		{name: "string", args: []value.Value{num("5")}, want: "5"},
		{name: "string", args: []value.Value{value.Null}, wantNull: true},
		{name: "date", args: []value.Value{str("2024-01-31")}, want: "2024-01-31"},
		{name: "date", args: []value.Value{dt}, want: "2024-01-31"},
		{name: "date", args: []value.Value{str("nope")}, wantNull: true},
	})
}

func TestNumericBuiltins(t *testing.T) {
	run(t, []tc{
		{name: "floor", args: []value.Value{num("2.7")}, want: "2"},
		{name: "floor", args: []value.Value{num("-2.1")}, want: "-3"},
		{name: "ceiling", args: []value.Value{num("2.1")}, want: "3"},
		{name: "ceiling", args: []value.Value{num("-2.7")}, want: "-2"},
		{name: "abs", args: []value.Value{num("-5")}, want: "5"},
		{name: "abs", args: []value.Value{num("5")}, want: "5"},
		{name: "floor", args: []value.Value{str("x")}, wantNull: true},
	})
}

func TestRegistryNamesAndHas(t *testing.T) {
	r := Default()
	if !r.Has("sum") || r.Has("nonexistent") {
		t.Error("Has() mismatch")
	}
	if len(r.Names()) < 18 {
		t.Errorf("expected at least 18 builtins, got %d", len(r.Names()))
	}
}
