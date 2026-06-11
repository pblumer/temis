package feel

import (
	"fmt"

	"github.com/pblumer/temis/internal/value"
)

// CompiledExpr is a compiled FEEL expression: a pure Go closure that evaluates
// against a Scope. It performs no AST walk or reflection in the hot path
// (ADR-0004) and is immutable, so it may be evaluated concurrently.
type CompiledExpr func(*Scope) (value.Value, error)

// CompileError is a compile-time failure (unknown variable, unsupported
// construct, malformed literal) with its source position.
type CompileError struct {
	Msg  string
	Line int
	Col  int
}

func (e *CompileError) Error() string {
	return fmt.Sprintf("%d:%d: %s", e.Line, e.Col, e.Msg)
}

// Compile lowers an AST into a CompiledExpr, resolving variable names to slots
// via env. It returns the first CompileError encountered, if any.
func Compile(expr Expr, env *Env) (CompiledExpr, error) {
	c := &compiler{env: env}
	ce := c.compile(expr)
	if c.err != nil {
		return nil, c.err
	}
	return ce, nil
}

// CompileString parses and compiles src in one step.
func CompileString(src string, env *Env) (CompiledExpr, error) {
	expr, err := Parse(src)
	if err != nil {
		return nil, err
	}
	return Compile(expr, env)
}

type compiler struct {
	env *Env
	err *CompileError
}

// fail records the first compile error and returns a null-yielding closure so
// the compiled tree stays non-nil while the error propagates out of Compile.
func (c *compiler) fail(pos Position, format string, args ...any) CompiledExpr {
	if c.err == nil {
		c.err = &CompileError{Msg: fmt.Sprintf(format, args...), Line: pos.Line, Col: pos.Col}
	}
	return constNull
}

func constNull(*Scope) (value.Value, error) { return value.Null, nil }

func (c *compiler) compile(e Expr) CompiledExpr {
	switch n := e.(type) {
	case *NumberLit:
		num, err := value.ParseNumber(n.Text)
		if err != nil {
			return c.fail(n.Pos(), "invalid number %q", n.Text)
		}
		v := value.Value(num)
		return func(*Scope) (value.Value, error) { return v, nil }
	case *StringLit:
		v := value.Value(value.Str(n.Value))
		return func(*Scope) (value.Value, error) { return v, nil }
	case *BoolLit:
		v := value.BoolOf(n.Value)
		return func(*Scope) (value.Value, error) { return v, nil }
	case *NullLit:
		return constNull
	case *AtLit:
		v, err := parseTemporal(n.Value)
		if err != nil {
			return c.fail(n.Pos(), "invalid temporal literal @%q", n.Value)
		}
		return func(*Scope) (value.Value, error) { return v, nil }
	case *NameRef:
		i, ok := c.env.slot(n.Name)
		if !ok {
			return c.fail(n.Pos(), "unknown variable %q", n.Name)
		}
		return func(s *Scope) (value.Value, error) { return s.at(i), nil }
	case *UnaryExpr:
		x := c.compile(n.X)
		return func(s *Scope) (value.Value, error) {
			v, err := x(s)
			if err != nil {
				return nil, err
			}
			return value.Neg(v), nil
		}
	case *BinaryExpr:
		return c.compileBinary(n)
	case *BetweenExpr:
		return c.compileBetween(n)
	case *InExpr:
		return c.compileIn(n)
	case *IfExpr:
		return c.compileIf(n)
	case *ListLit:
		return c.compileList(n)
	case *ContextLit:
		return c.compileContext(n)
	case *IntervalLit:
		return c.compileInterval(n)
	case *PathExpr:
		return c.compilePath(n)
	case *CallExpr:
		return c.fail(n.Pos(), "function calls are not supported yet (WP-07)")
	case *ForExpr, *QuantifiedExpr, *FunctionDefExpr, *FilterExpr, *InstanceOfExpr:
		return c.fail(e.Pos(), "%T is not supported yet (WP-20)", e)
	default:
		return c.fail(e.Pos(), "unsupported expression %T", e)
	}
}

