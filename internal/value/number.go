package value

import (
	"fmt"

	"github.com/cockroachdb/apd/v3"
)

// numberContext is the FEEL arithmetic context: 34 significant digits with
// round-half-even, matching the IEEE 754-2008 decimal128 basis the DMN spec
// references (ADR-0007). Traps are disabled so we inspect condition flags and
// map them to FEEL semantics (e.g. division by zero yields null) ourselves.
var numberContext = apd.Context{
	Precision:   34,
	Rounding:    apd.RoundHalfEven,
	MaxExponent: apd.MaxExponent,
	MinExponent: apd.MinExponent,
	Traps:       0,
}

// Number is a FEEL number backed by an arbitrary-precision decimal.
type Number struct {
	dec *apd.Decimal
}

// Kind returns KindNumber.
func (Number) Kind() Kind { return KindNumber }
func (Number) isValue()   {}

// String renders the number in plain (non-scientific) decimal form, which is
// the canonical FEEL string for typical magnitudes.
func (n Number) String() string { return n.dec.Text('f') }

// Decimal returns the underlying decimal. Callers must not mutate it.
func (n Number) Decimal() *apd.Decimal { return n.dec }

// Cmp compares two numbers as -1, 0 or +1. Integer operands (the common case in
// decision-table range and equality tests) compare natively as int64, skipping
// the decimal alignment apd.Cmp performs; the result is identical.
func (n Number) Cmp(o Number) int {
	if a, ok := n.asInt64(); ok {
		if b, ok := o.asInt64(); ok {
			switch {
			case a < b:
				return -1
			case a > b:
				return 1
			default:
				return 0
			}
		}
	}
	return n.dec.Cmp(o.dec)
}

// IsZero reports whether the number is zero.
func (n Number) IsZero() bool { return n.dec.IsZero() }

// smallInts caches Numbers for the small non-negative integers, which dominate
// real inputs (counts, indices, flags, 0/1) and decision-table constants. A
// Number is immutable by contract — arithmetic always writes a fresh decimal and
// never mutates an operand — so sharing a cached instance is safe. This removes
// the per-conversion apd.Decimal allocation for the common case.
var smallInts [256]Number

func init() {
	for i := range smallInts {
		smallInts[i] = Number{dec: apd.New(int64(i), 0)}
	}
}

// NumberFromInt64 returns a Number for i.
func NumberFromInt64(i int64) Number {
	if i >= 0 && i < int64(len(smallInts)) {
		return smallInts[i]
	}
	return Number{dec: apd.New(i, 0)}
}

// ParseNumber parses a FEEL numeric literal (decimal, optional exponent; no
// hex/octal). It returns an error for malformed or non-finite input.
func ParseNumber(s string) (Number, error) {
	d, _, err := apd.NewFromString(s)
	if err != nil {
		return Number{}, fmt.Errorf("invalid number %q: %w", s, err)
	}
	if d.Form != apd.Finite {
		return Number{}, fmt.Errorf("non-finite number %q", s)
	}
	// Round to the FEEL context precision.
	out := new(apd.Decimal)
	if _, err := numberContext.Round(out, d); err != nil {
		return Number{}, fmt.Errorf("number %q out of range: %w", s, err)
	}
	return reduced(out), nil
}

// reduced normalises a decimal to its canonical FEEL form by stripping
// insignificant trailing zeros (10.0 → 10, 2.500 → 2.5), since FEEL numbers
// carry no notion of significance.
func reduced(d *apd.Decimal) Number {
	d.Reduce(d)
	return Number{dec: d}
}

// MustNumber parses s and panics on error; for tests and constants.
func MustNumber(s string) Number {
	n, err := ParseNumber(s)
	if err != nil {
		panic(err)
	}
	return n
}

// asInt64 reports the number as an exact int64 when it is an integer whose value
// fits int64, reading the decimal's fields directly so it never allocates. A FEEL
// number is reduced, so an integer may carry a positive exponent (100 is stored
// as coefficient 1, exponent 2); the coefficient must fit int64 and scaling by
// 10^exponent must not overflow. It is the gate for the native-integer fast paths
// below: whenever it returns ok for both operands, the native result is exactly
// what the decimal context would compute, so the fast path is a pure speed-up.
func (n Number) asInt64() (int64, bool) {
	d := n.dec
	if d.Form != apd.Finite || d.Exponent < 0 || !d.Coeff.IsInt64() {
		return 0, false
	}
	v := d.Coeff.Int64() // magnitude; apd keeps the sign in d.Negative
	for e := int32(0); e < d.Exponent; e++ {
		if v > maxInt64Div10 {
			return 0, false
		}
		v *= 10
	}
	if d.Negative {
		v = -v
	}
	return v, true
}

const (
	maxInt64      = 1<<63 - 1
	maxInt64Div10 = maxInt64 / 10
)

// The arithmetic operations apply the FEEL decimal context and report whether the
// result is valid (false ⇒ the caller should yield null, e.g. division by zero).
// Each first tries a native int64 path for integer operands — which dominate real
// inputs (counts, sums, indices) — falling back to the decimal context on
// non-integers or overflow. The native result is bit-for-bit the decimal result
// (integer arithmetic is exact), so this only removes work, never changes values;
// small results additionally hit the NumberFromInt64 cache and avoid allocation.

