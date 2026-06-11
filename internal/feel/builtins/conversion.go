package builtins

import "github.com/pblumer/temis/internal/value"

func registerConversion(r *Registry) {
	// number(from): parse a string into a number; an existing number passes
	// through; anything else yields null.
	r.Register(fixed("number", []string{"from"}, 1, 1, func(args []value.Value) (value.Value, error) {
		switch v := args[0].(type) {
		case value.Number:
			return v, nil
		case value.Str:
			n, err := value.ParseNumber(string(v))
			if err != nil {
				return value.Null, nil
			}
			return n, nil
		default:
			return value.Null, nil
		}
	}))

	// string(from): the FEEL string form of any value; null stays null.
	r.Register(fixed("string", []string{"from"}, 1, 1, func(args []value.Value) (value.Value, error) {
		if value.IsNull(args[0]) {
			return value.Null, nil
		}
		return value.Str(args[0].String()), nil
	}))

	// date(from): parse a date string, or extract the date of a date-and-time.
	r.Register(fixed("date", []string{"from"}, 1, 1, func(args []value.Value) (value.Value, error) {
		switch v := args[0].(type) {
		case value.Date:
			return v, nil
		case value.Str:
			d, err := value.ParseDate(string(v))
			if err != nil {
				return value.Null, nil
			}
			return d, nil
		case value.DateTime:
			t := v.Time()
			return value.NewDate(t.Year(), t.Month(), t.Day()), nil
		default:
			return value.Null, nil
		}
	}))
}
