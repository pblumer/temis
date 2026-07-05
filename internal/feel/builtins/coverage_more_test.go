package builtins

import (
	"errors"
	"testing"

	"github.com/pblumer/temis/internal/value"
)

// callRaw invokes a builtin returning both value and error (call() fatals on error).
func callRaw(t *testing.T, name string, args ...value.Value) (value.Value, error) {
	t.Helper()
	b, ok := Default().Lookup(name)
	if !ok {
		t.Fatalf("builtin %q not registered", name)
	}
	return b.Fn(args)
}

// openRange builds a numeric range with an unbounded (null) endpoint on the
// chosen side. low=nil makes the low endpoint unbounded; high=nil the high.
func openRange(low, high value.Value) value.Range {
	return value.Range{LowClosed: false, Low: low, High: high, HighClosed: false}
}

func TestVariadicMethod(t *testing.T) {
	sum, _ := Default().Lookup("sum")
	if !sum.Variadic() {
		t.Error("sum should be variadic")
	}
	floor, _ := Default().Lookup("floor")
	if floor.Variadic() {
		t.Error("floor should not be variadic")
	}
}

func TestAsIntNonInteger(t *testing.T) {
	// scaled() goes through asInt on its scale argument; a non-integer number
	// (fractional) makes Int64 fail, exercising asInt's ok=false branch.
	run(t, []tc{
		{name: "decimal", args: []value.Value{num("3.14"), num("1.5")}, wantNull: true},
		{name: "floor", args: []value.Value{num("3.14"), num("2.5")}, wantNull: true},
	})
}

func TestNumericNonNumberArgs(t *testing.T) {
	run(t, []tc{
		// numberMap (abs), numberCalc (sqrt/log/exp), scaled (decimal/floor),
		// parity (even/odd): non-number first arg → null.
		{name: "abs", args: []value.Value{str("x")}, wantNull: true},
		{name: "sqrt", args: []value.Value{str("x")}, wantNull: true},
		{name: "log", args: []value.Value{str("x")}, wantNull: true},
		{name: "exp", args: []value.Value{str("x")}, wantNull: true},
		{name: "exp", args: []value.Value{num("1")}, want: "2.718281828459045235360287471352662"},
		{name: "decimal", args: []value.Value{str("x"), num("2")}, wantNull: true},
		{name: "even", args: []value.Value{str("x")}, wantNull: true},
		{name: "odd", args: []value.Value{str("x")}, wantNull: true},
		// modulo with non-number args
		{name: "modulo", args: []value.Value{str("x"), num("2")}, wantNull: true},
		{name: "modulo", args: []value.Value{num("2"), str("x")}, wantNull: true},
	})
}

func TestConversionNumberNonStringDefault(t *testing.T) {
	run(t, []tc{
		// number(boolean) hits the default branch → null.
		{name: "number", args: []value.Value{value.True}, wantNull: true},
		{name: "number", args: []value.Value{list(num("1"))}, wantNull: true},
	})
}

func TestMinMaxIncomparable(t *testing.T) {
	run(t, []tc{
		// mixed types are incomparable → null (extremum ok=false branch).
		{name: "min", args: []value.Value{list(num("1"), str("a"))}, wantNull: true},
		{name: "max", args: []value.Value{list(num("1"), str("a"))}, wantNull: true},
	})
}

func TestSortNonFunctionAndError(t *testing.T) {
	l := list(num("3"), num("1"), num("2"))
	// second argument is not a function → null.
	if got := call(t, "sort", l, num("5")); !value.IsNull(got) {
		t.Errorf("sort(list, non-function) = %s, want null", got)
	}

	// a precedes function that errors propagates the error and yields null.
	boom := &value.Function{Name: "boom", Arity: 2, Call: func(args []value.Value) (value.Value, error) {
		return value.Null, errors.New("boom")
	}}
	got, err := callRaw(t, "sort", l, boom)
	if err == nil {
		t.Error("sort with erroring precedes should return an error")
	}
	if !value.IsNull(got) {
		t.Errorf("sort with erroring precedes = %s, want null", got)
	}
}

