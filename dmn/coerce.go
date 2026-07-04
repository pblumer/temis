package dmn

import (
	"github.com/pblumer/temis/internal/feel"
	"github.com/pblumer/temis/internal/value"
)

// coerceToType applies FEEL's item-definition coercion of a value to a declared
// type (DMN §10.3.2.9.4), used at decision-output boundaries:
//
//  1. no declared type (Any) imposes nothing;
//  2. a value that already conforms is kept;
//  3. a singleton list whose sole element conforms is unwrapped to that element;
//  4. otherwise the value does not conform and becomes null.
func coerceToType(v value.Value, t *feel.Type) value.Value {
	if t == nil { // Any
		return v
	}
	if conformsToType(v, t) {
		return v
	}
	if l, ok := v.(value.List); ok && len(l.Elements) == 1 && conformsToType(l.Elements[0], t) {
		return l.Elements[0]
	}
	return value.Null
}

// conformsToType reports whether v is a member of the FEEL type t. null is a
// member of every type; Any accepts anything; lists and contexts are checked
// element- and field-wise against any declared element/field types.
func conformsToType(v value.Value, t *feel.Type) bool {
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
			if !conformsToType(e, t.Elem) {
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
			if !ok || !conformsToType(fv, ft) {
				return false
			}
		}
		return true
	default:
		return true
	}
}
