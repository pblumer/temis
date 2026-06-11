package builtins

import (
	"strings"

	"github.com/pblumer/temis/internal/value"
)

func registerString(r *Registry) {
	// string length(string): number of characters (runes).
	r.Register(fixed("string length", []string{"string"}, 1, 1, func(args []value.Value) (value.Value, error) {
		s, ok := asString(args[0])
		if !ok {
			return value.Null, nil
		}
		return value.NumberFromInt64(int64(len([]rune(s)))), nil
	}))

	// upper case / lower case.
	r.Register(fixed("upper case", []string{"string"}, 1, 1, stringMap(strings.ToUpper)))
	r.Register(fixed("lower case", []string{"string"}, 1, 1, stringMap(strings.ToLower)))

	// contains(string, match), starts with(string, match), ends with(string, match).
	r.Register(fixed("contains", []string{"string", "match"}, 2, 2, stringPred(strings.Contains)))
	r.Register(fixed("starts with", []string{"string", "match"}, 2, 2, stringPred(strings.HasPrefix)))
	r.Register(fixed("ends with", []string{"string", "match"}, 2, 2, stringPred(strings.HasSuffix)))

	// substring(string, start position, length?): 1-indexed; negative start
	// counts from the end; an omitted or null length runs to the end.
	r.Register(fixed("substring", []string{"string", "start position", "length"}, 2, 3, substring))
}

func stringMap(f func(string) string) Func {
	return func(args []value.Value) (value.Value, error) {
		s, ok := asString(args[0])
		if !ok {
			return value.Null, nil
		}
		return value.Str(f(s)), nil
	}
}

func stringPred(f func(s, sub string) bool) Func {
	return func(args []value.Value) (value.Value, error) {
		s, ok1 := asString(args[0])
		sub, ok2 := asString(args[1])
		if !ok1 || !ok2 {
			return value.Null, nil
		}
		return value.BoolOf(f(s, sub)), nil
	}
}

func substring(args []value.Value) (value.Value, error) {
	s, ok := asString(args[0])
	if !ok {
		return value.Null, nil
	}
	start, ok := asInt(args[1])
	if !ok || start == 0 {
		return value.Null, nil
	}
	runes := []rune(s)
	n := len(runes)

	var begin int
	if start < 0 {
		begin = n + start // -1 is the last character
	} else {
		begin = start - 1 // 1-indexed
	}
	if begin < 0 || begin > n {
		return value.Null, nil
	}

	end := n
	if len(args) >= 3 && !value.IsNull(args[2]) {
		length, ok := asInt(args[2])
		if !ok || length < 0 {
			return value.Null, nil
		}
		end = begin + length
		if end > n {
			end = n
		}
	}
	return value.Str(string(runes[begin:end])), nil
}
