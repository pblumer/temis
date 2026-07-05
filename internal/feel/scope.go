package feel

import (
	"fmt"

	"github.com/pblumer/temis/internal/value"
)

// Scope holds a compiled expression's variables as a flat slot array. Compiled
// variable accesses are resolved to slot indices at compile time (ADR-0004,
// architecture §5.2), so evaluation is an array index rather than a map lookup.
// A Scope is read-only during evaluation and therefore safe to share across
// goroutines.
//
// A Scope may carry an opaque trace sink (WithTrace). The execution core treats
// it as an untyped value and never inspects it; a consumer that wants an
// explanation attaches its own recorder and type-asserts it back where it
// records (e.g. the decision-table evaluator). The default scope carries no sink
// (nil), so non-traced evaluation stays allocation-free.
type Scope struct {
	vars  []value.Value
	trace any
	// st is per-evaluation execution state shared across the scope lineage of a
	// single NewScope (propagated by Extend). It bounds user-function recursion
	// (WP-24). It is nil only for scopes built outside NewScope.
	st *evalState
}

// Trace returns the scope's opaque trace sink, or nil when tracing is off.
func (s *Scope) Trace() any { return s.trace }

// WithTrace returns a shallow copy of the scope carrying the given trace sink.
// The variable slots are shared (they are read-only), so this is cheap and is
// done once per traced evaluation at the root scope.
func (s *Scope) WithTrace(sink any) *Scope {
	ns := *s
	ns.trace = sink
	return &ns
}

// evalState carries mutable, per-evaluation execution state. A fresh instance is
// created by NewScope and shared (never copied) down the scope lineage, so it is
// confined to one evaluation and never touched by concurrent ones. It also
// enforces the per-evaluation resource limits (ADR-0008, WP-34).
type evalState struct {
	depth    int // current user-function call depth
	iter     int // iteration steps taken so far across all comprehensions
	maxDepth int // limit beyond which recursion is refused (0 = unlimited)
	maxIter  int // limit on total iteration steps (0 = unlimited)
	maxItems int // limit on the element count of any single produced list (0 = unlimited)
}

// enterCall accounts for entering a user-function call and reports a LimitError
// once the call-depth budget is exhausted. The matching leaveCall must run on
// the way out.
func (st *evalState) enterCall() error {
	if st == nil {
		return nil
	}
	if st.maxDepth != 0 && st.depth >= st.maxDepth {
		return &LimitError{Limit: "call depth", Max: st.maxDepth}
	}
	st.depth++
	return nil
}

func (st *evalState) leaveCall() {
	if st != nil {
		st.depth--
	}
}

// step accounts for one iteration step and reports a LimitError once the
// configured iteration budget is exhausted.
func (st *evalState) step() error {
	if st == nil || st.maxIter == 0 {
		return nil
	}
	st.iter++
	if st.iter > st.maxIter {
		return &LimitError{Limit: "iterations", Max: st.maxIter}
	}
	return nil
}

// checkItems reports a LimitError when a produced list would exceed the
// element-count limit.
func (st *evalState) checkItems(n int) error {
	if st == nil || st.maxItems == 0 || n <= st.maxItems {
		return nil
	}
	return &LimitError{Limit: "list size", Max: st.maxItems}
}

// LimitError reports that an evaluation exceeded a configured resource limit
// (ADR-0008). The execution-edge classifier maps it to a distinct code so a
// limit breach is distinguishable from other runtime failures.
type LimitError struct {
	Limit string // which limit: "call depth", "iterations", "list size"
	Max   int
}

func (e *LimitError) Error() string {
	return fmt.Sprintf("feel: %s limit %d exceeded", e.Limit, e.Max)
}

// Limits configures the per-evaluation resource bounds. A zero field means that
// dimension is unbounded; DefaultLimits supplies safe non-zero defaults.
type Limits struct {
	MaxCallDepth  int // nested user-function (BKM / function literal) calls
	MaxIterations int // total iteration steps across all comprehensions
	MaxListSize   int // element count of any single produced list
}

