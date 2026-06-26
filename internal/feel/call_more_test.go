package feel

import "testing"

// TestEvalNewBuiltinsEndToEnd exercises the WP-21 builtins through the full
// parse→compile→evaluate pipeline, confirming the parser assembles their
// multi-word names via the registry oracle.
func TestEvalNewBuiltinsEndToEnd(t *testing.T) {
	cases := map[string]string{
		`decimal(3.14159, 2)`:               "3.14",
		`round half up(5.5, 0)`:             "6",
		`modulo(12, 5)`:                     "2",
		`sqrt(16)`:                          "4",
		`even(2)`:                           "true",
		`odd(3)`:                            "true",
		`string join(["a", "b", "c"], "-")`: "a-b-c",
		`substring before("foobar", "bar")`: "foo",
		`matches("foobar", "^fo*bar$")`:     "true",
		`distinct values([1, 2, 1, 3])`:     "[1, 2, 3]",
		`index of([1, 2, 1], 1)`:            "[1, 3]",
		`sublist([1, 2, 3, 4], 2, 2)`:       "[2, 3]",
		`reverse([1, 2, 3])`:                "[3, 2, 1]",
		`flatten([1, [2, [3]]])`:            "[1, 2, 3]",
		`product([2, 3, 4])`:                "24",
		`median([3, 1, 2])`:                 "2",
		`sort([3, 1, 2])`:                   "[1, 2, 3]",
		`before(1, 10)`:                     "true",
		`includes([1..10], 5)`:              "true",
		`during(5, [1..10])`:                "true",
		`overlaps([1..5], [3..8])`:          "true",
		`get value({a: 1, b: 2}, "b")`:      "2",
		`get entries({a: 1})[1].key`:        "a",
		`context put({a: 1}, "b", 2).b`:     "2",
		`all([true, true])`:                 "true",
		`any([false, true])`:                "true",
	}
	for src, want := range cases {
		if got := evalStr(t, src, nil); got.String() != want {
			t.Errorf("%q = %s, want %s", src, got, want)
		}
	}
}