func (c *compiler) compileBinary(n *BinaryExpr) CompiledExpr {
	x := c.compile(n.X)
	y := c.compile(n.Y)
	switch n.Op {
	case "+":
		return valueBinop(x, y, value.Add)
	case "-":
		return valueBinop(x, y, value.Sub)
	case "*":
		return valueBinop(x, y, value.Mul)
	case "/":
		return valueBinop(x, y, value.Div)
	case "**":
		return valueBinop(x, y, value.Exp)
	case "=":
		return valueBinop(x, y, value.Equal)
	case "!=":
		return valueBinop(x, y, func(a, b value.Value) value.Value {
			return value.BoolOf(value.Equal(a, b) == value.False)
		})
	case "<", "<=", ">", ">=":
		return c.compileCompare(n.Op, x, y)
	case "and":
		return boolBinop(x, y, and3)
	case "or":
		return boolBinop(x, y, or3)
	default:
		return c.fail(n.Pos(), "unsupported operator %q", n.Op)
	}
}

func (c *compiler) compileCompare(op string, x, y CompiledExpr) CompiledExpr {
	return func(s *Scope) (value.Value, error) {
		a, err := x(s)
		if err != nil {
			return nil, err
		}
		b, err := y(s)
		if err != nil {
			return nil, err
		}
		cmp, ok := value.Compare(a, b)
		if !ok {
			return value.Null, nil
		}
		switch op {
		case "<":
			return value.BoolOf(cmp < 0), nil
		case "<=":
			return value.BoolOf(cmp <= 0), nil
		case ">":
			return value.BoolOf(cmp > 0), nil
		default: // ">="
			return value.BoolOf(cmp >= 0), nil
		}
	}
}

func (c *compiler) compileBetween(n *BetweenExpr) CompiledExpr {
	x := c.compile(n.X)
	lo := c.compile(n.Low)
	hi := c.compile(n.High)
	return func(s *Scope) (value.Value, error) {
		xv, err := x(s)
		if err != nil {
			return nil, err
		}
		lv, err := lo(s)
		if err != nil {
			return nil, err
		}
		hv, err := hi(s)
		if err != nil {
			return nil, err
		}
		lcmp, ok1 := value.Compare(lv, xv) // low <= x
		hcmp, ok2 := value.Compare(xv, hv) // x <= high
		if !ok1 || !ok2 {
			return value.Null, nil
		}
		return value.BoolOf(lcmp <= 0 && hcmp <= 0), nil
	}
}

func (c *compiler) compileIn(n *InExpr) CompiledExpr {
	x := c.compile(n.X)
	tests := make([]CompiledExpr, len(n.Tests))
	for i, t := range n.Tests {
		tests[i] = c.compile(t)
	}
	return func(s *Scope) (value.Value, error) {
		xv, err := x(s)
		if err != nil {
			return nil, err
		}
		for _, tc := range tests {
			tv, err := tc(s)
			if err != nil {
				return nil, err
			}
			if matchIn(xv, tv) {
				return value.True, nil
			}
		}
		return value.False, nil
	}
}

func (c *compiler) compileIf(n *IfExpr) CompiledExpr {
	cond := c.compile(n.Cond)
	then := c.compile(n.Then)
	els := c.compile(n.Else)
	return func(s *Scope) (value.Value, error) {
		cv, err := cond(s)
		if err != nil {
			return nil, err
		}
		if cv == value.True {
			return then(s)
		}
		return els(s)
	}
}

func (c *compiler) compileList(n *ListLit) CompiledExpr {
	elems := make([]CompiledExpr, len(n.Elements))
	for i, el := range n.Elements {
		elems[i] = c.compile(el)
	}
	return func(s *Scope) (value.Value, error) {
		vs := make([]value.Value, len(elems))
		for i, ce := range elems {
			v, err := ce(s)
			if err != nil {
				return nil, err
			}
			vs[i] = v
		}
		return value.NewList(vs...), nil
	}
}

