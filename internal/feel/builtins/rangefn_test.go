package builtins

import (
	"testing"

	"github.com/pblumer/temis/internal/value"
)

// TestRangeFunction covers the DMN-1.5 range(from) builtin (WP-41, TCK 1156):
// bracket variants, unbounded ends, and string/temporal endpoints.
func TestRangeFunction(t *testing.T) {
	rng := func(src string) value.Range {
		t.Helper()
		v := call(t, "range", value.Str(src))
		r, ok := v.(value.Range)
		if !ok {
			t.Fatalf("range(%q) = %s, want a range", src, v)
		}
		return r
	}

	r := rng("[1..3]")
	if !r.LowClosed || !r.HighClosed || r.Low.String() != "1" || r.High.String() != "3" {
		t.Errorf("[1..3] = %v", r)
	}
	if r := rng("(18..21]"); r.LowClosed || !r.HighClosed {
		t.Errorf("(18..21] brackets wrong: %v", r)
	}
	if r := rng("[18..21["); !r.LowClosed || r.HighClosed {
		t.Errorf("[18..21[ brackets wrong: %v", r)
	}
	// Unbounded ends must use an open bracket on the unbounded side.
	if r := rng("[1..)"); !value.IsNull(r.High) || r.Low.String() != "1" {
		t.Errorf("[1..) should be low-bounded only: %v", r)
	}
	if r := rng("(..2]"); !value.IsNull(r.Low) || r.High.String() != "2" {
		t.Errorf("(..2] should be high-bounded only: %v", r)
	}
	// A closed bracket on an unbounded side, mismatched endpoint types, or a
	// reversed range are all invalid (null).
	for _, bad := range []string{"[1..]", "[..2]", `[1.."b"]`, "[3..1]", `["z".."a"]`} {
		if got := call(t, "range", value.Str(bad)); !value.IsNull(got) {
			t.Errorf("range(%q) = %s, want null", bad, got)
		}
	}
	// String and temporal endpoints.
	if r := rng(`["a".."c"]`); r.Low.Kind() != value.KindString || r.High.String() != "c" {
		t.Errorf(`["a".."c"] endpoints wrong: %v`, r)
	}
	if r := rng(`[@"1970-01-01"..@"1970-01-02"]`); r.Low.Kind() != value.KindDate {
		t.Errorf("date range low kind = %v", r.Low.Kind())
	}
	// Membership through a parsed range.
	if got := call(t, "range", value.Str("[1..3]")); !matchesRange(got, value.MustNumber("2")) {
		t.Error("2 should be in range [1..3]")
	}

	// Malformed input yields null.
	if v := call(t, "range", value.Str("nonsense")); !value.IsNull(v) {
		t.Errorf("range(nonsense) = %s, want null", v)
	}
}

func matchesRange(r value.Value, x value.Value) bool {
	rng, ok := r.(value.Range)
	if !ok {
		return false
	}
	loOK := value.IsNull(rng.Low)
	if !loOK {
		if c, ok := value.Compare(x, rng.Low); ok {
			loOK = c > 0 || (c == 0 && rng.LowClosed)
		}
	}
	hiOK := value.IsNull(rng.High)
	if !hiOK {
		if c, ok := value.Compare(x, rng.High); ok {
			hiOK = c < 0 || (c == 0 && rng.HighClosed)
		}
	}
	return loOK && hiOK
}
