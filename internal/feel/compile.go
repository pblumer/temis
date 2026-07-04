package feel

import (
	"fmt"
	"sort"

	"github.com/pblumer/temis/internal/feel/builtins"
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
	return CompileWith(expr, env, nil)
}

// CompileWith is Compile with a set of user-defined functions (BKMs, boxed
// function definitions) in scope, resolved by name when an expression calls or
// references them (WP-23/WP-24). A nil map behaves like Compile.
func CompileWith(expr Expr, env *Env, funcs map[string]*Func) (CompiledExpr, error) {
	c := &compiler{env: env, builtins: builtins.Default(), funcs: funcs}
	ce := c.compile(expr)
	if c.err != nil {
		return nil, c.err
	}
	return ce, nil
}

// CompileString parses and compiles src in one step. It supplies the built-in
// registry as the parser's name oracle so multi-word builtin names whose
// fragments include keywords (e.g. "index of") assemble correctly.
func CompileString(src string, env *Env) (CompiledExpr, error) {
	return CompileStringWith(src, env, nil)
}

// CompileStringWith is CompileString with user-defined functions in scope. The
// parser's name oracle covers both the built-ins and the function names, so a
// multi-word function name (e.g. a BKM named "Rate Table") assembles correctly.
func CompileStringWith(src string, env *Env, funcs map[string]*Func) (CompiledExpr, error) {
	expr, err := ParseWithNames(src, nameOracle(funcs))
	if err != nil {
		return nil, err
	}
	return CompileWith(expr, env, funcs)
}

// CompileStringRefs is CompileString that also returns which of env's variable
// names the expression references (its free variables drawn from env). It lets a
// caller learn an expression's dependencies without a separate AST walk — used by
// package dmn's CompiledExpression to report references (full-FEEL flow mappings).
// The returned names are sorted and unique.
func CompileStringRefs(src string, env *Env) (CompiledExpr, []string, error) {
	expr, err := ParseWithNames(src, nameOracle(nil))
	if err != nil {
		return nil, nil, err
	}
	c := &compiler{env: env, builtins: builtins.Default(), used: map[string]bool{}}
	if env != nil {
		c.rootNames = make(map[string]bool, len(env.Names()))
		for _, n := range env.Names() {
			c.rootNames[n] = true
		}
	}
	ce := c.compile(expr)
	if c.err != nil {
		return nil, nil, c.err
	}
	refs := make([]string, 0, len(c.used))
	for n := range c.used {
		refs = append(refs, n)
	}
	sort.Strings(refs)
	return ce, refs, nil
}

// nameOracle returns the parser name oracle covering the built-ins and any
// user-function names, so multi-word names from either source assemble.
func nameOracle(funcs map[string]*Func) NameSet {
	sets := unionNames{builtins.Default(), feelTypeNames}
	if len(funcs) > 0 {
		sets = append(sets, funcNames(funcs))
	}
	return sets
}

type compiler struct {
	env      *Env
	builtins *builtins.Registry
	funcs    map[string]*Func
	err      *CompileError
	// rootNames and used, when non-nil, track which of the root env's variable
	// names the expression actually references (CompileStringRefs). They stay nil
	// on the normal compile path, so it keeps its allocation-free behaviour.
	rootNames map[string]bool
	used      map[string]bool
	// implicit holds the scope slots of enclosing filter elements (innermost
	// last). A name that resolves to no static slot is looked up against these
	// at runtime, so filter predicates can reference the keys of context
	// elements directly (e.g. people[age > 18]).
	implicit []int
}

// withEnv runs fn with c.env temporarily replaced, restoring it afterwards. It
// lets iteration and filter bodies compile against an env extended with their
// loop variables without disturbing the surrounding compilation.
func (c *compiler) withEnv(env *Env, fn func()) {
	prev := c.env
	c.env = env
	fn()
	c.env = prev
}

// fail records the first compile error and returns a null-yielding closure so
// the compiled tree stays non-nil while the error propagates out of Compile.
func (c *compiler) fail(pos Position, format string, args ...any) CompiledExpr {
	if c.err == nil {
		c.err = &CompileError{Msg: fmt.Sprintf(format, args...), Line: pos.Line, Col: pos.Col}
	}
	return constNull
}

// nullCall handles a function invocation that is syntactically well-formed but
// semantically invalid — the wrong number of arguments, or unknown/mixed named
// parameters. FEEL evaluates such a call to null and keeps the decision
// executable (a total-function semantics), rather than making the whole decision
// non-executable. bindArgs/bindNamedArgs return this (nil), which their callers
// compile to a null-yielding expression; c.err is deliberately left unset.
func (c *compiler) nullCall() []CompiledExpr { return nil }

func constNull(*Scope) (value.Value, error) { return value.Null, nil }