func (n Number) add(o Number) (Number, bool) {
	if a, ok := n.asInt64(); ok {
		if b, ok := o.asInt64(); ok {
			if r := a + b; (r > a) == (b > 0) { // no signed-overflow
				return NumberFromInt64(r), true
			}
		}
	}
	return n.binop(o, numberContext.Add)
}

func (n Number) sub(o Number) (Number, bool) {
	if a, ok := n.asInt64(); ok {
		if b, ok := o.asInt64(); ok {
			if r := a - b; (r < a) == (b > 0) { // no signed-overflow
				return NumberFromInt64(r), true
			}
		}
	}
	return n.binop(o, numberContext.Sub)
}

func (n Number) mul(o Number) (Number, bool) {
	if a, ok := n.asInt64(); ok {
		if b, ok := o.asInt64(); ok {
			if a == 0 || b == 0 {
				return NumberFromInt64(0), true
			}
			if r := a * b; r/b == a { // no signed-overflow
				return NumberFromInt64(r), true
			}
		}
	}
	return n.binop(o, numberContext.Mul)
}

func (n Number) div(o Number) (Number, bool) {
	if o.dec.IsZero() {
		return Number{}, false // FEEL: division by zero is null
	}
	// Only exact integer division short-circuits; an inexact quotient (e.g. 1/3)
	// needs the decimal context to produce FEEL's rounded 34-digit result. Both
	// operands come from asInt64, so neither is math.MinInt64 and a/b cannot
	// overflow.
	if a, ok := n.asInt64(); ok {
		if b, ok := o.asInt64(); ok && b != 0 && a%b == 0 {
			return NumberFromInt64(a / b), true
		}
	}
	return n.binop(o, numberContext.Quo)
}

func (n Number) pow(o Number) (Number, bool) {
	res := new(apd.Decimal)
	cond, err := numberContext.Pow(res, n.dec, o.dec)
	if err != nil || cond.DivisionByZero() || cond.Overflow() || cond.InvalidOperation() {
		return Number{}, false
	}
	return reduced(res), true
}

func (n Number) neg() Number {
	res := new(apd.Decimal)
	res.Neg(n.dec)
	return reduced(res)
}

// Floor returns the greatest integer less than or equal to n.
func (n Number) Floor() Number { return n.roundIntegral(apd.RoundFloor) }

// Ceiling returns the smallest integer greater than or equal to n.
func (n Number) Ceiling() Number { return n.roundIntegral(apd.RoundCeiling) }

// Abs returns the absolute value of n.
func (n Number) Abs() Number {
	res := new(apd.Decimal)
	res.Abs(n.dec)
	return reduced(res)
}

// Int64 returns n truncated to an int64 and whether it fit exactly as an integer.
func (n Number) Int64() (int64, bool) {
	i, err := n.dec.Int64()
	return i, err == nil
}

// SecondsNanos splits a non-negative second count into whole seconds and the
// remaining nanoseconds, for the time()/date and time() constructors, which
// accept a fractional second (e.g. time(12,59,1.3,…) → …01.3…). The fraction is
// rounded half-even to nanosecond precision. ok is false when n is negative or
// its whole-second part does not fit an int64.
func (n Number) SecondsNanos() (sec int64, nanos int, ok bool) {
	if n.dec.Negative {
		return 0, 0, false
	}
	ctx := numberContext // copy so we do not mutate the shared context
	ctx.Rounding = apd.RoundDown
	whole := new(apd.Decimal)
	if _, err := ctx.RoundToIntegralValue(whole, n.dec); err != nil {
		return 0, 0, false
	}
	sec, err := whole.Int64()
	if err != nil {
		return 0, 0, false
	}
	frac := new(apd.Decimal)
	if _, err := ctx.Sub(frac, n.dec, whole); err != nil {
		return 0, 0, false
	}
	scaled := new(apd.Decimal)
	if _, err := ctx.Mul(scaled, frac, apd.New(1_000_000_000, 0)); err != nil {
		return 0, 0, false
	}
	ctx.Rounding = apd.RoundHalfEven
	rounded := new(apd.Decimal)
	if _, err := ctx.RoundToIntegralValue(rounded, scaled); err != nil {
		return 0, 0, false
	}
	nn, err := rounded.Int64()
	if err != nil {
		return 0, 0, false
	}
	return sec, int(nn), true
}

func (n Number) roundIntegral(mode apd.Rounder) Number {
	ctx := numberContext // copy so we don't mutate the shared context
	ctx.Rounding = mode
	res := new(apd.Decimal)
	_, _ = ctx.RoundToIntegralValue(res, n.dec)
	return reduced(res)
}

func (n Number) binop(o Number, op func(d, x, y *apd.Decimal) (apd.Condition, error)) (Number, bool) {
	res := new(apd.Decimal)
	cond, err := op(res, n.dec, o.dec)
	if err != nil || cond.DivisionByZero() || cond.Overflow() || cond.InvalidOperation() {
		return Number{}, false
	}
	return reduced(res), true
}
