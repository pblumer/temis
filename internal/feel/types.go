package feel

import (
	"strings"

	"github.com/pblumer/temis/internal/value"
)

// Type is a FEEL type used by the static type checker (WP-30) and the `instance
// of` operator. A nil *Type means Any — an unknown or unconstrained type that
// the checker never flags and that conforms to (and from) every type.
//
// Concrete scalar types are the package singletons (TNumber, TString, …). A List
// carries its element type in Elem (nil = list of Any); a Context carries its
// known field types in Fields.
type Type struct {
	Kind   value.Kind
	Elem   *Type            // element type for a list (nil = unknown element)
	Fields map[string]*Type // field types for a context (nil = open/unknown)
}

// Built-in scalar type singletons. nil is used directly for Any.
var (
	TNumber              = &Type{Kind: value.KindNumber}
	TString              = &Type{Kind: value.KindString}
	TBoolean             = &Type{Kind: value.KindBool}
	TDate                = &Type{Kind: value.KindDate}
	TTime                = &Type{Kind: value.KindTime}
	TDateTime            = &Type{Kind: value.KindDateTime}
	TDaysTimeDuration    = &Type{Kind: value.KindDaysTimeDuration}
	TYearsMonthsDuration = &Type{Kind: value.KindYearsMonthsDuration}
	TNull                = &Type{Kind: value.KindNull}
)

// ListOf returns the list type with the given element type (nil elem = list of
// Any).
func ListOf(elem *Type) *Type { return &Type{Kind: value.KindList, Elem: elem} }

// ContextOf returns a context type with the given field types.
func ContextOf(fields map[string]*Type) *Type {
	return &Type{Kind: value.KindContext, Fields: fields}
}

// String renders the type in canonical FEEL form.
func (t *Type) String() string {
	if t == nil {
		return "Any"
	}
	switch t.Kind {
	case value.KindList:
		if t.Elem != nil {
			return "list<" + t.Elem.String() + ">"
		}
		return "list"
	case value.KindContext:
		return "context"
	default:
		return t.Kind.String()
	}
}

// numeric reports whether t is the number type.
func (t *Type) numeric() bool { return t != nil && t.Kind == value.KindNumber }

// isAny reports whether t imposes no constraint (Any/unknown).
func (t *Type) isAny() bool { return t == nil }

// duration reports whether t is one of the two FEEL duration types.
func (t *Type) duration() bool {
	return t != nil && (t.Kind == value.KindDaysTimeDuration || t.Kind == value.KindYearsMonthsDuration)
}

// BuiltinType resolves a FEEL built-in type name (optionally namespace-prefixed,
// e.g. "feel:number", and ignoring any generic parameter) to its Type. The
// second result is false for a name that is not a built-in type (Any, or a
// user-defined item-definition type the caller must resolve itself).
func BuiltinType(name string) (*Type, bool) {
	switch normalizeTypeName(name) {
	case "number":
		return TNumber, true
	case "string":
		return TString, true
	case "boolean":
		return TBoolean, true
	case "date":
		return TDate, true
	case "time":
		return TTime, true
	case "dateandtime", "datetime":
		return TDateTime, true
	case "daysandtimeduration", "daytimeduration":
		return TDaysTimeDuration, true
	case "yearsandmonthsduration", "yearmonthduration":
		return TYearsMonthsDuration, true
	case "duration":
		// "duration" without qualification matches either duration type; model
		// it as days-and-time, the more common form, for inference purposes.
		return TDaysTimeDuration, true
	case "list":
		return ListOf(nil), true
	case "context":
		return ContextOf(nil), true
	case "range":
		// The generic parameter (range<number>) is dropped by normalizeTypeName;
		// instance-of matches on the range kind (FEEL has no runtime element type).
		return &Type{Kind: value.KindRange}, true
	case "function":
		// The signature (function<…> -> …) is dropped; instance-of matches on the
		// function kind only.
		return &Type{Kind: value.KindFunction}, true
	default:
		return nil, false
	}
}

// feelTypeNames are the multi-word FEEL type names whose fragments include the
// `and` keyword. They are fed to the parser's name oracle so a type name like
// "days and time duration" assembles as one name (the single-word names and
// "date and time" — also a built-in function — already assemble without help).
var feelTypeNames = typeNameSet{
	"days and time duration":    true,
	"years and months duration": true,
	"date and time":             true,
}

type typeNameSet map[string]bool

func (s typeNameSet) Has(name string) bool { return s[name] }

// normalizeTypeName lowercases a type name, drops any namespace prefix and
// generic parameter, and removes spaces, so "feel:date and time" and
// "dateTime" both normalise to "dateandtime".
func normalizeTypeName(name string) string {
	name = strings.TrimSpace(name)
	if i := strings.IndexByte(name, '<'); i >= 0 {
		name = name[:i]
	}
	if i := strings.LastIndexByte(name, ':'); i >= 0 {
		name = name[i+1:]
	}
	return strings.ToLower(strings.ReplaceAll(name, " ", ""))
}

