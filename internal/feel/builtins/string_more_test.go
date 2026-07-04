package builtins

import (
	"testing"

	"github.com/pblumer/temis/internal/value"
)

func TestStringMoreBuiltins(t *testing.T) {
	run(t, []tc{
		{name: "substring before", args: []value.Value{str("foobar"), str("bar")}, want: "foo"},
		{name: "substring before", args: []value.Value{str("foobar"), str("xyz")}, want: ""},
		{name: "substring after", args: []value.Value{str("foobar"), str("ob")}, want: "ar"},
		{name: "substring after", args: []value.Value{str("foobar"), str("xyz")}, want: ""},
		{name: "substring before", args: []value.Value{num("1"), str("x")}, wantNull: true},

		{name: "matches", args: []value.Value{str("foobar"), str("^fo*bar$")}, want: "true"},
		{name: "matches", args: []value.Value{str("FooBar"), str("foobar"), str("i")}, want: "true"},
		{name: "matches", args: []value.Value{str("abc"), str("xyz")}, want: "false"},
		{name: "matches", args: []value.Value{str("abc"), str("(")}, wantNull: true},
		{name: "matches", args: []value.Value{num("1"), str("x")}, wantNull: true},

		{name: "replace", args: []value.Value{str("abcd"), str("(ab)|(a)"), str("[1=$1][2=$2]")}, want: "[1=ab][2=]cd"},
		{name: "replace", args: []value.Value{str("a.b.c"), str("\\."), str("-")}, want: "a-b-c"},
		{name: "replace", args: []value.Value{str("x"), str("("), str("y")}, wantNull: true},

		{name: "string join", args: []value.Value{list(str("a"), str("b"), str("c"))}, want: "abc"},
		{name: "string join", args: []value.Value{list(str("a"), str("b")), str("-")}, want: "a-b"},
		{name: "string join", args: []value.Value{list(str("a"), value.Null, str("c")), str(",")}, want: "a,c"},
		{name: "string join", args: []value.Value{list(str("a"), num("1"))}, wantNull: true},
		// a null list argument yields null (not the empty string).
		{name: "string join", args: []value.Value{value.Null}, wantNull: true},
		{name: "string join", args: []value.Value{value.Null, str("X")}, wantNull: true},
	})
}

func TestSplitBuiltin(t *testing.T) {
	got := call(t, "split", str("a;b;c"), str(";"))
	want := value.NewList(str("a"), str("b"), str("c"))
	if value.Equal(got, want) != value.True {
		t.Errorf("split = %s, want %s", got, want.String())
	}
	got = call(t, "split", str("foo  bar"), str("\\s+"))
	want = value.NewList(str("foo"), str("bar"))
	if value.Equal(got, want) != value.True {
		t.Errorf("split regex = %s, want %s", got, want.String())
	}
	if got := call(t, "split", num("1"), str(";")); !value.IsNull(got) {
		t.Errorf("split(number) = %s, want null", got)
	}
}
