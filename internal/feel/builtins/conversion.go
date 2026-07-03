package builtins

import (
	"strings"

	"github.com/pblumer/temis/internal/value"
)

func registerConversion(r *Registry) {
	// number(from, grouping separator?, decimal separator?): parse a string into a
	// number (DMN 1.5). The grouping separator (space, comma or period) is stripped
	// and the decimal separator (period or comma) normalised to "." before parsing.
	// An existing number passes through; anything unparseable yields null.
	r.Register(fixed("number", []string{"from", "grouping separator", "decimal separator"}, 1, 3, func(args []value.Value) (value.Value, error) {
		switch v := args[0].(type) {
		case value.Number:
			return v, nil
		case value.Str:
			str := string(v)
			if len(args) >= 2 {
				if g, ok := args[1].(value.Str); ok && g != "" {
					str = strings.ReplaceAll(str, string(g), "")
				}
			}
			if len(args) >= 3 {
				if d, ok := args[2].(value.Str); ok && d != "" && string(d) != "." {
					str = strings.ReplaceAll(str, string(d), ".")
				}
			}
			n, err := value.ParseNumber(str)
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

	// The temporal conversions date/time/date and time/duration live in
	// temporal.go (registerTemporal) alongside the other date/time builtins.
}