// NullExpr is a CompiledExpr that always yields null. It fills omitted arguments
// of a call so the callee always receives a full argument list.
func NullExpr(s *Scope) (value.Value, error) { return constNull(s) }

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
		if i, ok := c.env.slot(n.Name); ok {
			if c.used != nil && c.rootNames[n.Name] {
				c.used[n.Name] = true
			}
			return func(s *Scope) (value.Value, error) { return s.at(i), nil }
		}
		// A named user function (BKM / function definition) referenced as a value
		// lifts to a first-class function value bound to the current recursion
		// budget, so it can be passed to higher-order built-ins or stored.
		if f, ok := c.funcs[n.Name]; ok {
			return func(s *Scope) (value.Value, error) { return f.asValue(s), nil }
		}
		// Not a static variable: inside a filter, resolve against the enclosing
		// context elements at runtime (innermost first); otherwise it is an error.
		if len(c.implicit) > 0 {
			slots := append([]int(nil), c.implicit...)
			name := n.Name
			return func(s *Scope) (value.Value, error) {
				for k := len(slots) - 1; k >= 0; k-- {
					if ctx, ok := s.at(slots[k]).(*value.Context); ok {
						if v, ok := ctx.Get(name); ok {
							return v, nil
						}
					}
				}
				return value.Null, nil
			}
		}
		return c.fail(n.Pos(), "unknown variable %q", n.Name)
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
		return c.compileCall(n)
	case *ForExpr:
		return c.compileForExpr(n)
	case *QuantifiedExpr:
		return c.compileQuantified(n)
	case *FilterExpr:
		return c.compileFilter(n)
	case *FunctionDefExpr:
		return c.compileFunctionDef(n)
	case *InstanceOfExpr:
		return c.compileInstanceOf(n)
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
		return valueBinop(x, y, feelEqualOp)
	case "!=":
		return valueBinop(x, y, func(a, b value.Value) value.Value {
			return notBool(feelEqualOp(a, b))
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
	// Each right-hand test is either an operator comparison (CmpTest → compare the
	// in-value against the operand) or a plain value/interval/list matched by
	// matchIn. `x in t1, t2, …` is the disjunction of the tests (FEEL 10.3.2.15).
	type inTest struct {
		cmp string       // comparison op when non-empty; otherwise a matchIn test
		e   CompiledExpr // the operand (cmp) or the test value (matchIn)
	}
	tests := make([]inTest, len(n.Tests))
	for i, t := range n.Tests {
		if ct, ok := t.(*CmpTest); ok {
			tests[i] = inTest{cmp: ct.Op, e: c.compile(ct.Y)}
		} else {
			tests[i] = inTest{e: c.compile(t)}
		}
	}
	return func(s *Scope) (value.Value, error) {
		xv, err := x(s)
		if err != nil {
			return nil, err
		}
		for _, t := range tests {
			tv, err := t.e(s)
			if err != nil {
				return nil, err
			}
			if t.cmp != "" {
				if cmpSatisfies(t.cmp, xv, tv) {
					return value.True, nil
				}
			} else if matchIn(xv, tv) {
				return value.True, nil
			}
		}
		return value.False, nil
	}
}

// cmpSatisfies reports whether `x <op> y` holds for an operator-prefixed unary
// test on the right of `in`. Incomparable operands are not a match.
func cmpSatisfies(op string, x, y value.Value) bool {
	switch op {
	case "=":
		return value.Equal(x, y) == value.True
	case "!=":
		return value.Equal(x, y) == value.False
	}
	cmp, ok := value.Compare(x, y)
	if !ok {
		return false
	}
	switch op {
	case "<":
		return cmp < 0
	case "<=":
		return cmp <= 0
	case ">":
		return cmp > 0
	case ">=":
		return cmp >= 0
	}
	return false
}

// compileInstanceOf compiles `X instance of Type`. The type name must be a FEEL
// built-in (or Any); an unknown name (e.g. a user-defined item-definition type)
// is a compile error until the type system binds them (WP-31).
func (c *compiler) compileInstanceOf(n *InstanceOfExpr) CompiledExpr {
	if _, ok := instanceOf(value.Null, n.Type); !ok {
		return c.fail(n.Pos(), "unknown type %q in instance of", n.Type)
	}
	x := c.compile(n.X)
	typeName := n.Type
	return func(s *Scope) (value.Value, error) {
		v, err := x(s)
		if err != nil {
			return nil, err
		}
		res, _ := instanceOf(v, typeName)
		return value.BoolOf(res), nil
	}
}

func (c *compiler) compileIf(n *IfExpr) CompiledExpr {
	return IfThenElse(c.compile(n.Cond), c.compile(n.Then), c.compile(n.Else))
}

// IfThenElse builds a FEEL conditional: then runs only when cond is exactly the
// boolean true, otherwise els runs (so a null or non-boolean condition takes the
// else branch). It is the shared runtime of the literal `if` and the boxed
// <conditional> (WP-26).
func IfThenElse(cond, then, els CompiledExpr) CompiledExpr {
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
		// Path projection over a list: e.name yields the list of each element's
		// member (null where an element is not a context or lacks the key).
		if l, ok := v.(value.List); ok {
			out := make([]value.Value, len(l.Elements))
			for i, e := range l.Elements {
				out[i] = memberOf(e, name)
			}
			return value.NewList(out...), nil
		}
		return memberOf(v, name), nil
	}
}

