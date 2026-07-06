package builtins

import (
	"strings"

	"github.com/pblumer/temis/internal/value"
)

// registerRangeFn adds the DMN-1.5 `range(from)` builtin (TCK 1156): it parses a
// range string such as "[1..10]", "(18..21]", "]18..21[", "[1..]" or
// "[@\"1970-01-01\"..@\"1970-01-02\"]" into a range value. Endpoints may be
// numbers, quoted strings, @-prefixed temporal literals, or empty (unbounded).
func registerRangeFn(r *Registry) {
	r.Register(fixed("range", []string{"from"}, 1, 1, func(args []value.Value) (value.Value, error) {
		s, ok := args[0].(value.Str)
		if !ok {
			return value.Null, nil
		}
		rng, ok := parseRangeString(string(s))
		if !ok {
			return value.Null, nil
		}
		return rng, nil
	}))
}

func parseRangeString(s string) (value.Value, bool) {
	s = strings.TrimSpace(s)
	if len(s) < 4 {
		return value.Null, false
	}
	var lowClosed, highClosed bool
	switch s[0] {
	case '[':
		lowClosed = true
	case '(', ']':
		lowClosed = false
	default:
		return value.Null, false
	}
	switch s[len(s)-1] {
	case ']':
		highClosed = true
	case ')', '[':
		highClosed = false
	default:
		return value.Null, false
	}
	inner := s[1 : len(s)-1]
	i := strings.Index(inner, "..")
	if i < 0 {
		return value.Null, false
	}
	low, ok1 := parseRangeEndpoint(inner[:i])
	high, ok2 := parseRangeEndpoint(inner[i+2:])
	if !ok1 || !ok2 {
		return value.Null, false
	}
	if !validRangeBounds(low, high, lowClosed, highClosed) {
		return value.Null, false
	}
	return value.Range{LowClosed: lowClosed, Low: low, High: high, HighClosed: highClosed}, true
}

// validRangeBounds rejects the malformed ranges the DMN range() constructor must
// turn into null: an unbounded endpoint marked as closed, endpoints of different
// types, and a low bound that exceeds the high bound.
func validRangeBounds(low, high value.Value, lowClosed, highClosed bool) bool {
	lowUnbounded, highUnbounded := value.IsNull(low), value.IsNull(high)
	if (lowUnbounded && lowClosed) || (highUnbounded && highClosed) {
		return false
	}
	if lowUnbounded || highUnbounded {
		return true
	}
	if low.Kind() != high.Kind() {
		return false
	}
	cmp, ok := value.Compare(low, high)
	return ok && cmp <= 0
}

// parseRangeEndpoint parses one range endpoint. An empty endpoint is an
// unbounded end (null). Quoted strings, @-temporal literals and numbers are
// supported.
func parseRangeEndpoint(s string) (value.Value, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return value.Null, true
	}
	if strings.HasPrefix(s, "@\"") && strings.HasSuffix(s, "\"") {
		return parseTemporalLiteral(s[2 : len(s)-1])
	}
	if strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"") && len(s) >= 2 {
		return value.Str(s[1 : len(s)-1]), true
	}
	if v, ok := parseConstructorEndpoint(s); ok {
		return v, true
	}
	if n, err := value.ParseNumber(s); err == nil {
		return n, true
	}
	return value.Null, false
}

// parseConstructorEndpoint resolves a temporal-constructor endpoint written as a
// call over a quoted string — date("1970-01-01"), time("00:00:00"),
// date and time("…T…") or duration("P1D") — which the TCK 1156 range() cases use
// interchangeably with @"…" literals.
func parseConstructorEndpoint(s string) (value.Value, bool) {
	open := strings.IndexByte(s, '(')
	if open < 0 || !strings.HasSuffix(s, ")") {
		return value.Null, false
	}
	arg := strings.TrimSpace(s[open+1 : len(s)-1])
	if len(arg) < 2 || arg[0] != '"' || arg[len(arg)-1] != '"' {
		return value.Null, false
	}
	inner := arg[1 : len(arg)-1]
	switch strings.TrimSpace(s[:open]) {
	case "date":
		if d, err := value.ParseDate(inner); err == nil {
			return d, true
		}
	case "time":
		if t, err := value.ParseTime(inner); err == nil {
			return t, true
		}
	case "date and time":
		if dt, err := value.ParseDateTime(inner); err == nil {
			return dt, true
		}
	case "duration":
		if d, err := value.ParseDuration(inner); err == nil {
			return d, true
		}
	}
	return value.Null, false
}

// parseTemporalLiteral resolves a date, date-time, time or duration literal (the
// content of an @"…" literal) by trying each parser in a disambiguating order.
func parseTemporalLiteral(s string) (value.Value, bool) {
	if strings.Contains(s, "T") {
		if dt, err := value.ParseDateTime(s); err == nil {
			return dt, true
		}
	}
	if strings.HasPrefix(s, "P") || strings.HasPrefix(s, "-P") {
		if d, err := value.ParseDuration(s); err == nil {
			return d, true
		}
	}
	if strings.Contains(s, "-") && !strings.Contains(s, ":") {
		if d, err := value.ParseDate(s); err == nil {
			return d, true
		}
	}
	if strings.Contains(s, ":") {
		if tm, err := value.ParseTime(s); err == nil {
			return tm, true
		}
	}
	// Last resort: try every parser.
	if d, err := value.ParseDate(s); err == nil {
		return d, true
	}
	if dt, err := value.ParseDateTime(s); err == nil {
		return dt, true
	}
	if tm, err := value.ParseTime(s); err == nil {
		return tm, true
	}
	if d, err := value.ParseDuration(s); err == nil {
		return d, true
	}
	return value.Null, false
}
