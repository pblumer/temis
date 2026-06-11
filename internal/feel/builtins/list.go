package builtins

import "github.com/pblumer/temis/internal/value"

func registerList(r *Registry) {
	// count(list): number of elements.
	r.Register(variadic("count", 1, func(args []value.Value) (value.Value, error) {
		return value.NumberFromInt64(int64(len(listOf(args)))), nil
	}))

	// sum(list): sum of numbers; empty or any non-number yields null.
	r.Register(variadic("sum", 1, func(args []value.Value) (value.Value, error) {
		elems := listOf(args)
		if len(elems) == 0 {
			return value.Null, nil
		}
		total := value.Value(value.NumberFromInt64(0))
		for _, e := range elems {
			if _, ok := asNumber(e); !ok {
				return value.Null, nil
			}
			total = value.Add(total, e)
		}
		return total, nil
	}))

	// mean(list): arithmetic mean of numbers; empty yields null.
	r.Register(variadic("mean", 1, func(args []value.Value) (value.Value, error) {
		elems := listOf(args)
		if len(elems) == 0 {
			return value.Null, nil
		}
		total := value.Value(value.NumberFromInt64(0))
		for _, e := range elems {
			if _, ok := asNumber(e); !ok {
				return value.Null, nil
			}
			total = value.Add(total, e)
		}
		return value.Div(total, value.NumberFromInt64(int64(len(elems)))), nil
	}))

	// min(list): least element by FEEL ordering; incomparable elements yield null.
	r.Register(variadic("min", 1, func(args []value.Value) (value.Value, error) {
		return extremum(listOf(args), -1)
	}))

	// max(list): greatest element by FEEL ordering.
	r.Register(variadic("max", 1, func(args []value.Value) (value.Value, error) {
		return extremum(listOf(args), 1)
	}))

	// list contains(list, element): whether the list contains an equal element.
	r.Register(fixed("list contains", []string{"list", "element"}, 2, 2, func(args []value.Value) (value.Value, error) {
		l, ok := args[0].(value.List)
		if !ok {
			return value.Null, nil
		}
		for _, e := range l.Elements {
			if value.Equal(e, args[1]) == value.True {
				return value.True, nil
			}
		}
		return value.False, nil
	}))
}

// extremum returns the element that compares most extreme in the given direction
// (-1 for min, +1 for max). An empty list or any incomparable pair yields null.
func extremum(elems []value.Value, dir int) (value.Value, error) {
	if len(elems) == 0 {
		return value.Null, nil
	}
	best := elems[0]
	for _, e := range elems[1:] {
		cmp, ok := value.Compare(e, best)
		if !ok {
			return value.Null, nil
		}
		if (dir < 0 && cmp < 0) || (dir > 0 && cmp > 0) {
			best = e
		}
	}
	return best, nil
}
