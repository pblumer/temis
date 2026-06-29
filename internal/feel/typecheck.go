package feel

import (
	"fmt"

	"github.com/pblumer/temis/internal/value"
)

// TypeEnv maps variable names to their declared types for static checking. A
// name that is absent (or mapped to nil) is Any and is never flagged.
type TypeEnv struct {
	vars map[string]*Type
}

// NewTypeEnv returns an empty type environment.
func NewTypeEnv() *TypeEnv { return &TypeEnv{vars: map[string]*Type{}} }

// Set binds name to type t and returns the env for chaining.
func (e *TypeEnv) Set(name string, t *Type) *TypeEnv {
	e.vars[name] = t
	return e
}

func (e *TypeEnv) get(name string) *Type {
	if e == nil {
		return nil
	}
	return e.vars[name]
}

func (e *TypeEnv) child(name string, t *Type) *TypeEnv {
	c := &TypeEnv{vars: make(map[string]*Type, len(e.vars)+1)}
	for k, v := range e.vars {
		c.vars[k] = v
	}
	c.vars[name] = t
	return c
}

// TypeError is a static type-check finding with its source position (within the
// expression text).
type TypeError struct {
	Msg  string
	Line int
	Col  int
}

func (e TypeError) Error() string { return fmt.Sprintf("%d:%d: %s", e.Line, e.Col, e.Msg) }

// Typecheck statically infers types over expr against env and returns the
// provable type mismatches it finds. It is deliberately conservative: an operand
// whose type is unknown (Any) is never flagged, so a well-typed model and a
// model the checker cannot reason about both produce no findings — only a
// definite, statically-provable clash is reported. Evaluation still follows
// FEEL's null semantics regardless; these findings are advisory.
func Typecheck(expr Expr, env *TypeEnv) []TypeError {
	tc := &typechecker{env: env}
	tc.infer(expr, env)
	return tc.errs
}

// TypecheckString parses src (using the same name oracle as compilation, so
// multi-word built-in and function names assemble) and type-checks it against
// env. A source that does not parse yields no findings — the compile path
// reports the syntax error separately.
func TypecheckString(src string, env *TypeEnv, funcs map[string]*Func) []TypeError {
	expr, err := ParseWithNames(src, nameOracle(funcs))
	if err != nil {
		return nil
	}
	return Typecheck(expr, env)
}

type typechecker struct {
	env  *TypeEnv
	errs []TypeError
}

func (tc *typechecker) fail(pos Position, format string, args ...any) {
	tc.errs = append(tc.errs, TypeError{Msg: fmt.Sprintf(format, args...), Line: pos.Line, Col: pos.Col})
}

// infer returns the inferred type of e under env (nil = Any), recording any
// provable mismatch on the way.
func (tc *typechecker) infer(e Expr, env *TypeEnv) *Type {
	switch n := e.(type) {
	case *NumberLit:
		return TNumber
	case *StringLit:
		return TString
	case *BoolLit:
		return TBoolean
	case *NullLit:
		return nil // null is compatible with everything; never a clash
	case *AtLit:
		if v, err := parseTemporal(n.Value); err == nil {
			return &Type{Kind: v.Kind()}
		}
		return nil
	case *NameRef:
		return env.get(n.Name)
	case *UnaryExpr:
		t := tc.infer(n.X, env)
		if !t.isAny() && !t.numeric() && !t.duration() {
			tc.fail(n.Pos(), "negation needs a number or duration, got %s", t)
		}
		return t
	case *BinaryExpr:
		return tc.inferBinary(n, env)
	case *BetweenExpr:
		x := tc.infer(n.X, env)
		lo := tc.infer(n.Low, env)
		hi := tc.infer(n.High, env)
		tc.requireComparable(n.Pos(), x, lo)
		tc.requireComparable(n.Pos(), x, hi)
		return TBoolean
	case *InExpr:
		tc.infer(n.X, env)
		for _, t := range n.Tests {
			tc.infer(t, env)
		}
		return TBoolean
	case *IfExpr:
		return tc.inferIf(n, env)
	case *ListLit:
		return tc.inferList(n, env)
	case *ContextLit:
		return tc.inferContext(n, env)
	case *InstanceOfExpr:
		tc.infer(n.X, env)
		return TBoolean
	case *PathExpr:
		return tc.inferPath(n, env)
	case *ForExpr:
		return tc.inferFor(n, env)
	case *QuantifiedExpr:
		return tc.inferQuantified(n, env)
	case *FilterExpr:
		tc.infer(n.X, env)
		// The predicate runs per element with its own implicit binding; checking
		// it needs the element type, which is generally unknown here, so skip it.
		return ListOf(nil)
	default:
		// CallExpr, FunctionDefExpr, IntervalLit and any other form: do not infer
		// a concrete type (so nothing downstream is wrongly flagged). Still walk
		// call arguments so nested mismatches surface.
		if call, ok := e.(*CallExpr); ok {
			for _, a := range call.Args {
				tc.infer(a.Value, env)
			}
		}
		return nil
	}
}

