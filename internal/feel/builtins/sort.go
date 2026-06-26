package builtins

import (
	"sort"

	"github.com/pblumer/temis/internal/value"
)

func registerSort(r *Registry) {
	// sort(list, precedes?): a stably sorted copy of list. With a precedes
	// function f(a, b) → boolean, a sorts before b when f(a, b) is true; without
	// one, elements sort by natural FEEL ordering (ascending).
	r.Register(fixed("sort", []string{"list", "precedes"}, 1, 2, func(args []value.Value) (value.Value, error) {
		l, ok := args[0].(value.List)
		if !ok {
			return value.Null, nil
		}
		out := append([]value.Value{}, l.Elements...)

		if len(args) == 1 {
			sort.SliceStable(out, func(i, j int) bool {
				c, ok := value.Compare(out[i], out[j])
				return ok && c < 0
			})
			return value.NewList(out...), nil
		}

		fn, ok := args[1].(*value.Function)
		if !ok {
			return value.Null, nil
		}
		var callErr error
		sort.SliceStable(out, func(i, j int) bool {
			if callErr != nil {
				return false
			}
			res, err := fn.Call([]value.Value{out[i], out[j]})
			if err != nil {
				callErr = err
				return false
			}
			return res == value.True
		})
		if callErr != nil {
			return value.Null, callErr
		}
		return value.NewList(out...), nil
	}))
}
