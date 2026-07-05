package builtins

import "github.com/pblumer/temis/internal/value"

func registerNumeric(r *Registry) {
	// floor/ceiling(n, scale?): round toward -/+ infinity. Scale defaults to 0
	// (round to an integer), matching DMN 1.5's optional scale argument.
	r.Register(fixed("floor", []string{"n", "scale"}, 1, 2, scaled(value.Number.FloorTo)))
	r.Register(fixed("ceiling", []string{"n", "scale"}, 1, 2, scaled(value.Number.CeilingTo)))
	// abs(n): absolute value of a number or a duration (DMN 1.4+ extends abs to
	// both duration types).
	r.Register(fixed("abs", []string{"n"}, 1, 1, func(args []value.Value) (value.Value, error) {
		switch x := args[0].(type) {
		case value.Number:
			return x.Abs(), nil
		case value.DaysTimeDuration:
			if x.Duration() < 0 {
				return value.Neg(x), nil
			}
			return x, nil
		case value.YearsMonthsDuration:
			if x.Months() < 0 {
				return value.Neg(x), nil
			}
			return x, nil
		default:
			return value.Null, nil
		}
	}))

	// decimal and the explicit rounding modes take a mandatory scale.
	r.Register(fixed("decimal", []string{"n", "scale"}, 2, 2, scaled(value.Number.RoundHalfEven)))
	r.Register(fixed("round up", []string{"n", "scale"}, 2, 2, scaled(value.Number.RoundUp)))
	r.Register(fixed("round down", []string{"n", "scale"}, 2, 2, scaled(value.Number.RoundDown)))
	r.Register(fixed("round half up", []string{"n", "scale"}, 2, 2, scaled(value.Number.RoundHalfUp)))
	r.Register(fixed("round half down", []string{"n", "scale"}, 2, 2, scaled(value.Number.RoundHalfDown)))

	// modulo(dividend, divisor): result takes the sign of the divisor.
	r.Register(fixed("modulo", []string{"dividend", "divisor"}, 2, 2, func(args []value.Value) (value.Value, error) {
		a, ok1 := asNumber(args[0])
		b, ok2 := asNumber(args[1])
		if !ok1 || !ok2 {
			return value.Null, nil
		}
		return numOrNull(a.Modulo(b)), nil
	}))

	r.Register(fixed("sqrt", []string{"number"}, 1, 1, numberCalc(value.Number.Sqrt)))
	r.Register(fixed("log", []string{"number"}, 1, 1, numberCalc(value.Number.Ln)))
	r.Register(fixed("exp", []string{"number"}, 1, 1, numberCalc(value.Number.Exp)))

	r.Register(fixed("even", []string{"number"}, 1, 1, parity(value.Number.Even)))
	r.Register(fixed("odd", []string{"number"}, 1, 1, parity(value.Number.Odd)))
}

// numberCalc adapts a Number method that may fail (returning ok=false) into a
// builtin, mapping failure to null.
func numberCalc(f func(value.Number) (value.Number, bool)) Func {
	return func(args []value.Value) (value.Value, error) {
		n, ok := asNumber(args[0])
		if !ok {
			return value.Null, nil
		}
		return numOrNull(f(n)), nil
	}
}

// scaled adapts a Number rounding method taking a scale into a builtin whose
// optional second argument is the scale (default 0).
func scaled(f func(value.Number, int32) (value.Number, bool)) Func {
	return func(args []value.Value) (value.Value, error) {
		n, ok := asNumber(args[0])
		if !ok {
			return value.Null, nil
		}
		scale := 0
		if len(args) >= 2 {
			s, ok := asInt(args[1])
			if !ok {
				return value.Null, nil
			}
			scale = s
		}
		// The DMN round functions require the scale within the decimal128 exponent
		// range [-6111, 6176]; anything outside yields null (TCK 1141–1144).
		if scale < -6111 || scale > 6176 {
			return value.Null, nil
		}
		return numOrNull(f(n, int32(scale))), nil
	}
}

// parity adapts the even/odd predicate (ok=false for non-integers) into a builtin.
func parity(f func(value.Number) (bool, bool)) Func {
	return func(args []value.Value) (value.Value, error) {
		n, ok := asNumber(args[0])
		if !ok {
			return value.Null, nil
		}
		b, ok := f(n)
		if !ok {
			return value.Null, nil
		}
		return value.BoolOf(b), nil
	}
}

// numOrNull maps a (Number, ok) pair to a FEEL value, ok=false ⇒ null.
func numOrNull(n value.Number, ok bool) value.Value {
	if !ok {
		return value.Null
	}
	return n
}
