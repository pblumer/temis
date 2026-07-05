package builtins

import "github.com/pblumer/temis/internal/value"

// registerListReplaceAndIs adds three DMN-1.5 builtins the TCK exercises
// (WP-41): `list replace`, `is` and the multi-argument form of `number`.
func registerListReplaceAndIs(r *Registry) {
	// list replace(list, position, newItem): replace the 1-indexed element (a
	// negative position counts from the end). list replace(list, match, newItem):
	// replace every element for which match(item, newItem) is true (DMN 1.5).
	r.Register(overloaded("list replace", []string{"list", "position", "newItem"}, [][]string{{"list", "match", "newItem"}}, 3, 3, func(args []value.Value) (value.Value, error) {
		// The list argument coerces a singleton scalar to a one-element list; null
		// stays null.
		var elems []value.Value
		switch a := args[0].(type) {
		case value.List:
			elems = a.Elements
		default:
			if value.IsNull(a) {
				return value.Null, nil
			}
			elems = []value.Value{a}
		}
		out := append([]value.Value{}, elems...)
		switch sel := args[1].(type) {
		case value.Number:
			// A non-integer position truncates toward zero (2.5 → 2, -1.5 → -1).
			n, ok := sel.RoundDown(0)
			if !ok {
				return value.Null, nil
			}
			i, ok := n.Int64()
			if !ok {
				return value.Null, nil
			}
			idx := int(i)
			if idx < 0 {
				idx = len(out) + idx + 1
			}
			if idx < 1 || idx > len(out) {
				return value.Null, nil
			}
			out[idx-1] = args[2]
		case *value.Function:
			// The match predicate is match(item, newItem); a different arity, or a
			// non-boolean result, makes the call undefined (null).
			if sel.Arity != 2 {
				return value.Null, nil
			}
			for i := range out {
				res, err := sel.Call([]value.Value{out[i], args[2]})
				if err != nil {
					return value.Null, err
				}
				b, ok := res.(value.Bool)
				if !ok {
					return value.Null, nil
				}
				if bool(b) {
					out[i] = args[2]
				}
			}
		default:
			return value.Null, nil
		}
		return value.NewList(out...), nil
	}))

	// is(value1, value2): true when the two are the same value AND the same type
	// (stricter than `=`, which coerces). null is only `is` null.
	r.Register(fixed("is", []string{"value1", "value2"}, 2, 2, func(args []value.Value) (value.Value, error) {
		a, b := args[0], args[1]
		an, bn := value.IsNull(a), value.IsNull(b)
		if an || bn {
			return value.BoolOf(an && bn), nil
		}
		if a.Kind() != b.Kind() {
			return value.False, nil
		}
		// For date/time/date-and-time, `is` requires an identical representation,
		// not merely the same instant: @"10:00:00+01:00" and @"09:00:00Z" are the
		// same instant but are not `is` (they render differently).
		switch a.Kind() {
		case value.KindDate, value.KindTime, value.KindDateTime:
			return value.BoolOf(a.String() == b.String()), nil
		}
		return value.BoolOf(value.Equal(a, b) == value.True), nil
	}))
}
