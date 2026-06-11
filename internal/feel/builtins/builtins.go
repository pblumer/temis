// Package builtins is the registry of FEEL built-in functions. Each builtin is a
// data-driven entry {name, params, arity, fn} bound by name at compile time, so
// the hot path calls the Go function directly with no string dispatch
// (architecture §5.5). It depends only on internal/value, never on internal/feel,
// to avoid an import cycle.
//
// Built-ins follow FEEL's null semantics: a missing or wrongly typed argument
// yields null rather than an error. A returned Go error is reserved for genuine
// failures.
package builtins

import "github.com/pblumer/temis/internal/value"

// Func is the implementation of a builtin. args holds the evaluated arguments in
// parameter order; for variadic builtins it holds all supplied arguments.
type Func func(args []value.Value) (value.Value, error)

// Builtin is one registry entry. MaxArgs of -1 marks a variadic builtin.
type Builtin struct {
	Name    string
	Params  []string
	MinArgs int
	MaxArgs int
	Fn      Func
}

// Variadic reports whether the builtin accepts an unbounded number of arguments.
func (b *Builtin) Variadic() bool { return b.MaxArgs < 0 }

// Registry maps builtin names to their definitions.
type Registry struct {
	m map[string]*Builtin
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{m: map[string]*Builtin{}} }

// Register adds or replaces a builtin.
func (r *Registry) Register(b *Builtin) { r.m[b.Name] = b }

// Lookup returns the builtin with the given name.
func (r *Registry) Lookup(name string) (*Builtin, bool) {
	b, ok := r.m[name]
	return b, ok
}

// Names returns all registered builtin names (unordered); used as a name oracle
// for the parser so multi-word builtin names assemble correctly.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.m))
	for n := range r.m {
		names = append(names, n)
	}
	return names
}

// Has implements the parser's NameSet oracle.
func (r *Registry) Has(name string) bool {
	_, ok := r.m[name]
	return ok
}

var defaultRegistry = buildDefault()

// Default returns the shared registry of standard FEEL built-ins.
func Default() *Registry { return defaultRegistry }

func buildDefault() *Registry {
	r := NewRegistry()
	registerBoolean(r)
	registerList(r)
	registerString(r)
	registerConversion(r)
	registerNumeric(r)
	return r
}

// --- shared argument helpers ---

func asNumber(v value.Value) (value.Number, bool) {
	n, ok := v.(value.Number)
	return n, ok
}

func asString(v value.Value) (string, bool) {
	s, ok := v.(value.Str)
	return string(s), ok
}

func asInt(v value.Value) (int, bool) {
	n, ok := v.(value.Number)
	if !ok {
		return 0, false
	}
	i, ok := n.Int64()
	return int(i), ok
}

// listOf treats a single list argument as the list, otherwise the arguments
// themselves form the list (so sum([1,2,3]) and sum(1,2,3) both work).
func listOf(args []value.Value) []value.Value {
	if len(args) == 1 {
		if l, ok := args[0].(value.List); ok {
			return l.Elements
		}
	}
	return args
}

// fixed builds a fixed-arity builtin.
func fixed(name string, params []string, min, max int, fn Func) *Builtin {
	return &Builtin{Name: name, Params: params, MinArgs: min, MaxArgs: max, Fn: fn}
}

// variadic builds a variadic builtin requiring at least min arguments.
func variadic(name string, min int, fn Func) *Builtin {
	return &Builtin{Name: name, MinArgs: min, MaxArgs: -1, Fn: fn}
}
