package feel

import "testing"

func TestForComprehension(t *testing.T) {
	cases := map[string]string{
		"for x in [1, 2, 3] return x * x":           "[1, 4, 9]",
		"for i in 1..3 return i":                    "[1, 2, 3]",
		"for i in 3..1 return i":                    "[3, 2, 1]", // descending
		"for x in [1, 2], y in [10, 20] return x+y": "[11, 21, 12, 22]",
		"for x in [] return x":                      "[]",
		// A later iterator's domain sees the earlier loop variable.
		"for x in [1, 2], y in [x] return y": "[1, 2]",
	}
	for src, want := range cases {
		if got := evalStr(t, src, nil); got.String() != want {
			t.Errorf("%q = %s, want %s", src, got, want)
		}
	}
}

func TestQuantified(t *testing.T) {
	cases := map[string]string{
		"some x in [1, 2, 3] satisfies x > 2":  "true",
		"some x in [1, 2] satisfies x > 5":     "false",
		"every x in [2, 4, 6] satisfies x > 1": "true",
		"every x in [2, 4] satisfies x > 3":    "false",
		"some x in [] satisfies x > 0":         "false",
		"every x in [] satisfies x > 0":        "true",
	}
	for src, want := range cases {
		if got := evalStr(t, src, nil); got.String() != want {
			t.Errorf("%q = %s, want %s", src, got, want)
		}
	}
}

func TestFilterIndex(t *testing.T) {
	cases := map[string]string{
		"[10, 20, 30][1]":  "10",
		"[10, 20, 30][3]":  "30",
		"[10, 20, 30][-1]": "30", // negative counts from the end
		"[10, 20, 30][-3]": "10",
		"[10, 20, 30][5]":  "null", // out of range
		"[10, 20, 30][0]":  "null",
	}
	for src, want := range cases {
		if got := evalStr(t, src, nil); got.String() != want {
			t.Errorf("%q = %s, want %s", src, got, want)
		}
	}
}

func TestFilterPredicate(t *testing.T) {
	cases := map[string]string{
		"[1, 2, 3, 4][item > 2]":                        "[3, 4]",
		"[1, 2, 3, 4][item <= 2]":                       "[1, 2]",
		"[5, 6, 7][item > 100]":                         "[]",
		"[{a: 1}, {a: 5}, {a: 9}][a > 2]":               "[{a: 5}, {a: 9}]", // bare context key
		"[{age: 17}, {age: 20}][item.age >= 18]":        "[{age: 20}]",      // explicit item.key
		"[{n: \"x\", v: 1}, {n: \"y\", v: 2}][v = 2].n": "[y]",              // path projects over the filtered list
	}
	for src, want := range cases {
		if got := evalStr(t, src, nil); got.String() != want {
			t.Errorf("%q = %s, want %s", src, got, want)
		}
	}
}

func TestNestedFilter(t *testing.T) {
	// Inner and outer filters each bind their own item; bare keys resolve to the
	// innermost context that defines them.
	const src = "[[1, 2, 3], [4, 5, 6]][item[item > 4] != []]"
	if got := evalStr(t, src, nil); got.String() != "[[4, 5, 6]]" {
		t.Errorf("%q = %s, want [[4, 5, 6]]", src, got)
	}
}

func TestUnsupportedConstructsReport(t *testing.T) {
	for _, src := range []string{
		"function(x) external x",  // external functions
		"x instance of bogustype", // unknown type in instance of
	} {
		env := NewEnv("x")
		if _, err := CompileString(src, env); err == nil {
			t.Errorf("expected compile error for %q", src)
		}
	}
}