func TestContextEdgeBranches(t *testing.T) {
	m := ctx(str("a"), num("1"))

	// get value: key list where a step is not a string → null.
	if got := call(t, "get value", m, value.NewList(num("1"))); !value.IsNull(got) {
		t.Errorf("get value with non-string step = %s, want null", got)
	}
	// get value: key list that descends into a non-context → null.
	nested := ctx(str("x"), m)
	if got := call(t, "get value", nested, value.NewList(str("x"), str("a"), str("b"))); !value.IsNull(got) {
		t.Errorf("get value descending into non-context = %s, want null", got)
	}
	// get value: key list with a missing key → null.
	if got := call(t, "get value", m, value.NewList(str("z"))); !value.IsNull(got) {
		t.Errorf("get value missing key in list = %s, want null", got)
	}
	// get value: key neither string nor list → null (default branch).
	if got := call(t, "get value", m, num("1")); !value.IsNull(got) {
		t.Errorf("get value with numeric key = %s, want null", got)
	}

	// get entries on a non-context → null.
	if got := call(t, "get entries", num("1")); !value.IsNull(got) {
		t.Errorf("get entries(non-context) = %s, want null", got)
	}

	// context put: non-context and non-string key → null.
	if got := call(t, "context put", num("1"), str("k"), num("2")); !value.IsNull(got) {
		t.Errorf("context put(non-context) = %s, want null", got)
	}
	if got := call(t, "context put", m, num("1"), num("2")); !value.IsNull(got) {
		t.Errorf("context put(non-string key) = %s, want null", got)
	}

	// context merge: a non-context element → null.
	if got := call(t, "context merge", value.NewList(m, num("1"))); !value.IsNull(got) {
		t.Errorf("context merge(non-context) = %s, want null", got)
	}

	// context(): non-list, entry not a context, entry missing key/value, non-string key.
	if got := call(t, "context", num("1")); !value.IsNull(got) {
		t.Errorf("context(non-list) = %s, want null", got)
	}
	if got := call(t, "context", value.NewList(num("1"))); !value.IsNull(got) {
		t.Errorf("context(list of non-context) = %s, want null", got)
	}
	noKey := value.NewContext().Put("value", num("1"))
	if got := call(t, "context", value.NewList(noKey)); !value.IsNull(got) {
		t.Errorf("context(entry missing key) = %s, want null", got)
	}
	noVal := value.NewContext().Put("key", str("a"))
	if got := call(t, "context", value.NewList(noVal)); !value.IsNull(got) {
		t.Errorf("context(entry missing value) = %s, want null", got)
	}
	badKey := value.NewContext().Put("key", num("1")).Put("value", num("2"))
	if got := call(t, "context", value.NewList(badKey)); !value.IsNull(got) {
		t.Errorf("context(entry non-string key) = %s, want null", got)
	}
}

func TestListMoreEdgeBranches(t *testing.T) {
	l123 := list(num("1"), num("2"), num("3"))
	run(t, []tc{
		// non-list first args → null
		{name: "sublist", args: []value.Value{num("1"), num("1")}, wantNull: true},
		{name: "insert before", args: []value.Value{num("1"), num("1"), num("9")}, wantNull: true},
		{name: "remove", args: []value.Value{num("1"), num("1")}, wantNull: true},
		{name: "reverse", args: []value.Value{num("1")}, wantNull: true},
		{name: "index of", args: []value.Value{num("1"), num("1")}, wantNull: true},
		{name: "distinct values", args: []value.Value{num("1")}, wantNull: true},
		{name: "flatten", args: []value.Value{num("1")}, wantNull: true},
		{name: "concatenate", args: []value.Value{list(num("1")), num("2")}, wantNull: true},
		{name: "union", args: []value.Value{list(num("1")), num("2")}, wantNull: true},

		// sublist: bad start (non-int), length non-int, out of range, negative length
		{name: "sublist", args: []value.Value{l123, str("x")}, wantNull: true},
		{name: "sublist", args: []value.Value{l123, num("4")}, want: "[]"},
		{name: "sublist", args: []value.Value{l123, num("9")}, wantNull: true},
		{name: "sublist", args: []value.Value{l123, num("-9")}, wantNull: true},
		{name: "sublist", args: []value.Value{l123, num("1"), str("x")}, wantNull: true},
		{name: "sublist", args: []value.Value{l123, num("1"), num("-1")}, wantNull: true},
		{name: "sublist", args: []value.Value{l123, num("2"), num("9")}, want: "[2, 3]"},

		// insert before / remove: non-int position
		{name: "insert before", args: []value.Value{l123, str("x"), num("9")}, wantNull: true},
		{name: "remove", args: []value.Value{l123, str("x")}, wantNull: true},
		{name: "insert before", args: []value.Value{l123, num("4"), num("9")}, want: "[1, 2, 3, 9]"},

		// stddev / median / mode / product: non-number elements → null / empty
		{name: "stddev", args: []value.Value{list(num("1"), str("x"))}, wantNull: true},
		{name: "median", args: []value.Value{list(num("1"), str("x"))}, wantNull: true},
	})

	// mode with non-comparable (mixed) values still returns a list (no sort panic).
	got := call(t, "mode", list(num("1"), str("a"), num("1")))
	if value.IsNull(got) {
		t.Errorf("mode(mixed) = null, want list")
	}
}

