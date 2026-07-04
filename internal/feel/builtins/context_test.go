package builtins

import (
	"testing"

	"github.com/pblumer/temis/internal/value"
)

func ctx(pairs ...value.Value) *value.Context {
	c := value.NewContext()
	for i := 0; i+1 < len(pairs); i += 2 {
		c.Put(string(pairs[i].(value.Str)), pairs[i+1])
	}
	return c
}

func TestContextBuiltins(t *testing.T) {
	m := ctx(str("a"), num("1"), str("b"), num("2"))

	if got := call(t, "get value", m, str("a")); got.String() != "1" {
		t.Errorf("get value a = %s, want 1", got)
	}
	if got := call(t, "get value", m, str("z")); !value.IsNull(got) {
		t.Errorf("get value z = %s, want null", got)
	}
	// nested via key list
	nested := ctx(str("x"), m)
	if got := call(t, "get value", nested, value.NewList(str("x"), str("b"))); got.String() != "2" {
		t.Errorf("nested get value = %s, want 2", got)
	}

	// context put returns a copy; original unchanged.
	put := call(t, "context put", m, str("c"), num("3"))
	if got, _ := put.(*value.Context).Get("c"); got.String() != "3" {
		t.Errorf("context put c = %s, want 3", got)
	}
	if _, ok := m.Get("c"); ok {
		t.Error("context put mutated its input")
	}

	// context merge: later wins.
	merged := call(t, "context merge", value.NewList(m, ctx(str("b"), num("9"), str("c"), num("3"))))
	mc := merged.(*value.Context)
	if v, _ := mc.Get("b"); v.String() != "9" {
		t.Errorf("merge b = %s, want 9 (later wins)", v)
	}
	if v, _ := mc.Get("a"); v.String() != "1" {
		t.Errorf("merge a = %s, want 1", v)
	}

	// get entries / context round-trip.
	entries := call(t, "get entries", m)
	rebuilt := call(t, "context", entries)
	if value.Equal(rebuilt, m) != value.True {
		t.Errorf("context(get entries(m)) = %s, want %s", rebuilt, m)
	}

	// null propagation
	if got := call(t, "get value", num("1"), str("a")); !value.IsNull(got) {
		t.Errorf("get value on non-context = %s, want null", got)
	}
}

// TestContextPutPathAndContextEdges covers the WP-41.5 additions: nested
// context put via a key path, and the context() constructor's single-entry and
// duplicate-key handling.
func TestContextPutPathAndContextEdges(t *testing.T) {
	ctx := func(kv ...value.Value) value.Value {
		c := value.NewContext()
		for i := 0; i+1 < len(kv); i += 2 {
			c.Put(string(kv[i].(value.Str)), kv[i+1])
		}
		return c
	}
	list := func(vs ...value.Value) value.Value { return value.NewList(vs...) }
	s := func(x string) value.Value { return value.Str(x) }
	n := value.MustNumber

	// context put with a path list updates a nested entry.
	nested := ctx(s("x"), n("1"), s("y"), ctx(s("a"), n("0")))
	got := call(t, "context put", nested, list(s("y"), s("a")), n("2"))
	if got.String() != "{x: 1, y: {a: 2}}" {
		t.Errorf("nested context put = %s, want {x: 1, y: {a: 2}}", got)
	}
	// A path into a non-context intermediate is null.
	if got := call(t, "context put", nested, list(s("x"), s("a")), n("2")); !value.IsNull(got) {
		t.Errorf("path through scalar = %s, want null", got)
	}
	// An empty path is null.
	if got := call(t, "context put", nested, list(), n("2")); !value.IsNull(got) {
		t.Errorf("empty path = %s, want null", got)
	}

	// context() accepts a single unwrapped entry.
	entry := ctx(s("key"), s("a"), s("value"), n("1"))
	if got := call(t, "context", entry); got.String() != "{a: 1}" {
		t.Errorf("context(single entry) = %s, want {a: 1}", got)
	}
	// Duplicate keys make the result null.
	dup := list(ctx(s("key"), s("a"), s("value"), n("1")), ctx(s("key"), s("a"), s("value"), n("2")))
	if got := call(t, "context", dup); !value.IsNull(got) {
		t.Errorf("context(duplicate keys) = %s, want null", got)
	}
}
