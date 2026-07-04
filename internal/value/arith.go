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
	case Str:
		if y, ok := b.(Str); ok {
			return x + y // FEEL: string + string concatenates
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
	case Time:
		if y, ok := b.(Time); ok {
			return DaysTimeDuration{d: x.t.Sub(y.t)}
		}
	case Date, DateTime:
		if r, ok := subTemporal(a, b); ok {
			return r
		}
	}
	return shiftOrNull(a, b, -1)
}

// subTemporal implements FEEL subtraction of one date/date-and-time from another,
// yielding the days-and-time duration between their instants. A plain date is the
// UTC start of its day and mixes freely with either kind; two date-and-time values
// must agree on whether they carry a timezone, otherwise the result is null (DMN
// §10.3.2.3.5). It reports ok=false when b is not temporal (a duration), so the
// caller falls through to temporal-minus-duration shifting.
func subTemporal(a, b Value) (Value, bool) {
	at, aok := instantOf(a)
	bt, bok := instantOf(b)
	if !aok || !bok {
		return nil, false
	}
	if temporalZoned(a) != temporalZoned(b) {
		return Null, true
	}
	return DaysTimeDuration{d: at.Sub(bt)}, true
}

// instantOf returns the backing instant of a date or date-and-time value.
func instantOf(v Value) (time.Time, bool) {
	switch x := v.(type) {
	case Date:
		return x.t, true
	case DateTime:
		return x.t, true
	}
	return time.Time{}, false
}

// temporalZoned reports whether a temporal value carries a timezone for the
// purpose of subtraction. A date-and-time may be local (no zone); a plain date is
// anchored to UTC and always counts as zoned, so subtracting a zoned from an
// unzoned date-and-time (or vice versa) yields null.
func temporalZoned(v Value) bool {
	if dt, ok := v.(DateTime); ok {
		return dt.z.zoned
	}
	return true
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
		// DMN: date ± duration stays a date (the time component is dropped). The
		// days-and-time case adds the full duration to the start-of-day instant and
		// truncates back to the resulting calendar day.
		switch d := b.(type) {
		case YearsMonthsDuration:
			return Date{t: addMonths(base.t, int64(sign)*d.months)}
		case DaysTimeDuration:
			shifted := base.t.Add(time.Duration(sign) * d.d)
			return Date{t: time.Date(shifted.Year(), shifted.Month(), shifted.Day(), 0, 0, 0, 0, time.UTC)}
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