func TestStringEdgeBranches(t *testing.T) {
	run(t, []tc{
		// compileRegex: unknown flag → null; the 'x','s','m' flags are accepted.
		{name: "matches", args: []value.Value{str("a"), str("a"), str("q")}, wantNull: true},
		// the 'x' (extended) flag strips insignificant whitespace from the pattern,
		// so "a b" becomes "ab" and matches "ab" (WP-41.18).
		{name: "matches", args: []value.Value{str("ab"), str("a b"), str("x")}, want: "true"},
		{name: "matches", args: []value.Value{str("a\nb"), str("a.b"), str("s")}, want: "true"},
		{name: "matches", args: []value.Value{str("a\nb"), str("^b$"), str("m")}, want: "true"},
		// matches: non-string pattern → null
		{name: "matches", args: []value.Value{str("a"), num("1")}, wantNull: true},
		// matches: non-string flags → null
		{name: "matches", args: []value.Value{str("a"), str("a"), num("1")}, wantNull: true},

		// replace: non-string args and bad regex / unknown flag
		{name: "replace", args: []value.Value{num("1"), str("a"), str("b")}, wantNull: true},
		{name: "replace", args: []value.Value{str("a"), num("1"), str("b")}, wantNull: true},
		{name: "replace", args: []value.Value{str("a"), str("a"), num("1")}, wantNull: true},
		{name: "replace", args: []value.Value{str("a"), str("a"), str("b"), num("1")}, wantNull: true},
		{name: "replace", args: []value.Value{str("a"), str("a"), str("b"), str("q")}, wantNull: true},

		// split: non-string delimiter and bad regex
		{name: "split", args: []value.Value{str("a"), num("1")}, wantNull: true},
		{name: "split", args: []value.Value{str("a"), str("(")}, wantNull: true},

		// substring: non-int start, start 0, length non-int, negative length,
		// out-of-range start, length running past the end.
		{name: "substring", args: []value.Value{str("foobar"), str("x")}, wantNull: true},
		{name: "substring", args: []value.Value{str("foobar"), num("0")}, wantNull: true},
		{name: "substring", args: []value.Value{str("foobar"), num("9")}, wantNull: true},
		{name: "substring", args: []value.Value{str("foobar"), num("-9")}, wantNull: true},
		{name: "substring", args: []value.Value{str("foobar"), num("2"), str("x")}, wantNull: true},
		{name: "substring", args: []value.Value{str("foobar"), num("2"), num("-1")}, wantNull: true},
		{name: "substring", args: []value.Value{str("foobar"), num("2"), num("99")}, want: "oobar"},
		{name: "substring", args: []value.Value{str("foobar"), num("2"), value.Null}, want: "oobar"},

		// string join: delimiter present but non-string (and non-null) → null;
		// prefix/suffix supplied.
		{name: "string join", args: []value.Value{list(str("a"), str("b")), num("1")}, wantNull: true},
		{name: "string join", args: []value.Value{list(str("a"), str("b")), str("-"), str("<"), str(">")}, want: "<a-b>"},
		// string join: non-list first arg treated as the single element.
		{name: "string join", args: []value.Value{str("a"), str("-")}, want: "a"},
	})
}