// ConstValue returns the value of src when it is a single constant literal — a
// number, string, boolean, null or @-temporal — and false otherwise. It lets a
// caller evaluate a constant cell (e.g. a decision-table output) without a
// scope, for example to test it against an allowed-values constraint (WP-31).
func ConstValue(src string) (value.Value, bool) {
	expr, err := Parse(src)
	if err != nil {
		return nil, false
	}
	switch n := expr.(type) {
	case *NumberLit:
		num, err := value.ParseNumber(n.Text)
		if err != nil {
			return nil, false
		}
		return num, true
	case *StringLit:
		return value.Str(n.Value), true
	case *BoolLit:
		return value.BoolOf(n.Value), true
	case *NullLit:
		return value.Null, true
	case *AtLit:
		v, err := parseTemporal(n.Value)
		if err != nil {
			return nil, false
		}
		return v, true
	default:
		return nil, false
	}
}

// resolveTypeString parses a FEEL type reference into a *Type: a built-in or
// user-defined (item-definition) name, or a parametrized generic — list<T>,
// context<a: T, b: U> (including nested and empty <>), or function<…>/range<…>
// (matched by kind). A nil *Type with ok=true means Any. ok is false for a name
// that resolves to neither a built-in, a user type nor a well-formed generic.
func resolveTypeString(s string, types map[string]*Type) (t *Type, ok bool) {
	s = strings.TrimSpace(s)
	lt := strings.IndexByte(s, '<')
	if lt < 0 { // a bare name: Any, built-in, or user-defined type
		name := s
		if i := strings.LastIndexByte(name, ':'); i >= 0 { // drop a namespace prefix
			name = strings.TrimSpace(name[i+1:])
		}
		if strings.EqualFold(name, "Any") {
			return nil, true
		}
		if bt, isBuiltin := BuiltinType(name); isBuiltin {
			return bt, true
		}
		if ut, isUser := types[name]; isUser {
			return ut, true
		}
		return nil, false
	}
	close := matchAngle(s, lt)
	if close < 0 {
		return nil, false
	}
	content := s[lt+1 : close]
	switch normalizeTypeName(s[:lt]) {
	case "list":
		elem, ok := resolveTypeString(content, types)
		if !ok {
			return nil, false
		}
		return ListOf(elem), true
	case "context":
		fields, ok := parseTypeFields(content, types)
		if !ok {
			return nil, false
		}
		return ContextOf(fields), true
	default: // function<…>->R, range<…>: matched by kind only
		return BuiltinType(s[:lt])
	}
}

// matchAngle returns the index of the '>' matching the '<' at open, or -1.
func matchAngle(s string, open int) int {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '<':
			depth++
		case '>':
			if depth--; depth == 0 {
				return i
			}
		}
	}
	return -1
}

// parseTypeFields parses a context<…> field list "a: T, b: U" into field types.
// An empty list is an open context (nil fields) that matches any context.
func parseTypeFields(s string, types map[string]*Type) (map[string]*Type, bool) {
	if strings.TrimSpace(s) == "" {
		return nil, true
	}
	fields := map[string]*Type{}
	for _, part := range splitTopLevel(s, ',') {
		colon := strings.IndexByte(part, ':')
		if colon < 0 {
			return nil, false
		}
		ft, ok := resolveTypeString(part[colon+1:], types)
		if !ok {
			return nil, false
		}
		fields[strings.TrimSpace(part[:colon])] = ft
	}
	return fields, true
}

// splitTopLevel splits s on sep, ignoring separators nested inside <…>.
func splitTopLevel(s string, sep byte) []string {
	var parts []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '<':
			depth++
		case '>':
			depth--
		case sep:
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, s[start:])
}

// instanceOfType evaluates `v instance of` the resolved type t (name is the raw
// type reference, for the unqualified-duration special case). Per the TCK, null
// is not an instance of any type (including Any); Any otherwise matches any
// non-null value; a bare "duration" matches either duration kind; everything else
// is item-definition conformance.
func instanceOfType(v value.Value, t *Type, name string) bool {
	if value.IsNull(v) {
		return false
	}
	if normalizeTypeName(stripGenericPrefix(name)) == "duration" {
		k := v.Kind()
		return k == value.KindDaysTimeDuration || k == value.KindYearsMonthsDuration
	}
	if t == nil { // Any
		return true
	}
	return ConformsToType(v, t)
}

// stripGenericPrefix drops any generic parameter so a name like "duration" is
// recognised even when written with one.
func stripGenericPrefix(name string) string {
	if i := strings.IndexByte(name, '<'); i >= 0 {
		return name[:i]
	}
	return name
}
