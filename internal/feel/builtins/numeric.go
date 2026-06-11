package builtins

import "github.com/pblumer/temis/internal/value"

func registerNumeric(r *Registry) {
	r.Register(fixed("floor", []string{"n"}, 1, 1, numberMap(value.Number.Floor)))
	r.Register(fixed("ceiling", []string{"n"}, 1, 1, numberMap(value.Number.Ceiling)))
	r.Register(fixed("abs", []string{"n"}, 1, 1, numberMap(value.Number.Abs)))
}

func numberMap(f func(value.Number) value.Number) Func {
	return func(args []value.Value) (value.Value, error) {
		n, ok := asNumber(args[0])
		if !ok {
			return value.Null, nil
		}
		return f(n), nil
	}
}
