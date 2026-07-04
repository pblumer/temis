package feel

import "testing"

// TestInOperator covers the FEEL `in` operator's positive-unary-test right-hand
// side (WP-41, TCK 0072): list membership, ranges, operator-prefixed tests and
// comma-separated test lists.
func TestInOperator(t *testing.T) {
	cases := map[string]string{
		// list membership (values and ranges as elements).
		"1 in [2,3,1]":         "true",
		"1 in [2,3,4]":         "false",
		"1 in [[2..4],[1..3]]": "true",
		"5 in [[2..4],[1..3]]": "false",
		// operator-prefixed unary tests.
		"1 in <= 10":  "true",
		"10 in <= 10": "true",
		"11 in <= 10": "false",
		"11 in > 10":  "true",
		"10 in > 10":  "false",
		// intervals (open/closed endpoints).
		"2 in (2..4)": "false",
		"2 in [2..4)": "true",
		"4 in (2..4]": "true",
		"4 in [2..4)": "false",
		// single value / explicit equality.
		"1 in 1":     "true",
		"1 in 2":     "false",
		"10 in = 10": "true",
		// comma-separated test list with a mix of forms.
		"10 in (1, < 5, >= 10)": "true",
		"7 in (1, < 5, >= 10)":  "false",
		"5 in (1, 5, 9)":        "true",
	}
	for src, want := range cases {
		if got := evalStr(t, src, nil); got.String() != want {
			t.Errorf("%s = %s, want %s", src, got, want)
		}
	}
}
