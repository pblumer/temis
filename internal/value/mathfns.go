package value

import "github.com/cockroachdb/apd/v3"

// The functions here back the FEEL numeric built-ins (WP-21). They live in the
// value package so internal/feel/builtins never has to import apd directly; each
// applies the shared FEEL decimal context and reports ok=false where FEEL maps
// the result to null (e.g. sqrt of a negative number, log of a non-positive
// number, modulo by zero).

// Sqrt returns the square root of n. A negative operand yields ok=false.
func (n Number) Sqrt() (Number, bool) {
	if n.dec.Negative && !n.dec.IsZero() {
		return Number{}, false
	}
	return n.unary(numberContext.Sqrt)
}

// Ln returns the natural logarithm of n. A non-positive operand yields ok=false.
func (n Number) Ln() (Number, bool) {
	if n.dec.Negative || n.dec.IsZero() {
		return Number{}, false
	}
	return n.unary(numberContext.Ln)
}

// Exp returns e raised to the power n.
func (n Number) Exp() (Number, bool) { return n.unary(numberContext.Exp) }

// Modulo returns the FEEL modulo of n by o, defined as n - o*floor(n/o) so the
// result takes the sign of the divisor. A zero divisor yields ok=false.
func (n Number) Modulo(o Number) (Number, bool) {
	if o.dec.IsZero() {
		return Number{}, false
	}
	q := new(apd.Decimal)
	if _, err := numberContext.Quo(q, n.dec, o.dec); err != nil {
		return Number{}, false
	}
	floorCtx := numberContext
	floorCtx.Rounding = apd.RoundFloor
	if _, err := floorCtx.RoundToIntegralValue(q, q); err != nil {
		return Number{}, false
	}
	prod := new(apd.Decimal)
	if _, err := numberContext.Mul(prod, o.dec, q); err != nil {
		return Number{}, false
	}
	res := new(apd.Decimal)
	if _, err := numberContext.Sub(res, n.dec, prod); err != nil {
		return Number{}, false
	}
	return reduced(res), true
}

// IsInteger reports whether n has no fractional part.
func (n Number) IsInteger() bool {
	res := new(apd.Decimal)
	if _, err := numberContext.RoundToIntegralValue(res, n.dec); err != nil {
		return false
	}
	return res.Cmp(n.dec) == 0
}

// Even reports whether n is an even integer. A non-integer yields ok=false.
func (n Number) Even() (bool, bool) { return n.parity(true) }

// Odd reports whether n is an odd integer. A non-integer yields ok=false.
func (n Number) Odd() (bool, bool) { return n.parity(false) }

func (n Number) parity(wantEven bool) (bool, bool) {
	if !n.IsInteger() {
		return false, false
	}
	rem, ok := n.Modulo(NumberFromInt64(2))
	if !ok {
		return false, false
	}
	isEven := rem.IsZero()
	if wantEven {
		return isEven, true
	}
	return !isEven, true
}

// RoundHalfEven rounds n to scale digits after the decimal point using
// round-half-even, matching the FEEL decimal(n, scale) built-in.
func (n Number) RoundHalfEven(scale int32) (Number, bool) {
	return n.quantize(scale, apd.RoundHalfEven)
}

// RoundUp rounds n to scale digits, away from zero on ties and otherwise toward
// the next magnitude (FEEL "round up").
func (n Number) RoundUp(scale int32) (Number, bool) { return n.quantize(scale, apd.RoundUp) }

// RoundDown rounds n to scale digits toward zero (FEEL "round down", truncation).
func (n Number) RoundDown(scale int32) (Number, bool) { return n.quantize(scale, apd.RoundDown) }

// RoundHalfUp rounds n to scale digits, ties away from zero (FEEL "round half up").
func (n Number) RoundHalfUp(scale int32) (Number, bool) { return n.quantize(scale, apd.RoundHalfUp) }

// RoundHalfDown rounds n to scale digits, ties toward zero (FEEL "round half down").
func (n Number) RoundHalfDown(scale int32) (Number, bool) {
	return n.quantize(scale, apd.RoundHalfDown)
}

// FloorTo rounds n down (toward -infinity) to scale digits after the decimal
// point, matching FEEL floor(n, scale).
func (n Number) FloorTo(scale int32) (Number, bool) { return n.quantize(scale, apd.RoundFloor) }

// CeilingTo rounds n up (toward +infinity) to scale digits after the decimal
// point, matching FEEL ceiling(n, scale).
func (n Number) CeilingTo(scale int32) (Number, bool) { return n.quantize(scale, apd.RoundCeiling) }

func (n Number) quantize(scale int32, mode apd.Rounder) (Number, bool) {
	// Quantizing to more fractional digits than n already carries cannot change
	// its value, so short-circuit: this both avoids needless work and keeps a very
	// large (but valid) scale from overflowing the 34-digit context — e.g.
	// round up(5.5, 6176) is just 5.5 (TCK 1141–1144).
	var fracDigits int32
	if n.dec.Exponent < 0 {
		fracDigits = -n.dec.Exponent
	}
	if scale >= fracDigits {
		return n, true
	}
	ctx := numberContext // copy; never mutate the shared context
	ctx.Rounding = mode
	res := new(apd.Decimal)
	if _, err := ctx.Quantize(res, n.dec, -scale); err != nil {
		return Number{}, false
	}
	return reduced(res), true
}

func (n Number) unary(op func(d, x *apd.Decimal) (apd.Condition, error)) (Number, bool) {
	res := new(apd.Decimal)
	cond, err := op(res, n.dec)
	if err != nil || cond.DivisionByZero() || cond.Overflow() || cond.InvalidOperation() {
		return Number{}, false
	}
	if res.Form != apd.Finite {
		return Number{}, false
	}
	return reduced(res), true
}
