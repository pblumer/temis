package builtins

import "github.com/pblumer/temis/internal/value"

func registerBoolean(r *Registry) {
	// not(negand): logical negation; non-boolean yields null.
	r.Register(fixed("not", []string{"negand"}, 1, 1, func(args []value.Value) (value.Value, error) {
		b, ok := args[0].(value.Bool)
		if !ok {
			return value.Null, nil
		}
		return value.BoolOf(!bool(b)), nil
	}))
}