func TestTemporalRemainingBranches(t *testing.T) {
	d := mustDate("2024-02-29")
	tm := value.NewTime(13, 30, 0, 0, nil)
	run(t, []tc{
		// timeFn: arity 2 falls through to default → null.
		{name: "time", args: []value.Value{num("1"), num("2")}, wantNull: true},
		// dateAndTimeFn: too many args (3) → default null; case 2 first non-Date.
		{name: "date and time", args: []value.Value{d, tm, d}, wantNull: true},
		{name: "date and time", args: []value.Value{num("1"), tm}, wantNull: true},
		{name: "date and time", args: []value.Value{value.True}, wantNull: true},
		// dateInstant Date branch + dateString/dateNumber on a plain Date.
		{name: "day of week", args: []value.Value{d}, want: "Thursday"},
		{name: "month of year", args: []value.Value{d}, want: "February"},
		{name: "week of year", args: []value.Value{d}, want: "9"},
		// dateInstant: string that fails to parse as a date → null.
		{name: "day of week", args: []value.Value{str("not-a-date")}, wantNull: true},
		// time(...) with too many args is rejected by arity, so feed the 4-arg
		// path with a null offset to cover offset==nil.
		{name: "time", args: []value.Value{num("9"), num("0"), num("0"), value.Null}, want: "09:00:00"},
		// timeFn: a string that fails to parse → null.
		{name: "time", args: []value.Value{str("not-a-time")}, wantNull: true},
		// mean: a non-number element → null.
		{name: "mean", args: []value.Value{list(num("1"), str("x"))}, wantNull: true},
	})
}

func TestRangeUnboundedAndIncomparable(t *testing.T) {
	closed := func(lo, hi string) value.Range { return rng(true, lo, hi, true) }
	run(t, []tc{
		// meets / overlaps / overlapsBefore require ranges on both sides; a point
		// makes them null (the !aR || !bR branches).
		{name: "overlaps", args: []value.Value{num("1"), num("2")}, wantNull: true},
		{name: "overlaps before", args: []value.Value{num("1"), num("2")}, wantNull: true},
		{name: "overlaps after", args: []value.Value{num("1"), num("2")}, wantNull: true},
		{name: "met by", args: []value.Value{num("1"), num("2")}, wantNull: true},

		// finishes / includes / starts / coincides default (point-point or
		// range-point in the wrong slot) → null.
		{name: "finishes", args: []value.Value{closed("1", "5"), num("5")}, wantNull: true},
		{name: "includes", args: []value.Value{num("5"), num("5")}, wantNull: true},
		{name: "starts", args: []value.Value{closed("1", "5"), num("1")}, wantNull: true},
		{name: "coincides", args: []value.Value{num("5"), closed("1", "5")}, wantNull: true},

		// range-range variants of finishes/includes/starts (the aR && bR branch).
		{name: "finishes", args: []value.Value{closed("3", "10"), closed("1", "10")}, want: "true"},
		{name: "finishes", args: []value.Value{closed("3", "10"), rng(true, "3", "10", false)}, want: "false"},
		{name: "includes", args: []value.Value{closed("1", "10"), closed("3", "8")}, want: "true"},
		{name: "includes", args: []value.Value{closed("1", "10"), closed("3", "20")}, want: "false"},
		{name: "starts", args: []value.Value{closed("1", "5"), closed("1", "10")}, want: "true"},
		{name: "starts", args: []value.Value{rng(false, "1", "5", true), closed("1", "10")}, want: "false"},

		// coincides false on a high-endpoint difference (range-range branch).
		{name: "coincides", args: []value.Value{closed("1", "5"), closed("1", "6")}, want: "false"},
		{name: "coincides", args: []value.Value{closed("1", "5"), rng(true, "1", "5", false)}, want: "false"},

		// incomparable point operands → null via cmpRes ok=false.
		{name: "before", args: []value.Value{num("1"), str("a")}, wantNull: true},
		{name: "coincides", args: []value.Value{num("1"), str("a")}, wantNull: true},
	})

	// Unbounded endpoints make and()/or() propagate non-ok → null.
	openLow := openRange(nil, num("10")) // (-inf, 10)
	openHigh := openRange(num("1"), nil) // (1, +inf)
	if got := call(t, "before", num("5"), openLow); !value.IsNull(got) {
		t.Errorf("before(point, range with unbounded low) = %s, want null", got)
	}
	if got := call(t, "before", openHigh, num("5")); !value.IsNull(got) {
		t.Errorf("before(range with unbounded high, point) = %s, want null", got)
	}
	if got := call(t, "before", openHigh, openLow); !value.IsNull(got) {
		t.Errorf("before(range, range) with unbounded endpoints = %s, want null", got)
	}
}
