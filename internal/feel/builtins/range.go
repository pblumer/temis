package builtins

import "github.com/pblumer/temis/internal/value"

// This file implements the DMN 1.5 range (interval) relation built-ins. Each
// takes two arguments that may be points (comparable scalars) or ranges, and
// returns a boolean — or null when the operands are incomparable or an endpoint
// needed for the decision is unbounded (a documented limitation: unbounded
// endpoints compare as null, so a relation that depends on one yields null).
//
// The semantics follow the spec's "Semantics of range functions" table. Six
// relations are the mirror image of another and are derived by swapping the
// arguments.

func registerRange(r *Registry) {
	reg := func(name string, fn func(a, b value.Value) value.Value) {
		r.Register(fixed(name, []string{"a", "b"}, 2, 2, func(args []value.Value) (value.Value, error) {
			if value.IsNull(args[0]) || value.IsNull(args[1]) {
				return value.Null, nil
			}
			return fn(args[0], args[1]), nil
		}))
	}

	reg("before", before)
	reg("after", func(a, b value.Value) value.Value { return before(b, a) })
	reg("meets", meets)
	reg("met by", func(a, b value.Value) value.Value { return meets(b, a) })
	reg("overlaps", overlaps)
	reg("overlaps before", overlapsBefore)
	reg("overlaps after", func(a, b value.Value) value.Value { return overlapsBefore(b, a) })
	reg("finishes", finishes)
	reg("finished by", func(a, b value.Value) value.Value { return finishes(b, a) })
	reg("includes", includes)
	reg("during", func(a, b value.Value) value.Value { return includes(b, a) })
	reg("starts", starts)
	reg("started by", func(a, b value.Value) value.Value { return starts(b, a) })
	reg("coincides", coincides)
}

// res carries a boolean together with an ok flag; ok=false propagates to a FEEL
// null. The combinators are strict: any non-ok operand makes the result non-ok.
type res struct{ b, ok bool }

func rtrue(b bool) res { return res{b: b, ok: true} }

func and(parts ...res) res {
	for _, p := range parts {
		if !p.ok {
			return res{}
		}
		if !p.b {
			return rtrue(false)
		}
	}
	return rtrue(true)
}

func or(parts ...res) res {
	for _, p := range parts {
		if !p.ok {
			return res{}
		}
		if p.b {
			return rtrue(true)
		}
	}
	return rtrue(false)
}

func cmpRes(a, b value.Value, want func(int) bool) res {
	c, ok := value.Compare(a, b)
	if !ok {
		return res{}
	}
	return rtrue(want(c))
}

func ltR(a, b value.Value) res { return cmpRes(a, b, func(c int) bool { return c < 0 }) }
func gtR(a, b value.Value) res { return cmpRes(a, b, func(c int) bool { return c > 0 }) }
func eqR(a, b value.Value) res { return cmpRes(a, b, func(c int) bool { return c == 0 }) }

func boolRes(r res) value.Value {
	if !r.ok {
		return value.Null
	}
	return value.BoolOf(r.b)
}

func asRange(v value.Value) (value.Range, bool) {
	r, ok := v.(value.Range)
	return r, ok
}

func before(a, b value.Value) value.Value {
	ra, aR := asRange(a)
	rb, bR := asRange(b)
	switch {
	case !aR && !bR:
		return boolRes(ltR(a, b))
	case !aR && bR:
		return boolRes(or(ltR(a, rb.Low), and(eqR(a, rb.Low), rtrue(!rb.LowClosed))))
	case aR && !bR:
		return boolRes(or(ltR(ra.High, b), and(eqR(ra.High, b), rtrue(!ra.HighClosed))))
	default:
		return boolRes(or(
			ltR(ra.High, rb.Low),
			and(eqR(ra.High, rb.Low), rtrue(!ra.HighClosed || !rb.LowClosed)),
		))
	}
}

