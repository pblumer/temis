package feel

import "github.com/pblumer/temis/internal/value"

// ConformsToType reports whether v is a member of the FEEL type t. null is a
// member of every type; a nil *Type is Any and accepts anything; lists and
// contexts are checked element- and field-wise against any declared element or
// field types (DMN §10.3.2.9.4).
func ConformsToType(v value.Value, t *Type) bool {
	if t == nil || value.IsNull(v) {
		return true
	}
	if v.Kind() != t.Kind {
		return false
	}
	switch t.Kind {
	case value.KindList:
		if t.Elem == nil {
			return true
		}
		for _, e := range v.(value.List).Elements {
			if !ConformsToType(e, t.Elem) {
				return false
			}
		}
		return true
	case value.KindContext:
		if t.Fields == nil {
			return true
		}
		c := v.(*value.Context)
		for name, ft := range t.Fields {
			fv, ok := c.Get(name)
			if !ok || !ConformsToType(fv, ft) {
				return false
			}
		}
		return true
	default:
		return true
	}
}

// CoerceToType applies FEEL's item-definition coercion of a value to a declared
// type (DMN §10.3.2.9.4): a conforming value is kept; a singleton list whose sole
// element conforms is unwrapped to that element; anything else becomes null. A
// nil *Type (Any) imposes nothing.
func CoerceToType(v value.Value, t *Type) value.Value {
	if t == nil {
		return v
	}
	if ConformsToType(v, t) {
		return v
	}
	if l, ok := v.(value.List); ok && len(l.Elements) == 1 && ConformsToType(l.Elements[0], t) {
		return l.Elements[0]
	}
	return value.Null
}

// CoerceArg coerces a call argument to a formal parameter's declared type. Unlike
// CoerceToType it distinguishes a genuine non-conformance (ok=false) from a valid
// coercion, so an invocation can evaluate to null as a whole — the function is
// "not invoked" — rather than binding a silently-nulled argument (DMN §10.4,
// TCK 0082/0085). A null argument conforms to every type.
func CoerceArg(v value.Value, t *Type) (value.Value, bool) {
	if t == nil || ConformsToType(v, t) {
		return v, true
	}
	if l, ok := v.(value.List); ok && len(l.Elements) == 1 && ConformsToType(l.Elements[0], t) {
		return l.Elements[0], true
	}
	return value.Null, false
}
