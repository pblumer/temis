package builtins

import (
	"testing"

	"github.com/pblumer/temis/internal/value"
)

// TestNumberSeparatorValidation covers WP-41.18: number() validates its optional
// grouping/decimal separators — invalid, equal, or non-string separators make the
// call null, even when the first argument is already a number (TCK 0058).
func TestNumberSeparatorValidation(t *testing.T) {
	run(t, []tc{
		{name: "number", args: []value.Value{str("1,000,000.01"), str(","), str(".")}, want: "1000000.01"},
		{name: "number", args: []value.Value{str("1.000.000,01"), str("."), str(",")}, want: "1000000.01"},
		// invalid decimal separator ":"
		{name: "number", args: []value.Value{str("1,000,000.01"), str(","), str(":")}, wantNull: true},
		// non-string decimal separator
		{name: "number", args: []value.Value{str("1,000,000.01"), str(","), num("123")}, wantNull: true},
		// grouping == decimal
		{name: "number", args: []value.Value{str("1,000,000.00"), str(","), str(",")}, wantNull: true},
		// equal separators reject even a numeric first argument
		{name: "number", args: []value.Value{num("123"), str("."), str(".")}, wantNull: true},
	})
}

// TestRangeConstructorEndpoints covers WP-41.18: range() parses temporal
// constructor-call endpoints (date("…"), time("…"), date and time("…"),
// duration("…")) the same as @"…" literals (TCK 1156).
func TestRangeConstructorEndpoints(t *testing.T) {
	cases := []struct{ src, lo, hi string }{
		{`[date("1970-01-01")..date("1970-01-02")]`, "1970-01-01", "1970-01-02"},
		{`[time("00:00:00")..@"12:00:00"]`, "00:00:00", "12:00:00"},
		{`[duration("P1D")..@"P2D"]`, "P1DT0H0M0S", "P2DT0H0M0S"},
	}
	for _, c := range cases {
		v := call(t, "range", str(c.src))
		r, ok := v.(value.Range)
		if !ok {
			t.Errorf("range(%q) = %s, want a range", c.src, v)
			continue
		}
		if r.Low.String() != c.lo || r.High.String() != c.hi {
			t.Errorf("range(%q) = [%s..%s], want [%s..%s]", c.src, r.Low, r.High, c.lo, c.hi)
		}
	}
}

// TestReplaceGroupAndExtendedFlag covers WP-41.18: replace() maps $N references
// to Go's ${N} form (so "$1c" is group 1 then a literal "c"), and the x flag
// strips insignificant whitespace from the pattern (TCK 1109).
func TestReplaceRegexFixes(t *testing.T) {
	run(t, []tc{
		{name: "replace", args: []value.Value{str("darted"), str("^(.*?)d(.*)$"), str("$1c$2")}, want: "carted"},
		{name: "replace", args: []value.Value{str("0123456789"), str(`(\d{3})(\d{3})(\d{4})`), str("($1) $2-$3")}, want: "(012) 345-6789"},
		{name: "replace", args: []value.Value{str("a b"), str("[a-z]"), str("#"), str("x")}, want: "# #"},
		{name: "matches", args: []value.Value{str("012"), str(`\d{3}`)}, want: "true"},
	})
}
