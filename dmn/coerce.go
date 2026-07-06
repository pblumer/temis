package dmn

import (
	"github.com/pblumer/temis/internal/feel"
	"github.com/pblumer/temis/internal/value"
)

// coerceToType applies FEEL's item-definition coercion of a value to a declared
// type at decision-output boundaries (DMN §10.3.2.9.4): a conforming value is
// kept, a singleton list whose sole element conforms is unwrapped, otherwise the
// value becomes null. The shared implementation lives in package feel so both
// decision outputs and function/service invocation boundaries coerce identically.
func coerceToType(v value.Value, t *feel.Type) value.Value {
	return feel.CoerceToType(v, t)
}