// memberOf returns the named member of a value: a context entry, or a temporal
// or duration property (e.g. date(...).year). It yields null when v has no such
// member.
func memberOf(v value.Value, name string) value.Value {
	if ctx, ok := v.(*value.Context); ok {
		if mv, ok := ctx.Get(name); ok {
			return mv
		}
		return value.Null
	}
	if mv, ok := value.Member(v, name); ok {
		return mv
	}
	return value.Null
}

func (c *compiler) compileCall(n *CallExpr) CompiledExpr {
	if name, ok := n.Fn.(*NameRef); ok {
		if b, ok := c.builtins.Lookup(name.Name); ok {
			return c.compileBuiltinCall(b, n)
		}
		if f, ok := c.funcs[name.Name]; ok {
			return c.compileFuncCall(f, n)
		}
		// A name bound to a function value (e.g. a parameter or context entry
		// holding a function) is resolved and called at runtime.
		if _, ok := c.env.slot(name.Name); ok {
			return c.compileValueCall(c.compile(name), n)
		}
		return c.fail(name.Pos(), "unknown function %q", name.Name)
	}
	// The callee is an arbitrary expression that must evaluate to a function. A
	// literal can never be one, so reject it at compile time.
	switch n.Fn.(type) {
	case *NumberLit, *StringLit, *BoolLit, *NullLit, *AtLit:
		return c.fail(n.Fn.Pos(), "callee is not a function")
	}
	return c.compileValueCall(c.compile(n.Fn), n)
}

func (c *compiler) compileBuiltinCall(b *builtins.Builtin, n *CallExpr) CompiledExpr {
	argExprs := c.bindArgs(b, n)
	if argExprs == nil {
		return constNull
	}
	fn := b.Fn
	return func(s *Scope) (value.Value, error) {
		vals, err := evalArgs(argExprs, s)
		if err != nil {
			return nil, err
		}
		return fn(vals)
	}
}

// compileFuncCall binds a call to a statically known user function, arranging
// named arguments into the function's parameter order. This is the path that
// supports recursion: the body's self-reference resolves to the same *Func.
func (c *compiler) compileFuncCall(f *Func, n *CallExpr) CompiledExpr {
	argExprs := c.bindNamedArgs(f.Params, f.Name, n)
	if argExprs == nil {
		return constNull
	}
	return func(s *Scope) (value.Value, error) {
		vals, err := evalArgs(argExprs, s)
		if err != nil {
			return nil, err
		}
		return f.call(s, vals)
	}
}

// compileValueCall calls whatever function value the callee expression yields,
// passing positional arguments (named arguments are not available without the
// callee's parameter list, so they are rejected).
func (c *compiler) compileValueCall(callee CompiledExpr, n *CallExpr) CompiledExpr {
	argExprs := make([]CompiledExpr, len(n.Args))
	for i, a := range n.Args {
		if a.Name != "" {
			return c.fail(n.Pos(), "named arguments require a statically known function")
		}
		argExprs[i] = c.compile(a.Value)
	}
	return func(s *Scope) (value.Value, error) {
		fv, err := callee(s)
		if err != nil {
			return nil, err
		}
		fn, ok := fv.(*value.Function)
		if !ok {
			return value.Null, nil
		}
		vals, err := evalArgs(argExprs, s)
		if err != nil {
			return nil, err
		}
		return fn.Call(vals)
	}
}

// compileFunctionDef compiles a function(...) literal into a closure that, when
// evaluated, captures the current scope and yields a function value. The body is
// compiled against the surrounding env extended with the formal parameters, so
// the closure can read enclosing variables (closures over context, WP-24).
func (c *compiler) compileFunctionDef(n *FunctionDefExpr) CompiledExpr {
	if n.External {
		return c.fail(n.Pos(), "external functions are not supported")
	}
	bodyEnv := c.env
	params := make([]string, len(n.Params))
	for i, p := range n.Params {
		params[i] = p.Name
		bodyEnv = bodyEnv.Append(p.Name)
	}
	var body CompiledExpr
	c.withEnv(bodyEnv, func() { body = c.compile(n.Body) })
	if c.err != nil {
		return constNull
	}
	return FuncValue(params, body)
}

