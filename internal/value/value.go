package value

import "strings"

// Kind identifies a FEEL value's type. The FEEL type system distinguishes two
// duration types that are not interconvertible (months vs. seconds).
type Kind uint8

// FEEL value kinds.
const (
	KindNull Kind = iota
	KindBool
	KindNumber
	KindString
	KindDate
	KindTime
	KindDateTime
	KindDaysTimeDuration
	KindYearsMonthsDuration
	KindList
	KindContext
	KindRange
	KindFunction
)

// String returns the FEEL type name of the kind.
func (k Kind) String() string {
	switch k {
	case KindNull:
		return "null"
	case KindBool:
		return "boolean"
	case KindNumber:
		return "number"
	case KindString:
		return "string"
	case KindDate:
		return "date"
	case KindTime:
		return "time"
	case KindDateTime:
		return "date and time"
	case KindDaysTimeDuration:
		return "days and time duration"
	case KindYearsMonthsDuration:
		return "years and months duration"
	case KindList:
		return "list"
	case KindContext:
		return "context"
	case KindRange:
		return "range"
	case KindFunction:
		return "function"
	default:
		return "unknown"
	}
}

// Value is a FEEL runtime value. Implementations are immutable; operations
// return new values. The null value is represented by Null, never a Go nil, so
// callers can always call methods safely.
type Value interface {
	Kind() Kind
	// String renders the value in its canonical FEEL form.
	String() string
	isValue()
}

// --- Null ---

type nullValue struct{}

func (nullValue) Kind() Kind     { return KindNull }
func (nullValue) String() string { return "null" }
func (nullValue) isValue()       {}

// Null is the single FEEL null value. Most operations propagate it.
var Null Value = nullValue{}

// IsNull reports whether v is the FEEL null (or a Go nil Value).
func IsNull(v Value) bool { return v == nil || v.Kind() == KindNull }

// --- Boolean ---

// Bool is a FEEL boolean.
type Bool bool

// Kind returns KindBool.
func (Bool) Kind() Kind { return KindBool }
func (b Bool) String() string {
	if bool(b) {
		return "true"
	}
	return "false"
}
func (Bool) isValue() {}

// Shared boolean values.
var (
	True  Value = Bool(true)
	False Value = Bool(false)
)

// BoolOf returns True or False for b.
func BoolOf(b bool) Value {
	if b {
		return True
	}
	return False
}

// --- String ---

// Str is a FEEL string.
type Str string

// Kind returns KindString.
func (Str) Kind() Kind       { return KindString }
func (s Str) String() string { return string(s) }
func (Str) isValue()         {}

// --- List ---

// List is an ordered FEEL list.
type List struct {
	Elements []Value
}

// Kind returns KindList.
func (List) Kind() Kind { return KindList }
func (l List) String() string {
	parts := make([]string, len(l.Elements))
	for i, e := range l.Elements {
		parts[i] = e.String()
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
func (List) isValue() {}

// NewList returns a List over the given elements.
func NewList(elems ...Value) List { return List{Elements: elems} }

// --- Context ---

// Context is an ordered key→value map. Iteration follows insertion order; two
// contexts are equal when they hold the same entries regardless of order.
type Context struct {
	keys   []string
	values map[string]Value
}

// NewContext returns an empty context.
func NewContext() *Context {
	return &Context{values: map[string]Value{}}
}

// Kind returns KindContext.
func (*Context) Kind() Kind { return KindContext }
func (*Context) isValue()   {}

// Put sets key to v, preserving insertion order for new keys, and returns c.
func (c *Context) Put(key string, v Value) *Context {
	if _, ok := c.values[key]; !ok {
		c.keys = append(c.keys, key)
	}
	c.values[key] = v
	return c
}

// Get returns the value for key and whether it is present.
func (c *Context) Get(key string) (Value, bool) {
	v, ok := c.values[key]
	return v, ok
}

// Keys returns the keys in insertion order.
func (c *Context) Keys() []string { return c.keys }

// Len returns the number of entries.
func (c *Context) Len() int { return len(c.keys) }

func (c *Context) String() string {
	parts := make([]string, len(c.keys))
	for i, k := range c.keys {
		parts[i] = k + ": " + c.values[k].String()
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// --- Range ---

// Range is a FEEL range. Low or High may be Null to denote an unbounded end.
type Range struct {
	LowClosed  bool
	Low        Value
	High       Value
	HighClosed bool
}

// Kind returns KindRange.
func (Range) Kind() Kind { return KindRange }
func (Range) isValue()   {}

func (r Range) String() string {
	lo, hi := "(", ")"
	if r.LowClosed {
		lo = "["
	}
	if r.HighClosed {
		hi = "]"
	}
	return lo + r.Low.String() + ".." + r.High.String() + hi
}

// --- Function ---

// Function is a callable FEEL value (builtin or user-defined). The Call closure
// is wired up by the compiler (WP-06); the value model only carries it.
type Function struct {
	Name  string
	Arity int
	Call  func(args []Value) (Value, error)
}

// Kind returns KindFunction.
func (*Function) Kind() Kind { return KindFunction }
func (*Function) isValue()   {}
func (f *Function) String() string {
	if f.Name != "" {
		return "function " + f.Name
	}
	return "function"
}
