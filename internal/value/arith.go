package value

import (
	"time"

	"github.com/cockroachdb/apd/v3"
)

// The arithmetic functions implement FEEL's binary operators over values. They
// propagate null: if an operand is null, or the operand kinds have no defined
// operation, the result is null (docs/30-feel-spec.md §10). Number results that
// overflow or divide by zero also yield null.

// Add implements `+`: numbers, like-typed durations, and date/time ± duration.
func Add(a, b Value) Value {
	if IsNull(a) || IsNull(b) {
		return Null
	}
	switch x := a.(type) {
	case Number:
		if y, ok := b.(Number); ok {
			return numOrNull(x.add(y))
		}
	case DaysTimeDuration:
		if y, ok := b.(DaysTimeDuration); ok {
			return DaysTimeDuration{d: x.d + y.d}
		}
	case YearsMonthsDuration:
		if y, ok := b.(YearsMonthsDuration); ok {
			return YearsMonthsDuration{months: x.months + y.months}
		}
	}
	// date/time/date-time + duration is commutative.
	if r := shift(a, b, +1); r != nil {
		return r
	}
	return shiftOrNull(b, a, +1)
}

// Sub implements `-`: numbers, like-typed durations, temporal differences and
// date/time minus duration.
func Sub(a, b Value) Value {
	if IsNull(a) || IsNull(b) {
		return Null
	}
	switch x := a.(type) {
	case Number:
		if y, ok := b.(Number); ok {
			return numOrNull(x.sub(y))
		}
	case DaysTimeDuration:
		if y, ok := b.(DaysTimeDuration); ok {
			return DaysTimeDuration{d: x.d - y.d}
		}
	case YearsMonthsDuration:
		if y, ok := b.(YearsMonthsDuration); ok {
			return YearsMonthsDuration{months: x.months - y.months}
		}
	case Date:
		if y, ok := b.(Date); ok {
			return DaysTimeDuration{d: x.t.Sub(y.t)}
		}
	case Time:
		if y, ok := b.(Time); ok {
			return DaysTimeDuration{d: x.t.Sub(y.t)}
		}
	case DateTime:
		if y, ok := b.(DateTime); ok {
			return DaysTimeDuration{d: x.t.Sub(y.t)}
		}
	}
	return shiftOrNull(a, b, -1)
}

// Mul implements `*`: number*number and duration*number (either order).
func Mul(a, b Value) Value {
	if IsNull(a) || IsNull(b) {
		return Null
	}
	if x, ok := a.(Number); ok {
		if y, ok := b.(Number); ok {
			return numOrNull(x.mul(y))
		}
		return scaleDuration(b, x, false)
	}
	if y, ok := b.(Number); ok {
		return scaleDuration(a, y, false)
	}
	return Null
}

// Div implements `/`: number/number, duration/number and duration/duration.
func Div(a, b Value) Value {
	if IsNull(a) || IsNull(b) {
		return Null
	}
	switch x := a.(type) {
	case Number:
		if y, ok := b.(Number); ok {
			return numOrNull(x.div(y))
		}
	case DaysTimeDuration:
		switch y := b.(type) {
		case Number:
			return scaleDuration(x, y, true)
		case DaysTimeDuration:
			if y.d == 0 {
				return Null
			}
			return numOrNull(NumberFromInt64(int64(x.d)).div(NumberFromInt64(int64(y.d))))
		}
	case YearsMonthsDuration:
		switch y := b.(type) {
		case Number:
			return scaleDuration(x, y, true)
		case YearsMonthsDuration:
			if y.months == 0 {
				return Null
			}
			return numOrNull(NumberFromInt64(x.months).div(NumberFromInt64(y.months)))
		}
	}
	return Null
}

// Exp implements `**` for numbers.
func Exp(a, b Value) Value {
	if IsNull(a) || IsNull(b) {
		return Null
	}
	x, ok := a.(Number)
	if !ok {
		return Null
	}
	y, ok := b.(Number)
	if !ok {
		return Null
	}
	return numOrNull(x.pow(y))
}

// Neg implements unary `-` for numbers and durations.
func Neg(a Value) Value {
	switch x := a.(type) {
	case Number:
		return x.neg()
	case DaysTimeDuration:
		return DaysTimeDuration{d: -x.d}
	case YearsMonthsDuration:
		return YearsMonthsDuration{months: -x.months}
	default:
		return Null
	}
}

// scaleDuration multiplies or divides a duration by a number, rounding to the
// duration's integral unit (nanoseconds or months).
func scaleDuration(dur Value, n Number, divide bool) Value {
	if divide && n.IsZero() {
		return Null
	}
	switch d := dur.(type) {
	case DaysTimeDuration:
		ns, ok := scaleInt64(int64(d.d), n, divide)
		if !ok {
			return Null
		}
		return DaysTimeDuration{d: time.Duration(ns)}
	case YearsMonthsDuration:
		m, ok := scaleInt64(d.months, n, divide)
		if !ok {
			return Null
		}
		return YearsMonthsDuration{months: m}
	default:
		return Null
	}
}

// scaleInt64 computes round(base * n) or round(base / n) as an int64 under the
// FEEL decimal context, rounding half-even to an integer.
func scaleInt64(base int64, n Number, divide bool) (int64, bool) {
	res := new(apd.Decimal)
	var cond apd.Condition
	var err error
	if divide {
		cond, err = numberContext.Quo(res, apd.New(base, 0), n.dec)
	} else {
		cond, err = numberContext.Mul(res, apd.New(base, 0), n.dec)
	}
	if err != nil || cond.DivisionByZero() || cond.Overflow() {
		return 0, false
	}
	rounded := new(apd.Decimal)
	if _, err := numberContext.RoundToIntegralValue(rounded, res); err != nil {
		return 0, false
	}
	i, err := rounded.Int64()
	if err != nil {
		return 0, false
	}
	return i, true
}

// shift applies a duration (b) to a temporal value (a) with the given sign,
// returning nil when a is not temporal or b is not a duration.
func shift(a, b Value, sign int) Value {
	switch base := a.(type) {
	case Date:
		switch d := b.(type) {
		case YearsMonthsDuration:
			return Date{t: addMonths(base.t, int64(sign)*d.months)}
		case DaysTimeDuration:
			return DateTime{t: base.t.Add(time.Duration(sign) * d.d)}
		}
	case DateTime:
		switch d := b.(type) {
		case YearsMonthsDuration:
			return DateTime{t: addMonths(base.t, int64(sign)*d.months), z: base.z}
		case DaysTimeDuration:
			return DateTime{t: base.t.Add(time.Duration(sign) * d.d), z: base.z}
		}
	case Time:
		if d, ok := b.(DaysTimeDuration); ok {
			return Time{t: base.t.Add(time.Duration(sign) * d.d), z: base.z}
		}
	}
	return nil
}

func shiftOrNull(a, b Value, sign int) Value {
	if r := shift(a, b, sign); r != nil {
		return r
	}
	return Null
}

func numOrNull(n Number, ok bool) Value {
	if !ok {
		return Null
	}
	return n
}
