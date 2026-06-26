package builtins

import (
	"regexp"
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

	// substring before/after(string, match): the part of string before/after the
	// first occurrence of match; an empty string when match does not occur.
	r.Register(fixed("substring before", []string{"string", "match"}, 2, 2, substringSide(true)))
	r.Register(fixed("substring after", []string{"string", "match"}, 2, 2, substringSide(false)))

	// matches(input, pattern, flags?): whether input matches the regular
	// expression. replace(input, pattern, replacement, flags?): regex replace.
	// split(string, delimiter): split on a regex into a list of strings.
	//
	// SPEC?: DMN mandates the XPath/XQuery regular-expression flavour; we use Go's
	// RE2 engine, which covers the common subset (character classes, anchors,
	// groups, the i/s/m/x flags, $N back-references in replace) but not XPath-only
	// constructs such as back-references inside the pattern. Documented as a known
	// limitation until a dedicated regex layer lands (tracked with the TCK work).
	r.Register(fixed("matches", []string{"input", "pattern", "flags"}, 2, 3, matches))
	r.Register(fixed("replace", []string{"input", "pattern", "replacement", "flags"}, 3, 4, replace))
	r.Register(fixed("split", []string{"string", "delimiter"}, 2, 2, split))

	// string join(list, delimiter?, prefix?, suffix?): concatenate the string
	// elements of list, skipping nulls, separated by delimiter (default "").
	r.Register(fixed("string join", []string{"list", "delimiter", "prefix", "suffix"}, 1, 4, stringJoin))
}

// substringSide returns a builtin computing the substring before (before=true)
// or after (before=false) the first occurrence of match.
func substringSide(before bool) Func {
	return func(args []value.Value) (value.Value, error) {
		s, ok1 := asString(args[0])
		match, ok2 := asString(args[1])
		if !ok1 || !ok2 {
			return value.Null, nil
		}
		idx := strings.Index(s, match)
		if idx < 0 {
			return value.Str(""), nil
		}
		if before {
			return value.Str(s[:idx]), nil
		}
		return value.Str(s[idx+len(match):]), nil
	}
}

// compileRegex builds a regexp from a FEEL pattern and optional flag string
// (i, s, m, x), mapping the flags to RE2 inline flags. A bad pattern yields nil.
func compileRegex(pattern string, flags string) *regexp.Regexp {
	var inline string
	for _, f := range flags {
		switch f {
		case 'i', 's', 'm':
			inline += string(f)
		case 'x':
			inline += "x"
		default:
			return nil // unknown flag ⇒ null
		}
	}
	if inline != "" {
		pattern = "(?" + inline + ")" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	return re
}

func regexFlags(args []value.Value, idx int) (string, bool) {
	if len(args) <= idx || value.IsNull(args[idx]) {
		return "", true
	}
	return asString(args[idx])
}

func matches(args []value.Value) (value.Value, error) {
	input, ok1 := asString(args[0])
	pattern, ok2 := asString(args[1])
	flags, ok3 := regexFlags(args, 2)
	if !ok1 || !ok2 || !ok3 {
		return value.Null, nil
	}
	re := compileRegex(pattern, flags)
	if re == nil {
		return value.Null, nil
	}
	return value.BoolOf(re.MatchString(input)), nil
}

func replace(args []value.Value) (value.Value, error) {
	input, ok1 := asString(args[0])
	pattern, ok2 := asString(args[1])
	repl, ok3 := asString(args[2])
	flags, ok4 := regexFlags(args, 3)
	if !ok1 || !ok2 || !ok3 || !ok4 {
		return value.Null, nil
	}
	re := compileRegex(pattern, flags)
	if re == nil {
		return value.Null, nil
	}
	return value.Str(re.ReplaceAllString(input, repl)), nil
}

func split(args []value.Value) (value.Value, error) {
	s, ok1 := asString(args[0])
	delim, ok2 := asString(args[1])
	if !ok1 || !ok2 {
		return value.Null, nil
	}
	re := compileRegex(delim, "")
	if re == nil {
		return value.Null, nil
	}
	parts := re.Split(s, -1)
	elems := make([]value.Value, len(parts))
	for i, p := range parts {
		elems[i] = value.Str(p)
	}
	return value.NewList(elems...), nil
}

func stringJoin(args []value.Value) (value.Value, error) {
	elems := listOf(args[:1])
	delim, prefix, suffix := "", "", ""
	if s, ok := optString(args, 1); ok {
		delim = s
	} else if len(args) > 1 && !value.IsNull(args[1]) {
		return value.Null, nil
	}
	if s, ok := optString(args, 2); ok {
		prefix = s
	}
	if s, ok := optString(args, 3); ok {
		suffix = s
	}
	parts := make([]string, 0, len(elems))
	for _, e := range elems {
		if value.IsNull(e) {
			continue // nulls are skipped
		}
		s, ok := asString(e)
		if !ok {
			return value.Null, nil
		}
		parts = append(parts, s)
	}
	return value.Str(prefix + strings.Join(parts, delim) + suffix), nil
}

// optString reads args[idx] as a string when present and non-null.
func optString(args []value.Value, idx int) (string, bool) {
	if len(args) <= idx || value.IsNull(args[idx]) {
		return "", false
	}
	return asString(args[idx])
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
