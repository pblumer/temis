package tck

import "testing"

// TestNumClose covers WP-41.22: numeric comparison at the expected value's stated
// precision. It must accept an over-precise-but-equal result, yet still reject a
// genuine difference at or above the last stated digit, and never relax an
// integer expectation.
func TestNumClose(t *testing.T) {
	cases := []struct {
		got, want string
		close     bool
	}{
		// a decimal128 result that rounds exactly to the expected precision
		{"2778.693549432766768088520383236299", "2778.69354943277", true},
		{"54.59815003314423907811026120286088", "54.59815003", true},
		{"0.3678794411714423215955237701614609", "0.36787944", true},
		// a genuine difference within the stated precision is not close
		{"2861.033777003901636", "2861.03377700389", false},
		{"3.15", "3.14", false},
		// integer expectations are compared exactly (numClose defers to ==)
		{"5.4", "5", false},
		{"5", "5", false}, // == handles this in resultEqual; numClose stays strict
		// non-numbers are never close
		{"foo", "3.14", false},
	}
	for _, c := range cases {
		if got := numClose(c.got, c.want); got != c.close {
			t.Errorf("numClose(%q, %q) = %v, want %v", c.got, c.want, got, c.close)
		}
	}
}

// TestResultEqualStructural confirms resultEqual still compares lists and contexts
// structurally and does not conflate a number with an unequal string.
func TestResultEqualStructural(t *testing.T) {
	if !resultEqual([]any{"1.500000001", "2"}, []any{"1.5", "2"}) {
		t.Error("list of near-equal numbers should match at the stated precision")
	}
	if resultEqual([]any{"1", "2"}, []any{"1", "3"}) {
		t.Error("lists with a real difference must not match")
	}
	if resultEqual(map[string]any{"a": "1"}, map[string]any{"a": "1", "b": "2"}) {
		t.Error("contexts of different size must not match")
	}
	if !resultEqual(map[string]any{"a": "1"}, map[string]any{"a": "1"}) {
		t.Error("identical contexts must match")
	}
}
