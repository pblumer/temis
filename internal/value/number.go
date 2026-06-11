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

// Cmp compares two numbers as -1, 0 or +1.
func (n Number) Cmp(o Number) int { return n.dec.Cmp(o.dec) }

// IsZero reports whether the number is zero.
func (n Number) IsZero() bool { return n.dec.IsZero() }

// NumberFromInt64 returns a Number for i.
func NumberFromInt64(i int64) Number {
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

// arithmetic operations apply the FEEL context and report whether the result is
// valid (false ⇒ the caller should yield null, e.g. division by zero).

func (n Number) add(o Number) (Number, bool) { return n.binop(o, numberContext.Add) }
func (n Number) sub(o Number) (Number, bool) { return n.binop(o, numberContext.Sub) }
func (n Number) mul(o Number) (Number, bool) { return n.binop(o, numberContext.Mul) }

func (n Number) div(o Number) (Number, bool) {
	if o.dec.IsZero() {
		return Number{}, false // FEEL: division by zero is null
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

func (n Number) binop(o Number, op func(d, x, y *apd.Decimal) (apd.Condition, error)) (Number, bool) {
	res := new(apd.Decimal)
	cond, err := op(res, n.dec, o.dec)
	if err != nil || cond.DivisionByZero() || cond.Overflow() || cond.InvalidOperation() {
		return Number{}, false
	}
	return reduced(res), true
}
