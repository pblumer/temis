package feel

import "github.com/pblumer/temis/internal/value"

// Scope holds a compiled expression's variables as a flat slot array. Compiled
// variable accesses are resolved to slot indices at compile time (ADR-0004,
// architecture §5.2), so evaluation is an array index rather than a map lookup.
// A Scope is read-only during evaluation and therefore safe to share across
// goroutines.
type Scope struct {
	vars []value.Value
}

// at returns the value in slot i, or null if i is out of range (defensive; the
// compiler only ever emits valid indices).
func (s *Scope) at(i int) value.Value {
	if i < 0 || i >= len(s.vars) {
		return value.Null
	}
	return s.vars[i]
}

// Env is the compile-time symbol table mapping variable names to slot indices.
// It defines the slot layout that a matching Scope must follow.
type Env struct {
	index map[string]int
	order []string
}

// NewEnv returns an Env with the given variable names assigned slots in order.
func NewEnv(names ...string) *Env {
	e := &Env{index: make(map[string]int, len(names))}
	for _, n := range names {
		e.define(n)
	}
	return e
}

// define assigns name a slot (idempotent) and returns its index.
func (e *Env) define(name string) int {
	if i, ok := e.index[name]; ok {
		return i
	}
	i := len(e.order)
	e.index[name] = i
	e.order = append(e.order, name)
	return i
}

// slot returns the index of name and whether it is known.
func (e *Env) slot(name string) (int, bool) {
	i, ok := e.index[name]
	return i, ok
}

// Names returns the variable names in slot order.
func (e *Env) Names() []string { return e.order }

// Derive returns a new Env with extra names appended after the existing slots.
// It is used to add the implicit unary-test input "?" to a decision env without
// disturbing the existing slot indices.
func (e *Env) Derive(extra ...string) *Env {
	d := &Env{index: make(map[string]int, len(e.index)+len(extra))}
	for _, n := range e.order {
		d.define(n)
	}
	for _, n := range extra {
		d.define(n)
	}
	return d
}

// Extend returns a new Scope with extra values appended after the existing
// slots, matching an Env produced by Derive. The receiver is left unchanged, so
// a base scope can be extended repeatedly with different values.
func (s *Scope) Extend(extra ...value.Value) *Scope {
	vars := make([]value.Value, len(s.vars)+len(extra))
	copy(vars, s.vars)
	copy(vars[len(s.vars):], extra)
	return &Scope{vars: vars}
}

// NewScope builds a runtime Scope from named input values, placing each into its
// env slot. Names absent from values (or with a nil value) become null. This is
// the single map→slots boundary; everything past it is index-based.
func (e *Env) NewScope(values map[string]value.Value) *Scope {
	vars := make([]value.Value, len(e.order))
	for i, n := range e.order {
		if v, ok := values[n]; ok && v != nil {
			vars[i] = v
		} else {
			vars[i] = value.Null
		}
	}
	return &Scope{vars: vars}
}
