package feel

import (
	"github.com/pblumer/temis/internal/feel/builtins"
	"github.com/pblumer/temis/internal/value"
)

// itemVar is the implicit name bound to the current element inside a filter
// predicate (e.g. nums[item > 2]). Context elements additionally expose their
// keys directly via the compiler's implicit-context resolution.
const itemVar = "item"

// compiledIter is one compiled for/some/every clause: the domain expression and
// the scope slot its loop variable occupies in the (extended) body scope.
type compiledIter struct {
	domain CompiledExpr
	slot   int
}

// compileIterators compiles each `name in domain` clause, extending the env with
// one fresh slot per loop variable. A later clause's domain sees the earlier
// loop variables (FEEL iterators are nested, not independent), so domains are
// compiled progressively. It returns the clauses and the env the body must
// compile against.
func (c *compiler) compileIterators(its []Iterator) ([]compiledIter, *Env) {
	env := c.env
	out := make([]compiledIter, len(its))
	for i, it := range its {
		var dom CompiledExpr
		c.withEnv(env, func() { dom = c.compile(it.In) })
		env = env.Append(it.Name)
		slot, _ := env.slot(it.Name)
		out[i] = compiledIter{domain: dom, slot: slot}
	}
	return out, env
}

// compileForExpr compiles `for it... return body` into a list comprehension over
// the cartesian product of the iterator domains.
func (c *compiler) compileForExpr(n *ForExpr) CompiledExpr {
	iters, bodyEnv := c.compileIterators(n.Iterators)
	var body CompiledExpr
	c.withEnv(bodyEnv, func() { body = c.compile(n.Return) })

	return func(s *Scope) (value.Value, error) {
		var out []value.Value
		err := iterate(s, iters, 0, func(sc *Scope) error {
			v, err := body(sc)
			if err != nil {
				return err
			}
			out = append(out, v)
			return nil
		})
		if err != nil {
			return nil, err
		}
		return value.NewList(out...), nil
	}
}