func meets(a, b value.Value) value.Value {
	ra, aR := asRange(a)
	rb, bR := asRange(b)
	if !aR || !bR {
		return value.Null
	}
	return boolRes(and(eqR(ra.High, rb.Low), rtrue(ra.HighClosed && rb.LowClosed)))
}

func overlaps(a, b value.Value) value.Value {
	ra, aR := asRange(a)
	rb, bR := asRange(b)
	if !aR || !bR {
		return value.Null
	}
	return boolRes(and(
		or(gtR(ra.High, rb.Low), and(eqR(ra.High, rb.Low), rtrue(ra.HighClosed && rb.LowClosed))),
		or(ltR(ra.Low, rb.High), and(eqR(ra.Low, rb.High), rtrue(ra.LowClosed && rb.HighClosed))),
	))
}

func overlapsBefore(a, b value.Value) value.Value {
	ra, aR := asRange(a)
	rb, bR := asRange(b)
	if !aR || !bR {
		return value.Null
	}
	return boolRes(and(
		or(ltR(ra.Low, rb.Low), and(eqR(ra.Low, rb.Low), rtrue(ra.LowClosed && !rb.LowClosed))),
		or(gtR(ra.High, rb.Low), and(eqR(ra.High, rb.Low), rtrue(ra.HighClosed && rb.LowClosed))),
		or(ltR(ra.High, rb.High), and(eqR(ra.High, rb.High), rtrue(!ra.HighClosed || rb.HighClosed))),
	))
}

func finishes(a, b value.Value) value.Value {
	ra, aR := asRange(a)
	rb, bR := asRange(b)
	switch {
	case !aR && bR: // point finishes range
		return boolRes(and(rtrue(rb.HighClosed), eqR(a, rb.High)))
	case aR && bR:
		return boolRes(and(
			rtrue(ra.HighClosed == rb.HighClosed),
			eqR(ra.High, rb.High),
			or(gtR(ra.Low, rb.Low), and(eqR(ra.Low, rb.Low), rtrue(!ra.LowClosed || rb.LowClosed))),
		))
	default:
		return value.Null
	}
}

func includes(a, b value.Value) value.Value {
	ra, aR := asRange(a)
	rb, bR := asRange(b)
	switch {
	case aR && !bR: // range includes point
		return boolRes(and(
			or(ltR(ra.Low, b), and(eqR(ra.Low, b), rtrue(ra.LowClosed))),
			or(gtR(ra.High, b), and(eqR(ra.High, b), rtrue(ra.HighClosed))),
		))
	case aR && bR:
		return boolRes(and(
			or(ltR(ra.Low, rb.Low), and(eqR(ra.Low, rb.Low), rtrue(ra.LowClosed || !rb.LowClosed))),
			or(gtR(ra.High, rb.High), and(eqR(ra.High, rb.High), rtrue(ra.HighClosed || !rb.HighClosed))),
		))
	default:
		return value.Null
	}
}

func starts(a, b value.Value) value.Value {
	ra, aR := asRange(a)
	rb, bR := asRange(b)
	switch {
	case !aR && bR: // point starts range
		return boolRes(and(eqR(a, rb.Low), rtrue(rb.LowClosed)))
	case aR && bR:
		return boolRes(and(
			eqR(ra.Low, rb.Low),
			rtrue(ra.LowClosed == rb.LowClosed),
			or(ltR(ra.High, rb.High), and(eqR(ra.High, rb.High), rtrue(!ra.HighClosed || rb.HighClosed))),
		))
	default:
		return value.Null
	}
}

func coincides(a, b value.Value) value.Value {
	ra, aR := asRange(a)
	rb, bR := asRange(b)
	switch {
	case !aR && !bR:
		return boolRes(eqR(a, b))
	case aR && bR:
		return boolRes(and(
			eqR(ra.Low, rb.Low),
			rtrue(ra.LowClosed == rb.LowClosed),
			eqR(ra.High, rb.High),
			rtrue(ra.HighClosed == rb.HighClosed),
		))
	default:
		return value.Null
	}
}