// DefaultMaxCallDepth bounds nested user-function (BKM / function literal) calls,
// turning unbounded recursion into a runtime error instead of a stack overflow
// (ADR-0008).
const DefaultMaxCallDepth = 256

// DefaultLimits returns the resource limits applied when none are configured.
// They are generous enough not to affect normal models yet bound hostile input
// (deep recursion, runaway comprehensions, huge lists).
func DefaultLimits() Limits {
	return Limits{MaxCallDepth: DefaultMaxCallDepth, MaxIterations: 10_000_000, MaxListSize: 10_000_000}
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
	// types resolves user-defined item-definition type names (e.g. tNumberList)
	// for the `instance of` operator; nil when only built-in types are in scope.
	// It is carried on the env so it reaches every compiled sub-expression without
	// threading a separate parameter, and is shared (never mutated) by Derive/Append.
	types map[string]*Type
}

// WithTypes returns e with the given user-type resolver attached (for `instance
// of`). The map is shared, not copied; callers must not mutate it afterwards.
func (e *Env) WithTypes(types map[string]*Type) *Env {
	e.types = types
	return e
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

// Has reports whether name is a bound variable. It lets an Env act as a parser
// NameSet oracle so a variable whose name embeds a keyword or a hyphen (e.g.
// "Date-Time") assembles as one name instead of a subtraction (WP-41.15).
func (e *Env) Has(name string) bool { _, ok := e.index[name]; return ok }

// Derive returns a new Env with extra names appended after the existing slots.
// It is used to add the implicit unary-test input "?" to a decision env without
// disturbing the existing slot indices.
func (e *Env) Derive(extra ...string) *Env {
	d := &Env{index: make(map[string]int, len(e.index)+len(extra)), types: e.types}
	for _, n := range e.order {
		d.define(n)
	}
	for _, n := range extra {
		d.define(n)
	}
	return d
}

// Append returns a new Env with name bound to a fresh trailing slot, shadowing
// any existing binding of the same name. Unlike Derive it always allocates a new
// slot, so it pairs with Scope.Extend to introduce iteration and filter
// variables that may shadow an outer variable of the same name.
func (e *Env) Append(name string) *Env {
	d := &Env{
		index: make(map[string]int, len(e.index)+1),
		order: append([]string(nil), e.order...),
		types: e.types,
	}
	for k, v := range e.index {
		d.index[k] = v
	}
	d.index[name] = len(d.order)
	d.order = append(d.order, name)
	return d
}

// Extend returns a new Scope with extra values appended after the existing
// slots, matching an Env produced by Derive. The receiver is left unchanged, so
// a base scope can be extended repeatedly with different values.
func (s *Scope) Extend(extra ...value.Value) *Scope {
	vars := make([]value.Value, len(s.vars)+len(extra))
	copy(vars, s.vars)
	copy(vars[len(s.vars):], extra)
	return &Scope{vars: vars, trace: s.trace, st: s.st}
}

// NewScope builds a runtime Scope from named input values, placing each into its
// env slot. Names absent from values (or with a nil value) become null. This is
// the single map→slots boundary; everything past it is index-based.
func (e *Env) NewScope(values map[string]value.Value) *Scope {
	return e.NewScopeWithLimits(values, DefaultLimits())
}

// NewScopeWithLimits is NewScope with explicit resource limits enforced for the
// evaluation rooted at the returned scope (ADR-0008, WP-34).
func (e *Env) NewScopeWithLimits(values map[string]value.Value, lim Limits) *Scope {
	vars := make([]value.Value, len(e.order))
	for i, n := range e.order {
		if v, ok := values[n]; ok && v != nil {
			vars[i] = v
		} else {
			vars[i] = value.Null
		}
	}
	return &Scope{vars: vars, st: &evalState{
		maxDepth: lim.MaxCallDepth,
		maxIter:  lim.MaxIterations,
		maxItems: lim.MaxListSize,
	}}
}