func (c *compiler) compileContext(n *ContextLit) CompiledExpr {
	keys := make([]string, len(n.Entries))
	vals := make([]CompiledExpr, len(n.Entries))
	for i, entry := range n.Entries {
		keys[i] = entry.Key
		vals[i] = c.compile(entry.Value)
	}
	return func(s *Scope) (value.Value, error) {
		ctx := value.NewContext()
		for i, ce := range vals {
			v, err := ce(s)
			if err != nil {
				return nil, err
			}
			ctx.Put(keys[i], v)
		}
		return ctx, nil
	}
}

func (c *compiler) compileInterval(n *IntervalLit) CompiledExpr {
	lo := c.compile(n.Low)
	hi := c.compile(n.High)
	lc, hc := n.LowClosed, n.HighClosed
	return func(s *Scope) (value.Value, error) {
		l, err := lo(s)
		if err != nil {
			return nil, err
		}
		h, err := hi(s)
		if err != nil {
			return nil, err
		}
		return value.Range{LowClosed: lc, Low: l, High: h, HighClosed: hc}, nil
	}
}

func (c *compiler) compilePath(n *PathExpr) CompiledExpr {
	base := c.compile(n.X)
	name := n.Name
	return func(s *Scope) (value.Value, error) {
		v, err := base(s)
		if err != nil {
			return nil, err
		}
		ctx, ok := v.(*value.Context)
		if !ok {
			return value.Null, nil
		}
		mv, ok := ctx.Get(name)
		if !ok {
			return value.Null, nil
		}
		return mv, nil
	}
}

// valueBinop evaluates both operands and applies a value-level binary op.
func valueBinop(x, y CompiledExpr, op func(a, b value.Value) value.Value) CompiledExpr {
	return func(s *Scope) (value.Value, error) {
		a, err := x(s)
		if err != nil {
			return nil, err
		}
		b, err := y(s)
		if err != nil {
			return nil, err
		}
		return op(a, b), nil
	}
}

// boolBinop evaluates both operands and applies a three-valued boolean op.
func boolBinop(x, y CompiledExpr, op func(a, b value.Value) value.Value) CompiledExpr {
	return valueBinop(x, y, op)
}

// and3 / or3 implement FEEL's three-valued logic, where an operand that is not a
// boolean (including null) is "unknown".
func and3(a, b value.Value) value.Value {
	ab, aok := boolVal(a)
	bb, bok := boolVal(b)
	if (aok && !ab) || (bok && !bb) {
		return value.False
	}
	if aok && bok {
		return value.True
	}
	return value.Null
}

func or3(a, b value.Value) value.Value {
	ab, aok := boolVal(a)
	bb, bok := boolVal(b)
	if (aok && ab) || (bok && bb) {
		return value.True
	}
	if aok && bok {
		return value.False
	}
	return value.Null
}

func boolVal(v value.Value) (bool, bool) {
	if b, ok := v.(value.Bool); ok {
		return bool(b), true
	}
	return false, false
}

// matchIn reports whether x matches a single `in` test: containment for a range
// test, equality otherwise.
func matchIn(x, t value.Value) bool {
	if r, ok := t.(value.Range); ok {
		return rangeContains(r, x)
	}
	return value.Equal(x, t) == value.True
}

func rangeContains(r value.Range, x value.Value) bool {
	if !value.IsNull(r.Low) {
		cmp, ok := value.Compare(r.Low, x)
		if !ok || cmp > 0 || (!r.LowClosed && cmp == 0) {
			return false
		}
	}
	if !value.IsNull(r.High) {
		cmp, ok := value.Compare(x, r.High)
		if !ok || cmp > 0 || (!r.HighClosed && cmp == 0) {
			return false
		}
	}
	return true
}

// parseTemporal resolves an @-literal's content to a date, time, date-and-time
// or duration value.
func parseTemporal(s string) (value.Value, error) {
	if v, err := value.ParseDate(s); err == nil {
		return v, nil
	}
	if v, err := value.ParseDateTime(s); err == nil {
		return v, nil
	}
	if v, err := value.ParseTime(s); err == nil {
		return v, nil
	}
	if v, err := value.ParseDuration(s); err == nil {
		return v, nil
	}
	return nil, fmt.Errorf("unrecognised temporal literal %q", s)
}
