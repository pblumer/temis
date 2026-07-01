package flow

import "testing"

func TestCoerce(t *testing.T) {
	cases := []struct {
		name     string
		in       any
		feelType string
		want     any
	}{
		// dmn renders numbers as decimal strings; a number-typed target must get a
		// number back, exact for integers.
		{"integer string to int64", "30", "number", int64(30)},
		{"negative integer string", "-7", "number", int64(-7)},
		{"decimal string to float", "3.5", "number", 3.5},
		// Non-numeric targets and non-string values pass through untouched.
		{"string target keeps string", "low", "string", "low"},
		{"untyped keeps string", "30", "", "30"},
		{"already numeric passes", 42, "number", 42},
		{"non-numeric string for number target", "n/a", "number", "n/a"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := coerce(tc.in, tc.feelType); got != tc.want {
				t.Fatalf("coerce(%v, %q) = %v (%T), want %v (%T)", tc.in, tc.feelType, got, got, tc.want, tc.want)
			}
		})
	}
}