// FuncValue returns a CompiledExpr that yields a first-class function value
// capturing the scope it is evaluated in (closure over enclosing variables). The
// body must have been compiled against an Env whose trailing slots are params,
// in order; calling the value binds the arguments to those slots (missing → null,
// surplus ignored) and runs the body under the recursion-depth limit.
func FuncValue(params []string, body CompiledExpr) CompiledExpr {
	arity := len(params)
	return func(s *Scope) (value.Value, error) {
		captured := s
		return &value.Function{
			Arity: arity,
			Call: func(args []value.Value) (value.Value, error) {
				st := captured.st
				if err := st.enterCall(); err != nil {
					return nil, err
				}
				defer st.leaveCall()
				vals := make([]value.Value, arity)
				for i := range vals {
					if i < len(args) {
						vals[i] = args[i]
					} else {
						vals[i] = value.Null
					}
				}
				return body(captured.Extend(vals...))
			},
		}, nil
	}
}

// bindNamedArgs resolves a call's arguments into params order. Calls to a known
// function may use positional or named arguments (not both); omitted parameters
// default to null.
func (c *compiler) bindNamedArgs(params []string, name string, n *CallExpr) []CompiledExpr {
	named, positional := false, false
	for _, a := range n.Args {
		if a.Name != "" {
			named = true
		} else {
			positional = true
		}
	}
	if named && positional {
		return c.nullCall()
	}
	if len(n.Args) > len(params) {
		return c.nullCall()
	}
	out := make([]CompiledExpr, len(params))
	for i := range out {
		out[i] = constNull
	}
	if !named {
		for i, a := range n.Args {
			out[i] = c.compile(a.Value)
		}
		return out
	}
	for _, a := range n.Args {
		idx := indexOf(params, a.Name)
		if idx < 0 {
			return c.nullCall()
		}
		out[idx] = c.compile(a.Value)
	}
	return out
}

// evalArgs evaluates each compiled argument against s.
func evalArgs(argExprs []CompiledExpr, s *Scope) ([]value.Value, error) {
	vals := make([]value.Value, len(argExprs))
	for i, ae := range argExprs {
		v, err := ae(s)
		if err != nil {
			return nil, err
		}
		vals[i] = v
	}
	return vals, nil
}

// bindArgs resolves a call's arguments to compiled expressions in parameter
// order, checking arity. Positional and named arguments may not be mixed.
func (c *compiler) bindArgs(b *builtins.Builtin, n *CallExpr) []CompiledExpr {
	named, positional := false, false
	for _, a := range n.Args {
		if a.Name != "" {
			named = true
		} else {
			positional = true
		}
	}
	if named && positional {
		return c.nullCall()
	}

	count := len(n.Args)
	if count < b.MinArgs || (!b.Variadic() && count > b.MaxArgs) {
		return c.nullCall()
	}

	if !named {
		out := make([]CompiledExpr, count)
		for i, a := range n.Args {
			out[i] = c.compile(a.Value)
		}
		return out
	}

	// Named arguments: place each at its parameter index; omitted parameters
	// default to null so optional trailing parameters work.
	if len(b.Params) == 0 {
		return c.nullCall()
	}
	out := make([]CompiledExpr, len(b.Params))
	for i := range out {
		out[i] = constNull
	}
	for _, a := range n.Args {
		idx := indexOf(b.Params, a.Name)
		if idx < 0 {
			return c.nullCall()
		}
		out[idx] = c.compile(a.Value)
	}
	return out
}

func indexOf(ss []string, s string) int {
	for i, v := range ss {
		if v == s {
			return i
		}
	}
	return -1
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

// feelEqualOp is the `=` operator. It follows value.Equal, except that comparing
// two non-null values of different types is undefined (null), not false
// (DMN §10.3.2.7). The internal value.Equal predicate keeps its boolean result
// for list membership, decision-table matching and the like.
func feelEqualOp(a, b value.Value) value.Value {
	if !value.IsNull(a) && !value.IsNull(b) && a.Kind() != b.Kind() {
		return value.Null
	}
	return value.Equal(a, b)
}

// notBool negates a three-valued boolean, propagating null.
func notBool(v value.Value) value.Value {
	switch v {
	case value.True:
		return value.False
	case value.False:
		return value.True
	default:
		return value.Null
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
	switch tv := t.(type) {
	case value.Range:
		return rangeContains(tv, x)
	case value.List:
		// A list on the right of `in` is a list of positive unary tests: x matches
		// if it is contained in a range element or equals any other element (FEEL
		// membership). E.g. 1 in [2,3,1] and 1 in [[2..4],[1..3]] are both true.
		for _, e := range tv.Elements {
			if r, ok := e.(value.Range); ok {
				if rangeContains(r, x) {
					return true
				}
			} else if value.Equal(x, e) == value.True {
				return true
			}
		}
		return false
	default:
		return value.Equal(x, t) == value.True
	}
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
