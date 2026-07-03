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

// instanceOf evaluates `v instance of typeName` per FEEL: it reports whether v's
// runtime kind matches the named type. "Any" matches every value (including
// null); a duration name matches either duration kind when unqualified. ok is
// false for a type name that is neither built-in nor "Any", so the compiler can
// reject an unknown type.
func instanceOf(v value.Value, typeName string) (result, ok bool) {
	n := normalizeTypeName(typeName)
	if n == "any" {
		return true, true
	}
	if n == "duration" {
		k := v.Kind()
		return k == value.KindDaysTimeDuration || k == value.KindYearsMonthsDuration, true
	}
	t, isBuiltin := BuiltinType(typeName)
	if !isBuiltin {
		return false, false
	}
	return v.Kind() == t.Kind, true
}
