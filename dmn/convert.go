package dmn

import (
	"fmt"
	"strconv"
	"time"

	"github.com/pblumer/feel/value"
)

// inputToValues converts an Input map into FEEL values keyed by variable name,
// without a declared schema to consult. Kept for the callers that have none (a
// standalone FEEL expression has no model to declare anything).
func inputToValues(in Input) (map[string]value.Value, error) {
	return inputToValuesTyped(in, nil)
}

// inputToValuesTyped is inputToValues with the model's own declarations in hand
// (ADR-0040).
//
// JSON has no date, so a caller over HTTP or MCP can only send one as text. Left
// to toValue that text becomes a FEEL *string*, and a decision table whose column
// is declared `date` and whose cell reads `< date("2026-01-01")` then matches
// nothing — the catch-all rule answers, with no diagnostic and no trace entry to
// read. The model said which type it wanted; this is where that is honoured.
//
// It is deliberately **not** FEEL coercion (DMN §10.3.2.9.4, coerceToType), which
// keeps a conforming value or makes it null. This is the Go-value-to-FEEL-value
// mapping one line further out — the boundary this engine defines, and which DMN
// leaves to the implementation. FEEL's own refusal to convert a string to a date
// inside an expression is untouched.
//
// A value the declared type cannot be made from is passed through unchanged
// rather than nulled: ValidateInput is what reports it, and an evaluation that
// quietly replaced it with null would be the failure this record exists to end.
func inputToValuesTyped(in Input, fields []InputField) (map[string]value.Value, error) {
	declared := make(map[string]string, len(fields))
	for _, f := range fields {
		if f.Type != "" {
			declared[f.Name] = f.Type
		}
	}
	vals := make(map[string]value.Value, len(in))
	for k, v := range in {
		if fv, ok := declaredValue(v, declared[k]); ok {
			vals[k] = fv
			continue
		}
		fv, err := toValue(v)
		if err != nil {
			return nil, fmt.Errorf("dmn: input %q: %w", k, err)
		}
		vals[k] = fv
	}
	return vals, nil
}

// declaredValue builds the FEEL value a declared temporal type asks for out of the
// text a JSON caller can send. It reports false for anything it does not handle —
// a value that is not a string, a type that is not temporal, or text the type
// cannot be made from — and the caller falls back to toValue's type-driven
// mapping, so nothing is lost and nothing is guessed.
//
// One spelling per type, and only ISO 8601: the formats FEEL's own date(), time(),
// date and time() and duration() literals accept. A locale-dependent spelling like
// `dd.MM.yyyy` is deliberately not accepted — an engine that guesses which of
// 03.04.2026 and 04.03.2026 was meant is the class of silent wrong answer this
// whole record is about.
func declaredValue(v any, declared string) (value.Value, bool) {
	s, ok := v.(string)
	if !ok || s == "" {
		return nil, false
	}
	var (
		fv  value.Value
		err error
	)
	switch declared {
	case "date":
		var d value.Date
		d, err = value.ParseDate(s)
		fv = d
	case "time":
		var t value.Time
		t, err = value.ParseTime(s)
		fv = t
	case "date and time":
		var dt value.DateTime
		dt, err = value.ParseDateTime(s)
		fv = dt
	case "duration":
		fv, err = value.ParseDuration(s)
	default:
		return nil, false
	}
	if err != nil {
		return nil, false
	}
	return fv, true
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