// compileQuantified compiles `some|every it... satisfies cond`. some is the
// three-valued OR over the satisfied results, every the three-valued AND: a
// false dominates every, a true dominates some, and unknowns otherwise yield
// null.
func (c *compiler) compileQuantified(n *QuantifiedExpr) CompiledExpr {
	iters, bodyEnv := c.compileIterators(n.Iterators)
	var cond CompiledExpr
	c.withEnv(bodyEnv, func() { cond = c.compile(n.Satisfies) })
	some := n.Quant == "some"

	return func(s *Scope) (value.Value, error) {
		sawTrue, sawFalse, sawNull := false, false, false
		err := iterate(s, iters, 0, func(sc *Scope) error {
			v, err := cond(sc)
			if err != nil {
				return err
			}
			switch v {
			case value.True:
				sawTrue = true
			case value.False:
				sawFalse = true
			default:
				sawNull = true
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		if some {
			switch {
			case sawTrue:
				return value.True, nil
			case sawNull:
				return value.Null, nil
			default:
				return value.False, nil
			}
		}
		switch {
		case sawFalse:
			return value.False, nil
		case sawNull:
			return value.Null, nil
		default:
			return value.True, nil
		}
	}
}

// iterate drives the nested loops over iters starting at clause i, extending the
// scope with each domain element and invoking yield at the innermost level.
func iterate(s *Scope, iters []compiledIter, i int, yield func(*Scope) error) error {
	if i == len(iters) {
		return yield(s)
	}
	dv, err := iters[i].domain(s)
	if err != nil {
		return err
	}
	for _, e := range iterateDomain(dv) {
		if err := iterate(s.Extend(e), iters, i+1, yield); err != nil {
			return err
		}
	}
	return nil
}

// iterateDomain yields the elements a for/some/every clause ranges over: a list
// is taken element-wise, a numeric range expands to its integer steps (ascending
// or descending, both ends inclusive), null is empty, and any other single value
// is treated as a one-element domain.
func iterateDomain(v value.Value) []value.Value {
	switch x := v.(type) {
	case value.List:
		return x.Elements
	case value.Range:
		return rangeSteps(x)
	case nil:
		return nil
	default:
		if value.IsNull(v) {
			return nil
		}
		return []value.Value{v}
	}
}

// rangeSteps expands an integer numeric range into its successive values. A
// non-integer or unbounded range yields nothing.
func rangeSteps(r value.Range) []value.Value {
	lo, ok := integerOf(r.Low)
	if !ok {
		return nil
	}
	hi, ok := integerOf(r.High)
	if !ok {
		return nil
	}
	var out []value.Value
	if lo <= hi {
		for i := lo; i <= hi; i++ {
			out = append(out, value.NumberFromInt64(i))
		}
	} else {
		for i := lo; i >= hi; i-- {
			out = append(out, value.NumberFromInt64(i))
		}
	}
	return out
}

// integerOf returns v as an int64 when it is an integral number.
func integerOf(v value.Value) (int64, bool) {
	n, ok := v.(value.Number)
	if !ok {
		return 0, false
	}
	return n.Int64()
}

// compileFilter compiles `X[F]`. F is compiled against an env extended with the
// implicit element variable item; its keys (for context elements) resolve
// dynamically. At runtime a numeric F indexes the collection (1-based, negative
// from the end) while any other F is a per-element boolean predicate.
func (c *compiler) compileFilter(n *FilterExpr) CompiledExpr {
	x := c.compile(n.X)
	f := c.compileFilterPredicate(n.Filter)
	return filterClosure(x, f)
}

// compileFilterPredicate compiles a filter predicate against the current env
// extended with the implicit element variable item; the element's context keys
// resolve dynamically while it is the innermost implicit scope.
func (c *compiler) compileFilterPredicate(pred Expr) CompiledExpr {
	filterEnv := c.env.Append(itemVar)
	itemSlot, _ := filterEnv.slot(itemVar)
	var f CompiledExpr
	c.withEnv(filterEnv, func() {
		c.implicit = append(c.implicit, itemSlot)
		f = c.compile(pred)
		c.implicit = c.implicit[:len(c.implicit)-1]
	})
	return f
}

// filterClosure is the runtime of a filter: x is the collection, f the predicate
// compiled against an env extended with the implicit item slot (so the predicate
// reads the current element via s.Extend). A numeric predicate selects by index
// (1-based, negative from the end); any other predicate is a per-element boolean.
func filterClosure(x, f CompiledExpr) CompiledExpr {
	return func(s *Scope) (value.Value, error) {
		xv, err := x(s)
		if err != nil {
			return nil, err
		}
		elems := asElements(xv)

		probe, err := f(s.Extend(value.Null))
		if err != nil {
			return nil, err
		}
		if idx, ok := integerOf(probe); ok {
			return indexElements(elems, idx), nil
		}

		var out []value.Value
		for _, e := range elems {
			r, err := f(s.Extend(e))
			if err != nil {
				return nil, err
			}
			if r == value.True {
				out = append(out, e)
			}
		}
		return value.NewList(out...), nil
	}
}

// ForOne builds a boxed `for` over a single iterator: coll yields the domain
// (list/range/single value, per the iteration rules) and body — compiled against
// an env with the iterator variable appended as its trailing slot — runs for each
// element, collecting the results into a list (WP-26).
func ForOne(coll, body CompiledExpr) CompiledExpr {
	return func(s *Scope) (value.Value, error) {
		cv, err := coll(s)
		if err != nil {
			return nil, err
		}
		var out []value.Value
		for _, e := range iterateDomain(cv) {
			v, err := body(s.Extend(e))
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return value.NewList(out...), nil
	}
}

// QuantifyOne builds a boxed `some` (some=true) or `every` (some=false) over a
// single iterator, applying FEEL's three-valued semantics: some is true on any
// satisfied element, every false on any unsatisfied one, with unknowns otherwise
// yielding null (WP-26). pred is compiled against an env with the iterator
// variable appended as its trailing slot.
func QuantifyOne(some bool, coll, pred CompiledExpr) CompiledExpr {
	return func(s *Scope) (value.Value, error) {
		cv, err := coll(s)
		if err != nil {
			return nil, err
		}
		sawTrue, sawFalse, sawNull := false, false, false
		for _, e := range iterateDomain(cv) {
			v, err := pred(s.Extend(e))
			if err != nil {
				return nil, err
			}
			switch v {
			case value.True:
				sawTrue = true
			case value.False:
				sawFalse = true
			default:
				sawNull = true
			}
		}
		if some {
			switch {
			case sawTrue:
				return value.True, nil
			case sawNull:
				return value.Null, nil
			default:
				return value.False, nil
			}
		}
		switch {
		case sawFalse:
			return value.False, nil
		case sawNull:
			return value.Null, nil
		default:
			return value.True, nil
		}
	}
}

// BoxedFilter compiles a boxed filter: coll is the already-compiled collection
// and matchSrc the FEEL predicate text, compiled against env extended with the
// implicit element variable item (its context keys resolve directly, e.g.
// `age > 18`). A numeric predicate selects by index (WP-26).
func BoxedFilter(coll CompiledExpr, matchSrc string, env *Env, funcs map[string]*Func) (CompiledExpr, error) {
	pred, err := ParseWithNames(matchSrc, nameOracle(funcs))
	if err != nil {
		return nil, err
	}
	c := &compiler{env: env, builtins: builtins.Default(), funcs: funcs}
	f := c.compileFilterPredicate(pred)
	if c.err != nil {
		return nil, c.err
	}
	return filterClosure(coll, f), nil
}

// asElements views a value as a list of elements for filtering: a list as-is,
// null as empty, and any other value as a single-element list.
func asElements(v value.Value) []value.Value {
	if l, ok := v.(value.List); ok {
		return l.Elements
	}
	if value.IsNull(v) {
		return nil
	}
	return []value.Value{v}
}

// indexElements returns the 1-based element at idx (negative counts from the
// end), or null when out of range.
func indexElements(elems []value.Value, idx int64) value.Value {
	n := int64(len(elems))
	switch {
	case idx > 0 && idx <= n:
		return elems[idx-1]
	case idx < 0 && -idx <= n:
		return elems[n+idx]
	default:
		return value.Null
	}
}
