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
	return value.Range{LowClosed: lowClosed, Low: low, High: high, HighClosed: highClosed}, true
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
	if n, err := value.ParseNumber(s); err == nil {
		return n, true
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
