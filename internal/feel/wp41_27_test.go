package feel

import (
	"testing"

	"github.com/pblumer/temis/internal/value"
)

// TestNamedTemporalConstructors covers the DMN component named-argument forms of
// the temporal constructors, which bind via the builtins' alternate signatures
// (TCK 1115/1116/1117). The single-argument from: form must keep working.
func TestNamedTemporalConstructors(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{`date(year:2017, month:8, day:30)`, "2017-08-30"},
		{`time(hour:11, minute:59, second:0, offset: duration("PT2H1M0S"))`, "11:59:00+02:01"},
		{`time(hour:11, minute:59, second:0, offset: duration("-PT2H"))`, "11:59:00-02:00"},
		{`date and time(date:date("2017-01-01"), time:time("23:59:01"))`, "2017-01-01T23:59:01"},
		// The single-argument from: forms remain bound.
		{`time(from: "12:45:00")`, "12:45:00"},
		{`date and time(from: "2012-12-24T23:59:00")`, "2012-12-24T23:59:00"},
	}
	for _, c := range cases {
		got := evalStr(t, c.src, nil)
		if got.String() != c.want {
			t.Errorf("%s = %s, want %s", c.src, got, c.want)
		}
	}
}

// TestNamedContextFunctions covers the named-argument forms of context merge and
// context put, including the nested keys: path form (TCK 1146/1147).
func TestNamedContextFunctions(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{`context merge(contexts: [{"a": 1}])`, `{a: 1}`},
		{`context merge(contexts: {"a": 1})`, `{a: 1}`},
		{`context put(context: {x:1, y: {a: 0}}, keys: ["y", "a"], value: 2)`, `{x: 1, y: {a: 2}}`},
	}
	for _, c := range cases {
		got := evalStr(t, c.src, nil)
		if got.String() != c.want {
			t.Errorf("%s = %s, want %s", c.src, got, c.want)
		}
	}
}

// TestBoxedFilterNonBooleanIsNull covers a filter whose predicate yields a
// genuine non-boolean value: the whole filter is null, while false/null
// predicates still simply exclude the element (TCK 1151).
func TestBoxedFilterNonBooleanIsNull(t *testing.T) {
	if got := evalStr(t, `[1,2,3,4,5][if item <= 3 then true else "x"]`, nil); !value.IsNull(got) {
		t.Errorf(`non-boolean filter predicate = %s, want null`, got)
	}
	if got := evalStr(t, `[1,2,3,4,5]["x"]`, nil); !value.IsNull(got) {
		t.Errorf(`constant non-boolean filter = %s, want null`, got)
	}
	// A null predicate still excludes elements rather than nullifying the filter.
	if got := evalStr(t, `[1,2,3][item > 5]`, nil); got.String() != "[]" {
		t.Errorf(`all-false filter = %s, want []`, got)
	}
}

// TestAtLiteralInvalidIsNull covers an @-literal with invalid content evaluating
// to null rather than a compile error (TCK 0093).
func TestAtLiteralInvalidIsNull(t *testing.T) {
	if got := evalStr(t, `@"foo"`, nil); !value.IsNull(got) {
		t.Errorf(`@"foo" = %s, want null`, got)
	}
}
