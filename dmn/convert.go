package dmn

import (
	"fmt"
	"strconv"
	"time"

	"github.com/pblumer/feel/value"
)

// inputToValues converts an Input map into FEEL values keyed by variable name.
func inputToValues(in Input) (map[string]value.Value, error) {
	vals := make(map[string]value.Value, len(in))
	for k, v := range in {
		fv, err := toValue(v)
		if err != nil {
			return nil, fmt.Errorf("dmn: input %q: %w", k, err)
		}
		vals[k] = fv
	}
	return vals, nil
}

// toValue converts a Go value into a FEEL value (see Evaluate for the mapping).
// An unsupported type is an error rather than a silent null.
func toValue(v any) (value.Value, error) {
	switch x := v.(type) {
	case nil:
		return value.Null, nil
	case value.Value:
		return x, nil
	case bool:
		return value.BoolOf(x), nil
	case string:
		return value.Str(x), nil
	case int:
		return value.NumberFromInt64(int64(x)), nil
	case int8:
		return value.NumberFromInt64(int64(x)), nil
	case int16:
		return value.NumberFromInt64(int64(x)), nil
	case int32:
		return value.NumberFromInt64(int64(x)), nil
	case int64:
		return value.NumberFromInt64(x), nil
	case uint8:
		return value.NumberFromInt64(int64(x)), nil
	case uint16:
		return value.NumberFromInt64(int64(x)), nil
	case uint32:
		return value.NumberFromInt64(int64(x)), nil
	case uint:
		return value.ParseNumber(strconv.FormatUint(uint64(x), 10))
	case uint64:
		return value.ParseNumber(strconv.FormatUint(x, 10))
	case float32:
		return value.ParseNumber(strconv.FormatFloat(float64(x), 'g', -1, 32))
	case float64:
		return value.ParseNumber(strconv.FormatFloat(x, 'g', -1, 64))
	case time.Time:
		return value.NewDateTime(x), nil
	case []any:
		elems := make([]value.Value, len(x))
		for i, e := range x {
			ev, err := toValue(e)
			if err != nil {
				return nil, err
			}
			elems[i] = ev
		}
		return value.NewList(elems...), nil
	case map[string]any:
		ctx := value.NewContext()
		for key, e := range x {
			ev, err := toValue(e)
			if err != nil {
				return nil, err
			}
			ctx.Put(key, ev)
		}
		return ctx, nil
	default:
		return nil, fmt.Errorf("unsupported input type %T", v)
	}
}

// fromValue converts a FEEL value back into a Go value (see Evaluate). Numbers
// render as their exact decimal string; null becomes nil.
func fromValue(v value.Value) any {
	if value.IsNull(v) {
		return nil
	}
	switch x := v.(type) {
	case value.Bool:
		return bool(x)
	case value.Str:
		return string(x)
	case value.Number:
		return x.String()
	case value.List:
		out := make([]any, len(x.Elements))
		for i, e := range x.Elements {
			out[i] = fromValue(e)
		}
		return out
	case *value.Context:
		out := make(map[string]any, x.Len())
		for _, k := range x.Keys() {
			ev, _ := x.Get(k)
			out[k] = fromValue(ev)
		}
		return out
	default:
		// Temporal values, ranges and functions render in canonical FEEL form.
		return x.String()
	}
}
