package value

import (
	"testing"

	"github.com/cockroachdb/apd/v3"
)

// The native int64 fast paths in add/sub/mul/div/Cmp must return exactly what the
// decimal context would. These tests pin that equivalence at the boundaries that
// matter: overflow (must fall back), inexact division (must fall back), values
// carrying a positive exponent (reduced multiples of ten), negatives and zero.

// decVia forces the pure decimal path for the same op, so a test can assert the
// fast path agrees with apd bit-for-bit at the value level.
func decVia(t *testing.T, op func(d, x, y *apd.Decimal) (apd.Condition, error), a, b Number) (Number, bool) {
	t.Helper()
	res := new(apd.Decimal)
	cond, err := op(res, a.dec, b.dec)
	if err != nil || cond.DivisionByZero() || cond.Overflow() || cond.InvalidOperation() {
		return Number{}, false
	}
	res.Reduce(res)
	return Number{dec: res}, true
}

func mustNum(t *testing.T, s string) Number {
	t.Helper()
	n, err := ParseNumber(s)
	if err != nil {
		t.Fatalf("ParseNumber(%q): %v", s, err)
	}
	return n
}

func TestInt64FastPathAgreesWithDecimal(t *testing.T) {
	cases := []struct{ a, b string }{
		{"6", "7"}, {"42", "3"}, {"0", "0"}, {"0", "5"}, {"5", "0"},
		{"-6", "7"}, {"6", "-7"}, {"-6", "-7"},
		{"100", "10"}, {"1000", "40"}, {"120", "3"}, // reduced positive exponents
		{"45", "2"}, {"1", "3"}, {"7", "2"}, {"10", "4"}, // (in)exact division
		{"9223372036854775807", "1"},               // max int64
		{"9223372036854775807", "2"},               // add/mul overflow → fallback
		{"-9223372036854775807", "-1"},             // near min
		{"1000000000000", "1000000000000"},         // mul overflow → fallback
		{"3.5", "2"}, {"2", "0.5"}, {"1.1", "2.2"}, // non-integers → fallback
	}
	for _, c := range cases {
		a, b := mustNum(t, c.a), mustNum(t, c.b)

		// add
		got, gok := a.add(b)
		want, wok := decVia(t, numberContext.Add, a, b)
		assertSame(t, "add", c.a, c.b, got, gok, want, wok)

		// sub
		got, gok = a.sub(b)
		want, wok = decVia(t, numberContext.Sub, a, b)
		assertSame(t, "sub", c.a, c.b, got, gok, want, wok)

		// mul
		got, gok = a.mul(b)
		want, wok = decVia(t, numberContext.Mul, a, b)
		assertSame(t, "mul", c.a, c.b, got, gok, want, wok)

		// div (skip divide-by-zero, which both report invalid differently)
		if !b.IsZero() {
			got, gok = a.div(b)
			want, wok = decVia(t, numberContext.Quo, a, b)
			assertSame(t, "div", c.a, c.b, got, gok, want, wok)
		}

		// Cmp
		if a.Cmp(b) != a.dec.Cmp(b.dec) {
			t.Errorf("Cmp(%s,%s)=%d, decimal=%d", c.a, c.b, a.Cmp(b), a.dec.Cmp(b.dec))
		}
	}
}

func assertSame(t *testing.T, op, a, b string, got Number, gok bool, want Number, wok bool) {
	t.Helper()
	if gok != wok {
		t.Errorf("%s(%s,%s): ok=%v, decimal ok=%v", op, a, b, gok, wok)
		return
	}
	if !gok {
		return
	}
	if got.Cmp(want) != 0 {
		t.Errorf("%s(%s,%s)=%s, decimal=%s", op, a, b, got.String(), want.String())
	}
	if got.String() != want.String() {
		t.Errorf("%s(%s,%s) string=%q, decimal string=%q", op, a, b, got.String(), want.String())
	}
}

// TestAsInt64 pins the exact-integer detection, including reduced positive
// exponents and rejection of fractional or out-of-range values.
func TestAsInt64(t *testing.T) {
	ok := map[string]int64{
		"0": 0, "5": 5, "-5": -5, "255": 255, "256": 256,
		"100": 100, "1000": 1000, "9223372036854775807": 1<<63 - 1,
		"-9223372036854775807": -(1<<63 - 1),
	}
	for s, want := range ok {
		if got, isInt := mustNum(t, s).asInt64(); !isInt || got != want {
			t.Errorf("asInt64(%q)=(%d,%v), want (%d,true)", s, got, isInt, want)
		}
	}
	notInt := []string{"3.5", "0.1", "-2.5", "9223372036854775808", "1e30"}
	for _, s := range notInt {
		if got, isInt := mustNum(t, s).asInt64(); isInt {
			t.Errorf("asInt64(%q)=(%d,true), want ok=false", s, got)
		}
	}
}