func (tc *typechecker) inferBinary(n *BinaryExpr, env *TypeEnv) *Type {
	x := tc.infer(n.X, env)
	y := tc.infer(n.Y, env)
	switch n.Op {
	case "+", "-", "*", "/", "**":
		tc.requireArithmetic(n.Pos(), n.Op, x)
		tc.requireArithmetic(n.Pos(), n.Op, y)
		if x.numeric() && y.numeric() {
			return TNumber
		}
		return nil // temporal/duration arithmetic has many result shapes
	case "<", "<=", ">", ">=":
		tc.requireComparable(n.Pos(), x, y)
		return TBoolean
	case "=", "!=":
		return TBoolean // equality is defined across types (yields false/null)
	case "and", "or":
		tc.requireBoolean(n.Pos(), n.Op, x)
		tc.requireBoolean(n.Pos(), n.Op, y)
		return TBoolean
	default:
		return nil
	}
}

// arithmeticForbidden are the concrete kinds that can never appear in arithmetic.
var arithmeticForbidden = map[value.Kind]bool{
	value.KindString:  true,
	value.KindBool:    true,
	value.KindList:    true,
	value.KindContext: true,
}

func (tc *typechecker) requireArithmetic(pos Position, op string, t *Type) {
	if t.isAny() {
		return
	}
	if arithmeticForbidden[t.Kind] {
		tc.fail(pos, "operator %q cannot apply to %s", op, t)
	}
}

func (tc *typechecker) requireBoolean(pos Position, op string, t *Type) {
	if !t.isAny() && t.Kind != value.KindBool {
		tc.fail(pos, "operator %q needs a boolean, got %s", op, t)
	}
}

// requireComparable flags an ordering comparison between two concrete types of
// different kinds (e.g. number < string), which can never be ordered.
func (tc *typechecker) requireComparable(pos Position, a, b *Type) {
	if a.isAny() || b.isAny() {
		return
	}
	if a.Kind != b.Kind {
		tc.fail(pos, "cannot compare %s with %s", a, b)
	}
}

func (tc *typechecker) inferIf(n *IfExpr, env *TypeEnv) *Type {
	cond := tc.infer(n.Cond, env)
	if !cond.isAny() && cond.Kind != value.KindBool {
		tc.fail(n.Cond.Pos(), "if condition must be boolean, got %s", cond)
	}
	then := tc.infer(n.Then, env)
	els := tc.infer(n.Else, env)
	return join(then, els)
}

// join returns the common type of two branches: the shared type when their kinds
// match, otherwise Any. A null branch (nil) yields the other branch's type.
func join(a, b *Type) *Type {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if a.Kind == b.Kind {
		return a
	}
	return nil
}

func (tc *typechecker) inferList(n *ListLit, env *TypeEnv) *Type {
	var elem *Type
	first := true
	for _, el := range n.Elements {
		t := tc.infer(el, env)
		if first {
			elem, first = t, false
			continue
		}
		elem = join(elem, t)
	}
	return ListOf(elem)
}

func (tc *typechecker) inferContext(n *ContextLit, env *TypeEnv) *Type {
	fields := make(map[string]*Type, len(n.Entries))
	// Later entries may reference earlier ones; thread the growing env.
	cur := env
	for _, entry := range n.Entries {
		t := tc.infer(entry.Value, cur)
		fields[entry.Key] = t
		cur = cur.child(entry.Key, t)
	}
	return ContextOf(fields)
}

func (tc *typechecker) inferPath(n *PathExpr, env *TypeEnv) *Type {
	base := tc.infer(n.X, env)
	if base != nil && base.Kind == value.KindContext && base.Fields != nil {
		if ft, ok := base.Fields[n.Name]; ok {
			return ft
		}
	}
	return nil // member of an unknown or open type
}

func (tc *typechecker) inferFor(n *ForExpr, env *TypeEnv) *Type {
	child := env
	for _, it := range n.Iterators {
		dom := tc.infer(it.In, child)
		child = child.child(it.Name, elementType(dom))
	}
	body := tc.infer(n.Return, child)
	return ListOf(body)
}

func (tc *typechecker) inferQuantified(n *QuantifiedExpr, env *TypeEnv) *Type {
	child := env
	for _, it := range n.Iterators {
		dom := tc.infer(it.In, child)
		child = child.child(it.Name, elementType(dom))
	}
	tc.infer(n.Satisfies, child)
	return TBoolean
}

// elementType returns the element type of a list type, or Any for anything else.
func elementType(t *Type) *Type {
	if t != nil && t.Kind == value.KindList {
		return t.Elem
	}
	return nil
}
