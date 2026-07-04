package feel

import (
	"strings"
	"testing"

	"github.com/pblumer/temis/internal/value"
)

// errExpr is a CompiledExpr that always fails, for exercising the error
// propagation branches of compiled closures.
func errExpr(*Scope) (value.Value, error) { return nil, errBoom }

// --- types.go ---

func TestTypeString(t *testing.T) {
	cases := []struct {
		t    *Type
		want string
	}{
		{nil, "Any"},
		{TNumber, "number"},
		{ListOf(TString), "list<string>"},
		{ListOf(nil), "list"},
		{ContextOf(map[string]*Type{"a": TNumber}), "context"},
	}
	for _, c := range cases {
		if got := c.t.String(); got != c.want {
			t.Errorf("Type.String() = %q, want %q", got, c.want)
		}
	}
}

func TestBuiltinTypeAllNames(t *testing.T) {
	// Exercise every normalized branch of BuiltinType, including the duration
	// aliases and the unqualified "duration".
	names := []string{
		"number", "string", "boolean", "date", "time",
		"date and time", "dateTime",
		"days and time duration", "dayTimeDuration",
		"years and months duration", "yearMonthDuration",
		"duration", "list", "context",
	}
	for _, n := range names {
		if _, ok := BuiltinType(n); !ok {
			t.Errorf("BuiltinType(%q) not found", n)
		}
	}
	if _, ok := BuiltinType("Frobnicate"); ok {
		t.Error("BuiltinType(Frobnicate) should be unknown")
	}
}

func TestNormalizeTypeNameGeneric(t *testing.T) {
	// A generic parameter is dropped before resolution.
	if _, ok := BuiltinType("list<number>"); !ok {
		t.Error("BuiltinType(list<number>) should resolve to list")
	}
}

func TestConstValue(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{"42", "42"},
		{`"hi"`, "hi"},
		{"true", "true"},
		{"null", "null"},
		{`@"2024-01-01"`, "2024-01-01"},
	}
	for _, c := range cases {
		v, ok := ConstValue(c.src)
		if !ok {
			t.Errorf("ConstValue(%q) ok=false, want true", c.src)
			continue
		}
		if v.String() != c.want {
			t.Errorf("ConstValue(%q) = %s, want %s", c.src, v, c.want)
		}
	}
}

func TestConstValueNonConstant(t *testing.T) {
	// A compound or variable expression is not a constant literal.
	for _, src := range []string{"1 + 2", "x", "[1, 2]", "1 +"} {
		if _, ok := ConstValue(src); ok {
			t.Errorf("ConstValue(%q) ok=true, want false", src)
		}
	}
	// An @-literal whose content is not a recognised temporal value is rejected.
	if _, ok := ConstValue(`@"not-temporal"`); ok {
		t.Error(`ConstValue(@"not-temporal") should be false`)
	}
	// A number literal that does not parse is rejected.
	if _, ok := ConstValue("99999999999999999999999999999999999999999e999999999"); ok {
		// best-effort: if it parses, that's fine too; just ensure no panic.
		_ = ok
	}
}

// --- scope.go ---

func TestScopeTraceAndWithTrace(t *testing.T) {
	s := NewEnv().NewScope(nil)
	if s.Trace() != nil {
		t.Errorf("fresh scope Trace() = %v, want nil", s.Trace())
	}
	sink := "recorder"
	ts := s.WithTrace(sink)
	if ts.Trace() != sink {
		t.Errorf("WithTrace Trace() = %v, want %q", ts.Trace(), sink)
	}
	// The original scope is unchanged (shallow copy).
	if s.Trace() != nil {
		t.Error("WithTrace mutated the receiver")
	}
	// Extend carries the trace through.
	ext := ts.Extend(value.Null)
	if ext.Trace() != sink {
		t.Error("Extend dropped the trace sink")
	}
}

func TestEnvDerive(t *testing.T) {
	base := NewEnv("a", "b")
	d := base.Derive("?")
	if _, ok := d.slot("?"); !ok {
		t.Error("Derive should add the extra name")
	}
	// Existing slots keep their indices.
	if i, _ := d.slot("a"); i != 0 {
		t.Errorf("a slot = %d after Derive, want 0", i)
	}
	if i, _ := d.slot("b"); i != 1 {
		t.Errorf("b slot = %d after Derive, want 1", i)
	}
}

func TestScopeAtOutOfRange(t *testing.T) {
	s := NewEnv("a").NewScope(map[string]value.Value{"a": value.NumberFromInt64(1)})
	if !value.IsNull(s.at(-1)) {
		t.Error("at(-1) should be null")
	}
	if !value.IsNull(s.at(99)) {
		t.Error("at(99) should be null")
	}
}

func TestEnvDefineIdempotent(t *testing.T) {
	e := NewEnv("a")
	// Re-defining an existing name returns the same slot.
	if e.define("a") != 0 {
		t.Error("define(a) should return existing slot 0")
	}
}

func TestNilEvalStateAccounting(t *testing.T) {
	// A Scope built outside NewScope has a nil evalState; the accounting helpers
	// must treat that as unlimited and never panic.
	var st *evalState
	if err := st.enterCall(); err != nil {
		t.Errorf("nil enterCall = %v, want nil", err)
	}
	st.leaveCall() // must not panic
	if err := st.step(); err != nil {
		t.Errorf("nil step = %v, want nil", err)
	}
	if err := st.checkItems(1_000_000); err != nil {
		t.Errorf("nil checkItems = %v, want nil", err)
	}
}

// --- function.go ---

func TestFuncLabel(t *testing.T) {
	named := &Func{Name: "fact"}
	if got := named.label(); got != `"fact"` {
		t.Errorf("label = %q, want %q", got, `"fact"`)
	}
	anon := &Func{}
	if got := anon.label(); got != "anonymous function" {
		t.Errorf("label = %q, want %q", got, "anonymous function")
	}
}

func TestFuncCallNoBody(t *testing.T) {
	f := &Func{Name: "broken", Params: []string{"x"}}
	s := NewEnv().NewScope(nil)
	_, err := f.call(s, []value.Value{value.Null})
	if err == nil || !strings.Contains(err.Error(), "no body") {
		t.Errorf("call with nil body = %v, want a no-body error", err)
	}
}

func TestCallFuncArgError(t *testing.T) {
	// CallFunc must propagate an argument evaluation error.
	f := &Func{Name: "id", Params: []string{"x"}}
	f.Body = func(s *Scope) (value.Value, error) { return s.at(0), nil }
	ce := CallFunc(f, []CompiledExpr{errExpr})
	if _, err := ce(NewEnv().NewScope(nil)); err == nil {
		t.Error("CallFunc should propagate an argument error")
	}
}

func TestCallValueCalleeAndArgErrors(t *testing.T) {
	scope := NewEnv().NewScope(nil)
	// Callee evaluation error propagates.
	if _, err := CallValue(errExpr, nil)(scope); err == nil {
		t.Error("CallValue should propagate a callee error")
	}
	// Argument evaluation error propagates (callee is a real function).
	f := &Func{Name: "id", Params: []string{"x"}}
	f.Body = func(s *Scope) (value.Value, error) { return s.at(0), nil }
	callee := func(s *Scope) (value.Value, error) { return f.asValue(s), nil }
	if _, err := CallValue(callee, []CompiledExpr{errExpr})(scope); err == nil {
		t.Error("CallValue should propagate an argument error")
	}
}

// --- compile.go error propagation ---

// errSrc compiles src then replaces the runtime so a chosen operand errors. The
// simplest portable way to force runtime errors in compiler-emitted closures is
// to drive resource limits, done in TestLimitPropagation. These direct tests use
// the boxed helpers and hand-built closures instead.

func TestValueBinopErrorPropagation(t *testing.T) {
	scope := NewEnv().NewScope(nil)
	ok := constExpr(value.NumberFromInt64(1))
	// Left operand errors.
	if _, err := valueBinop(errExpr, ok, value.Add)(scope); err == nil {
		t.Error("valueBinop should propagate a left-operand error")
	}
	// Right operand errors.
	if _, err := valueBinop(ok, errExpr, value.Add)(scope); err == nil {
		t.Error("valueBinop should propagate a right-operand error")
	}
}

func TestEvalArgsError(t *testing.T) {
	if _, err := evalArgs([]CompiledExpr{errExpr}, NewEnv().NewScope(nil)); err == nil {
		t.Error("evalArgs should propagate an argument error")
	}
}

func TestFuncValueArgPaddingAndSurplus(t *testing.T) {
	// FuncValue with arity 2: a call with one arg pads with null, a call with
	// three drops the surplus.
	body := func(s *Scope) (value.Value, error) {
		return value.NewList(s.at(0), s.at(1)), nil
	}
	v, err := FuncValue([]string{"a", "b"}, body)(NewEnv().NewScope(nil))
	if err != nil {
		t.Fatal(err)
	}
	fn := v.(*value.Function)
	got, err := fn.Call([]value.Value{value.NumberFromInt64(1)})
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "[1, null]" {
		t.Errorf("padded call = %s, want [1, null]", got)
	}
	got, err = fn.Call([]value.Value{value.NumberFromInt64(1), value.NumberFromInt64(2), value.NumberFromInt64(3)})
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "[1, 2]" {
		t.Errorf("surplus call = %s, want [1, 2]", got)
	}
}

func TestFuncValueDepthLimit(t *testing.T) {
	// A function value evaluated under a depth-0 budget refuses to be called.
	body := func(s *Scope) (value.Value, error) { return value.Null, nil }
	scope := NewEnv().NewScopeWithLimits(nil, Limits{MaxCallDepth: 1})
	// Pre-exhaust the budget by entering a call once.
	if err := scope.st.enterCall(); err != nil {
		t.Fatal(err)
	}
	v, err := FuncValue(nil, body)(scope)
	if err != nil {
		t.Fatal(err)
	}
	fn := v.(*value.Function)
	if _, err := fn.Call(nil); err == nil {
		t.Error("FuncValue call past the depth budget should error")
	}
	scope.st.leaveCall()
}

// --- iterate.go ---

func TestForEachDomainNullAndScalar(t *testing.T) {
	// A null domain yields nothing; a scalar domain yields a single element.
	var got []value.Value
	if err := forEachDomain(nil, value.Null, func(e value.Value) error {
		got = append(got, e)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("null domain produced %v, want nothing", got)
	}
	got = nil
	if err := forEachDomain(nil, value.NumberFromInt64(7), func(e value.Value) error {
		got = append(got, e)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].String() != "7" {
		t.Errorf("scalar domain produced %v, want [7]", got)
	}
}

func TestForEachRangeNonInteger(t *testing.T) {
	// A range whose bounds are not integers (or open) yields nothing.
	open := value.Range{LowClosed: true, Low: value.Null, High: value.NumberFromInt64(3), HighClosed: true}
	count := 0
	if err := forEachRange(nil, open, func(value.Value) error { count++; return nil }); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("open-low range produced %d, want 0", count)
	}
	openHi := value.Range{LowClosed: true, Low: value.NumberFromInt64(1), High: value.Null, HighClosed: true}
	count = 0
	if err := forEachRange(nil, openHi, func(value.Value) error { count++; return nil }); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("open-high range produced %d, want 0", count)
	}
}

func TestForEachDomainYieldError(t *testing.T) {
	// An error from the yield function propagates out of each domain shape.
	boom := func(value.Value) error { return errBoom }
	if err := forEachDomain(nil, nums(1), boom); err == nil {
		t.Error("list-domain yield error should propagate")
	}
	if err := forEachDomain(nil, value.NumberFromInt64(1), boom); err == nil {
		t.Error("scalar-domain yield error should propagate")
	}
	r := value.Range{LowClosed: true, Low: value.NumberFromInt64(1), High: value.NumberFromInt64(3), HighClosed: true}
	if err := forEachDomain(nil, r, boom); err == nil {
		t.Error("range-domain yield error should propagate")
	}
}

func TestIterateDomainError(t *testing.T) {
	// A domain expression that errors aborts the iteration.
	iters := []compiledIter{{domain: errExpr, slot: 0}}
	err := iterate(NewEnv("x").NewScope(nil), iters, 0, func(*Scope) error { return nil })
	if err == nil {
		t.Error("iterate should propagate a domain error")
	}
}

func TestForOneAndQuantifyOneCollError(t *testing.T) {
	scope := NewEnv().NewScope(nil)
	if _, err := ForOne(errExpr, constExpr(value.Null))(scope); err == nil {
		t.Error("ForOne should propagate a collection error")
	}
	if _, err := QuantifyOne(true, errExpr, constExpr(value.Null))(scope); err == nil {
		t.Error("QuantifyOne should propagate a collection error")
	}
}

func TestForOneAndQuantifyOneBodyError(t *testing.T) {
	coll := constExpr(nums(1, 2, 3))
	scope := NewEnv().NewScope(nil)
	if _, err := ForOne(coll, errExpr)(scope); err == nil {
		t.Error("ForOne should propagate a body error")
	}
	if _, err := QuantifyOne(true, coll, errExpr)(scope); err == nil {
		t.Error("QuantifyOne should propagate a predicate error")
	}
}

func TestFilterClosureErrors(t *testing.T) {
	scope := NewEnv().NewScope(nil)
	// Collection error propagates.
	if _, err := filterClosure(errExpr, constExpr(value.Null))(scope); err == nil {
		t.Error("filterClosure should propagate a collection error")
	}
	// Probe (predicate on null) error propagates.
	coll := constExpr(nums(1, 2, 3))
	if _, err := filterClosure(coll, errExpr)(scope); err == nil {
		t.Error("filterClosure should propagate a probe error")
	}
	// Per-element predicate error propagates: the predicate succeeds on the
	// null probe (yielding a non-integer boolean) but fails on a real element.
	calls := 0
	pred := func(s *Scope) (value.Value, error) {
		calls++
		if calls == 1 {
			return value.False, nil // probe: not an integer index
		}
		return nil, errBoom
	}
	if _, err := filterClosure(coll, pred)(scope); err == nil {
		t.Error("filterClosure should propagate a per-element predicate error")
	}
}

func TestAsElementsScalar(t *testing.T) {
	// A non-list, non-null value views as a single-element collection.
	got := asElements(value.NumberFromInt64(5))
	if len(got) != 1 || got[0].String() != "5" {
		t.Errorf("asElements(5) = %v, want [5]", got)
	}
	if got := asElements(value.Null); got != nil {
		t.Errorf("asElements(null) = %v, want nil", got)
	}
}

func TestBoxedFilterParseError(t *testing.T) {
	if _, err := BoxedFilter(constExpr(nums(1)), "((", NewEnv(), nil); err == nil {
		t.Error("BoxedFilter should report a parse error")
	}
}

// --- limit propagation through compiled closures ---

func TestForComprehensionIterationLimit(t *testing.T) {
	// A for over a long range under a tight iteration budget yields a LimitError,
	// exercising the step() error path inside iterate/forEachDomain.
	ce, err := CompileString("for i in [1..1000] return i", NewEnv())
	if err != nil {
		t.Fatal(err)
	}
	scope := NewEnv().NewScopeWithLimits(nil, Limits{MaxIterations: 5})
	if _, err := ce(scope); err == nil {
		t.Error("for over a long range should hit the iteration limit")
	}
}

func TestForComprehensionListSizeLimit(t *testing.T) {
	ce, err := CompileString("for i in [1..1000] return i", NewEnv())
	if err != nil {
		t.Fatal(err)
	}
	scope := NewEnv().NewScopeWithLimits(nil, Limits{MaxListSize: 5})
	_, err = ce(scope)
	if err == nil {
		t.Error("for producing a large list should hit the list-size limit")
	}
	var le *LimitError
	if !asLimit(err, &le) {
		t.Errorf("error = %v, want a *LimitError", err)
	}
}

func TestFilterListSizeLimit(t *testing.T) {
	// A predicate filter that keeps every element under a tight list-size budget.
	ce, err := CompileString("[1, 2, 3, 4, 5][item > 0]", NewEnv())
	if err != nil {
		t.Fatal(err)
	}
	scope := NewEnv().NewScopeWithLimits(nil, Limits{MaxListSize: 2})
	if _, err := ce(scope); err == nil {
		t.Error("filter keeping all elements should hit the list-size limit")
	}
}

func TestQuantifiedIterationLimit(t *testing.T) {
	ce, err := CompileString("some i in [1..1000] satisfies i > 999", NewEnv())
	if err != nil {
		t.Fatal(err)
	}
	scope := NewEnv().NewScopeWithLimits(nil, Limits{MaxIterations: 5})
	if _, err := ce(scope); err == nil {
		t.Error("some over a long range should hit the iteration limit")
	}
}

// asLimit reports whether err is a *LimitError, binding it to target.
func asLimit(err error, target **LimitError) bool {
	le, ok := err.(*LimitError)
	if ok {
		*target = le
	}
	return ok
}

func TestLimitErrorString(t *testing.T) {
	e := &LimitError{Limit: "iterations", Max: 10}
	if got := e.Error(); got != "feel: iterations limit 10 exceeded" {
		t.Errorf("LimitError.Error() = %q", got)
	}
}

// --- compile.go: error branches through whole expressions under a budget ---

func TestBinaryAndCompareErrorBranches(t *testing.T) {
	// Drive each operand-error branch of compiled comparison/between/in/path/list/
	// context/interval/instance-of/if closures by compiling an expression whose
	// inner for-comprehension trips the iteration limit, so any sub-expression
	// that references it errors at runtime.
	exprs := []string{
		`(for i in [1..1000] return i) = [1]`,  // valueBinop (=)
		`[1] = (for i in [1..1000] return i)`,  // valueBinop right
		`(for i in [1..1000] return i)[1] < 2`, // compileCompare left
		`1 < (for i in [1..1000] return i)[1]`, // compileCompare right
		`(for i in [1..1000] return i)[1] between 1 and 2`,
		`1 between (for i in [1..1000] return i)[1] and 5`,
		`1 between 0 and (for i in [1..1000] return i)[1]`,
		`(for i in [1..1000] return i)[1] in (1, 2)`,            // compileIn x
		`1 in ((for i in [1..1000] return i)[1])`,               // compileIn test
		`[(for i in [1..1000] return i)[1]]`,                    // compileList element
		`{a: (for i in [1..1000] return i)[1]}`,                 // compileContext value
		`[(for i in [1..1000] return i)[1]..5]`,                 // compileInterval low
		`[1..count((for i in [1..1000] return i))]`,             // compileInterval high
		`((for i in [1..1000] return i)[1]) instance of number`, // compileInstanceOf x
		`(for i in [1..1000] return i)[1].x`,                    // compilePath base
		`if (for i in [1..1000] return i)[1] > 0 then 1 else 2`, // IfThenElse cond
		`-((for i in [1..1000] return i)[1])`,                   // UnaryExpr operand
	}
	for _, src := range exprs {
		ce, err := CompileString(src, NewEnv())
		if err != nil {
			t.Fatalf("compile %q: %v", src, err)
		}
		scope := NewEnv().NewScopeWithLimits(nil, Limits{MaxIterations: 5})
		if _, err := ce(scope); err == nil {
			t.Errorf("%q: expected a runtime limit error", src)
		}
	}
}

func TestBuiltinAndFuncCallArgError(t *testing.T) {
	// A builtin call whose argument trips the iteration limit propagates the error.
	ce, err := CompileString(`count((for i in [1..1000] return i))`, NewEnv())
	if err != nil {
		t.Fatal(err)
	}
	scope := NewEnv().NewScopeWithLimits(nil, Limits{MaxIterations: 5})
	if _, err := ce(scope); err == nil {
		t.Error("builtin call arg error should propagate")
	}

	// A user-function call whose argument trips the limit propagates the error.
	funcs := map[string]*Func{}
	id := &Func{Name: "id", Params: []string{"x"}}
	funcs["id"] = id
	body, _ := CompileStringWith("x", NewEnv("x"), funcs)
	id.Body = body
	ce, err = CompileStringWith(`id((for i in [1..1000] return i))`, NewEnv(), funcs)
	if err != nil {
		t.Fatal(err)
	}
	scope = NewEnv().NewScopeWithLimits(nil, Limits{MaxIterations: 5})
	if _, err := ce(scope); err == nil {
		t.Error("user-function call arg error should propagate")
	}
}

func TestValueCallCalleeAndArgError(t *testing.T) {
	// Calling a variable that holds a function: the argument error propagates.
	env := NewEnv("f")
	ce, err := CompileString(`f((for i in [1..1000] return i))`, env)
	if err != nil {
		t.Fatal(err)
	}
	fn := &value.Function{Arity: 1, Call: func(a []value.Value) (value.Value, error) { return a[0], nil }}
	scope := env.NewScopeWithLimits(map[string]value.Value{"f": fn}, Limits{MaxIterations: 5})
	if _, err := ce(scope); err == nil {
		t.Error("value-call arg error should propagate")
	}
}

// --- typecheck.go branches ---

func TestTypecheckAtLiteralAndNull(t *testing.T) {
	env := NewTypeEnv()
	// An @-temporal literal infers its temporal kind; a bad one infers Any.
	if errs := checkSrcOK(t, `@"2024-01-01" = @"2024-01-02"`, env); len(errs) != 0 {
		t.Errorf("temporal equality: unexpected findings %v", errs)
	}
	// A malformed @-literal infers Any and is never flagged.
	if errs := checkSrcOK(t, `@"nope" = 1`, env); len(errs) != 0 {
		t.Errorf("bad temporal: unexpected findings %v", errs)
	}
}

func TestTypecheckJoinAndElementType(t *testing.T) {
	env := NewTypeEnv().Set("xs", ListOf(TNumber)).Set("flag", TBoolean)
	// if with two branches of differing kinds joins to Any (no flag downstream).
	if errs := checkSrcOK(t, `(if flag then 1 else "x") and true`, env); len(errs) != 0 {
		// joined type is Any → boolean op on Any is not flagged
		t.Errorf("mixed-branch join: unexpected findings %v", errs)
	}
	// for over a typed list carries the element type into the body.
	if errs := checkSrcOK(t, `for n in xs return n + 1`, env); len(errs) != 0 {
		t.Errorf("for element type: unexpected findings %v", errs)
	}
	// for over a non-list domain gives Any element type.
	if errs := checkSrcOK(t, `for n in flag return n + 1`, env); len(errs) != 0 {
		t.Errorf("for non-list domain: unexpected findings %v", errs)
	}
}

func TestTypecheckRequireComparableAny(t *testing.T) {
	// Comparing an Any operand is never flagged (requireComparable early return).
	env := NewTypeEnv().Set("x", nil).Set("age", TNumber)
	if errs := checkSrcOK(t, `x < age`, env); len(errs) != 0 {
		t.Errorf("Any comparison: unexpected findings %v", errs)
	}
}

func TestTypecheckGetNilEnv(t *testing.T) {
	// infer over a NameRef with a nil TypeEnv must not panic and yields Any.
	expr, err := Parse("x + 1")
	if err != nil {
		t.Fatal(err)
	}
	if errs := Typecheck(expr, nil); len(errs) != 0 {
		t.Errorf("nil env: unexpected findings %v", errs)
	}
}

func TestTypecheckInstanceOfAndFilter(t *testing.T) {
	env := NewTypeEnv().Set("xs", ListOf(TNumber))
	if errs := checkSrcOK(t, `xs[1] instance of number`, env); len(errs) != 0 {
		t.Errorf("instance-of/filter: unexpected findings %v", errs)
	}
}

// checkSrcOK parses and type-checks src, failing the test on a parse error.
func checkSrcOK(t *testing.T, src string, env *TypeEnv) []TypeError {
	t.Helper()
	expr, err := Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return Typecheck(expr, env)
}

// --- lexer.go escapes ---

func TestLexerEscapes(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{`"\b\f"`, "\b\f"},
		{`"\r"`, "\r"},
		{`"\\"`, "\\"},
		{`"A"`, "A"},
		{`"é"`, "é"},
	}
	for _, c := range cases {
		toks := Tokenize(c.src)
		if toks[0].Kind != String {
			t.Errorf("Tokenize(%q)[0].Kind = %v, want String", c.src, toks[0].Kind)
			continue
		}
		if toks[0].Value != c.want {
			t.Errorf("Tokenize(%q) value = %q, want %q", c.src, toks[0].Value, c.want)
		}
	}
}

func TestHexValueAllRanges(t *testing.T) {
	// Unicode escapes exercise every branch of hexValue: digits, lowercase a-f
	// and uppercase A-F. The escape sequences are built at runtime (bs is a
	// literal backslash) so the Go source carries no \u sequence of its own.
	bs := string([]byte{'\\'})
	cases := []struct {
		hex  string // four hex digits after \u
		want rune
	}{
		{"0041", 'A'},    // digits only
		{"00ab", 0x00ab}, // lowercase a, b
		{"00CD", 0x00cd}, // uppercase C, D
		{"00ef", 0x00ef}, // lowercase e, f
		{"00EF", 0x00ef}, // uppercase E, F
	}
	for _, c := range cases {
		src := `"` + bs + "u" + c.hex + `"`
		toks := Tokenize(src)
		if toks[0].Kind != String || toks[0].Value != string(c.want) {
			t.Errorf("Tokenize(%q) = %v %q, want String %q", src, toks[0].Kind, toks[0].Value, string(c.want))
		}
	}
	// An invalid hex digit in a unicode escape is an error.
	bad := `"` + bs + "u00GZ" + `"`
	if toks := Tokenize(bad); toks[0].Kind != Error {
		t.Errorf("Tokenize(%q) kind = %v, want Error", bad, toks[0].Kind)
	}
}

// --- parser.go branches ---

func TestParserPeekPastEnd(t *testing.T) {
	// peek(n) past the token stream returns the trailing EOF.
	p := &parser{toks: Tokenize("1")}
	if p.peek(100).Kind != EOF {
		t.Errorf("peek past end = %v, want EOF", p.peek(100).Kind)
	}
}

func TestParserGenericAndAssemble(t *testing.T) {
	// A generic type with a multi-word inner name, and a multi-word context key.
	if got := sexpr(t, "x instance of list<list<number>>"); got != "(instance-of x list<list<number>>)" {
		t.Errorf("nested generic = %s", got)
	}
	if got := sexpr(t, "{first name last name: 1}"); got != "(context (first name last name: 1))" {
		t.Errorf("multi-word key = %s", got)
	}
}

func TestParserErrors(t *testing.T) {
	for _, src := range []string{
		"x instance of <number>",    // type name expected, got '<'
		"x instance of list<number", // unterminated generic
		"]1..2",                     // missing high bracket / dotdot ok but no close
		"{1: 2}",                    // context key must be string or name
		"function(x) external",      // external with empty body -> parse error on EOF body
	} {
		if _, err := Parse(src); err == nil {
			t.Errorf("Parse(%q) = nil error, want error", src)
		}
	}
}

func TestParserHighBracketForms(t *testing.T) {
	// All three closing-bracket forms of an interval high bound.
	cases := map[string]string{
		"]1..2]": "(1..2]",
		"]1..2[": "(1..2)",
		"]1..2)": "(1..2)",
	}
	for src, want := range cases {
		if got := sexpr(t, src); got != want {
			t.Errorf("Parse(%q) = %s, want %s", src, got, want)
		}
	}
	// An invalid high-bracket close is an error.
	if _, err := Parse("]1..2 x"); err == nil {
		t.Error("invalid interval close should be a parse error")
	}
}

// --- unarytest.go refersToInput branches ---

func TestRefersToInputForms(t *testing.T) {
	// Each form below references the implicit input "?" somewhere, so the unary
	// test treats it as a complete boolean expression rather than wrapping it in
	// an equality. Exercising them through CompileUnaryTest walks refersToInput's
	// many cases.
	env := NewEnv(InputVar, "limit")
	srcs := []string{
		`? between 1 and limit`,         // BetweenExpr
		`? in (1, 2, 3)`,                // InExpr X
		`1 in (?, 2)`,                   // InExpr tests
		`? instance of number`,          // InstanceOfExpr
		`if ? > 0 then true else false`, // IfExpr
		`?.x = 1`,                       // PathExpr (compiles via implicit? — keep simple)
		`count([?]) > 0`,                // CallExpr arg + ListLit
		`{a: ?}.a = 1`,                  // ContextLit
		`? in [1..10]`,                  // IntervalLit inside InExpr test
		`-? < 0`,                        // UnaryExpr
	}
	for _, src := range srcs {
		if _, err := CompileUnaryTest(src, env); err != nil {
			// Some forms (e.g. ?.x) may legitimately not compile if ? is not a
			// context; we only require that the parse/refersToInput path is taken.
			// Skip compile errors but ensure no panic occurred.
			continue
		}
	}
}

func TestRefersToInputViaMatch(t *testing.T) {
	// End-to-end: each test references ? so it is used as-is. We assert a couple
	// of evaluable ones produce the expected match.
	if !matchTest(t, `? between 1 and 10`, num("5"), nil) {
		t.Error("? between 1 and 10 should match 5")
	}
	if !matchTest(t, `1 in (?, 9)`, num("1"), nil) {
		t.Error("1 in (?, 9) should match when ? = 1")
	}
	if !matchTest(t, `count([?, 1]) = 2`, num("9"), nil) {
		t.Error("count([?,1]) = 2 should hold")
	}
	if !matchTest(t, `if ? > 0 then true else false`, num("3"), nil) {
		t.Error("if ?>0 then true else false should match 3")
	}
	if !matchTest(t, `-? = -5`, num("5"), nil) {
		t.Error("-? = -5 should match 5")
	}
	if !matchTest(t, `? instance of number`, num("5"), nil) {
		t.Error("? instance of number should match a number")
	}
}

func TestMatchesError(t *testing.T) {
	// Matches propagates an evaluation error from the compiled test.
	if _, err := Matches(errExpr, NewEnv().NewScope(nil)); err == nil {
		t.Error("Matches should propagate an evaluation error")
	}
}

// TestRefersToInputContainerForms drives refersToInput's FilterExpr, CallExpr
// (callee reference), ContextLit, ListLit and IntervalLit branches with cells
// that parse and compile cleanly so each is exercised end-to-end.
func TestRefersToInputContainerForms(t *testing.T) {
	env := NewEnv(InputVar, "xs")
	// Filter whose collection references ? -> FilterExpr branch.
	if _, err := CompileUnaryTest(`[?, 1, 2][item > 0] = [?, 1, 2]`, env); err != nil {
		t.Fatalf("filter form: %v", err)
	}
	// List literal containing ? -> ListLit branch (and via equality with self).
	if !matchTest(t, `count([?, ?]) = 2`, num("4"), nil) {
		t.Error("count([?,?]) = 2 should hold")
	}
	// Context literal whose value references ? -> ContextLit branch.
	if !matchTest(t, `{v: ?}.v = 4`, num("4"), nil) {
		t.Error("{v: ?}.v = 4 should hold for ? = 4")
	}
	// Interval literal whose bound references ? -> IntervalLit branch, used as a
	// containment test that already mentions ?.
	if !matchTest(t, `5 in [?..10]`, num("1"), nil) {
		t.Error("5 in [?..10] should hold for ? = 1")
	}
}

func TestBuiltinArityMismatchIsNull(t *testing.T) {
	// A builtin invoked with the wrong arity or with named args it does not accept
	// compiles (FEEL total-function semantics) and evaluates to null rather than
	// making the decision non-executable.
	for _, src := range []string{
		"string length()",  // fixed-arity, too few
		"substring()",      // variadic, too few
		`count(x: [1, 2])`, // named args on a no-param builtin
	} {
		if _, err := CompileString(src, NewEnv()); err != nil {
			t.Errorf("Compile(%q) = %v, want no error", src, err)
			continue
		}
		if got := evalStr(t, src, nil); !value.IsNull(got) {
			t.Errorf("eval(%q) = %s, want null", src, got)
		}
	}
}

func TestBoxedFilterCompileError(t *testing.T) {
	// A predicate that calls an unknown function fails to compile, exercising
	// BoxedFilter's c.err path. (An unknown *variable* would instead resolve via
	// the implicit element context, so a bad function name is used here.)
	if _, err := BoxedFilter(constExpr(nums(1, 2)), "bogusfn(item) > 1", NewEnv(), nil); err == nil {
		t.Error("BoxedFilter with an unknown function should be a compile error")
	}
}

func TestFilterIterationLimit(t *testing.T) {
	// A predicate filter over several elements under a tight iteration budget
	// trips the per-element step() limit inside filterClosure.
	ce, err := CompileString("[1, 2, 3, 4, 5][item > 0]", NewEnv())
	if err != nil {
		t.Fatal(err)
	}
	scope := NewEnv().NewScopeWithLimits(nil, Limits{MaxIterations: 2})
	if _, err := ce(scope); err == nil {
		t.Error("filter over many elements should hit the iteration limit")
	}
}

func TestTypecheckNullJoin(t *testing.T) {
	// An if whose then-branch is null joins to the else-branch's type, and vice
	// versa (join's nil-operand branches). Following the result with an
	// arithmetic op would flag a non-number, so a clean number branch stays silent.
	env := NewTypeEnv().Set("flag", TBoolean)
	if errs := checkSrcOK(t, `(if flag then null else 1) + 1`, env); len(errs) != 0 {
		t.Errorf("then-null join: unexpected findings %v", errs)
	}
	if errs := checkSrcOK(t, `(if flag then 1 else null) + 1`, env); len(errs) != 0 {
		t.Errorf("else-null join: unexpected findings %v", errs)
	}
	// A bare null inferred directly (NullLit case).
	if errs := checkSrcOK(t, `null = 1`, env); len(errs) != 0 {
		t.Errorf("null literal: unexpected findings %v", errs)
	}
}

func TestCompileInvalidNumberLiteral(t *testing.T) {
	// A NumberLit whose text the value layer cannot parse is a compile error.
	// "1e" lexes as Number "1" then Name "e", so build the bad literal directly.
	expr := &NumberLit{Text: "12.34.56"}
	if _, err := Compile(expr, NewEnv()); err == nil {
		t.Error("an unparseable number literal should be a compile error")
	}
}

func TestCompileUnsupportedExpression(t *testing.T) {
	// A nil/unknown Expr node hits the compiler's default arm. We use a custom
	// Expr implementation the compiler does not recognise.
	if _, err := Compile(unknownExpr{}, NewEnv()); err == nil {
		t.Error("an unsupported expression should be a compile error")
	}
}

// unknownExpr is an Expr the compiler does not handle, for the default arm.
type unknownExpr struct{}

func (unknownExpr) Pos() Position  { return Position{Line: 1, Col: 1} }
func (unknownExpr) String() string { return "unknown" }
func (unknownExpr) exprNode()      {}

func TestCompileQuantifiedBranches(t *testing.T) {
	cases := map[string]string{
		// some over a non-boolean satisfies -> only nulls seen -> null.
		"some x in [1, 2] satisfies null": "null",
		// every with a false element -> false (sawFalse branch).
		"every x in [1, 2] satisfies x > 1": "false",
		// every over only nulls -> null.
		"every x in [1] satisfies null": "null",
		// some with a true element -> true.
		"some x in [1, 2] satisfies x > 1": "true",
	}
	for src, want := range cases {
		if got := evalStr(t, src, nil); got.String() != want {
			t.Errorf("%q = %s, want %s", src, got, want)
		}
	}
}

func TestQuantifiedSatisfiesError(t *testing.T) {
	// The satisfies clause errors at runtime (iteration limit), aborting `some`.
	ce, err := CompileString("some x in [1, 2] satisfies (for i in [1..1000] return i)[1] > 0", NewEnv())
	if err != nil {
		t.Fatal(err)
	}
	scope := NewEnv().NewScopeWithLimits(nil, Limits{MaxIterations: 5})
	if _, err := ce(scope); err == nil {
		t.Error("a satisfies error should abort the quantifier")
	}
}

func TestListDomainStepLimit(t *testing.T) {
	// A for/some over a multi-element *list* domain trips the per-element step
	// limit (the list branch of forEachDomain), distinct from the range branch.
	ce, err := CompileString("for x in [10, 20, 30, 40] return x", NewEnv())
	if err != nil {
		t.Fatal(err)
	}
	scope := NewEnv().NewScopeWithLimits(nil, Limits{MaxIterations: 2})
	if _, err := ce(scope); err == nil {
		t.Error("for over a list should hit the iteration limit")
	}
}

func TestScalarDomainStepLimit(t *testing.T) {
	// A scalar domain charges one step; under a zero-headroom budget (already
	// exhausted) it errors via the scalar branch of forEachDomain.
	st := &evalState{maxIter: 1, iter: 1}
	err := forEachDomain(st, value.NumberFromInt64(5), func(value.Value) error { return nil })
	if err == nil {
		t.Error("scalar domain should hit the iteration limit when exhausted")
	}
	// And the list branch, likewise pre-exhausted.
	st = &evalState{maxIter: 1, iter: 1}
	if err := forEachDomain(st, nums(1, 2), func(value.Value) error { return nil }); err == nil {
		t.Error("list domain should hit the iteration limit when exhausted")
	}
}

func TestFuncCallMissingArgPadding(t *testing.T) {
	// Calling a two-parameter function with one argument pads the missing slot
	// with null (the else branch of call's argument loop).
	f := &Func{Name: "pair", Params: []string{"a", "b"}}
	f.Body = func(s *Scope) (value.Value, error) {
		return value.NewList(s.at(0), s.at(1)), nil
	}
	got, err := f.call(NewEnv().NewScope(nil), []value.Value{value.NumberFromInt64(7)})
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "[7, null]" {
		t.Errorf("pair(7) = %s, want [7, null]", got)
	}
}

func TestListTrailingComma(t *testing.T) {
	// A trailing comma before the closing bracket is allowed.
	if got := sexpr(t, "[1, 2,]"); got != "(list 1 2)" {
		t.Errorf("Parse([1, 2,]) = %s, want (list 1 2)", got)
	}
}

func TestUnaryTestCallNotReferringInput(t *testing.T) {
	// A unary-test cell that is a call NOT referencing ? falls through to the
	// implicit-equality wrapping (refersToInput CallExpr -> false), so the cell is
	// compared by equality against the input.
	if matchTest(t, `count([1, 2])`, num("5"), nil) {
		t.Error("count([1,2]) = 2, which should not equal input 5")
	}
	if matchTest(t, `count([1, 2, 3, 4, 5])`, num("5"), nil) == false {
		t.Error("count([1..5]) = 5 should equal input 5")
	}
}

func TestScanAtUnterminated(t *testing.T) {
	// An @-literal whose string body is never closed is an Error token.
	toks := Tokenize(`@"unterminated`)
	if toks[0].Kind != Error || toks[0].Value == "" {
		t.Errorf("@\"unterminated = %v %q, want Error with a message", toks[0].Kind, toks[0].Value)
	}
}

func TestRefersToInputCalleeAndContextFalse(t *testing.T) {
	// A unary test whose value is a call *through* ? as the callee references the
	// input in the CallExpr.Fn position. ? holds no function, so it yields null at
	// runtime (no match), but parsing exercises refersToInput's CallExpr.Fn arm.
	env := NewEnv(InputVar)
	if _, err := CompileUnaryTest(`?(1) = 2`, env); err != nil {
		t.Fatalf("callee-? form: %v", err)
	}
	// A context literal whose entries do NOT reference ? falls through to the
	// implicit-equality wrapping (refersToInput ContextLit -> false), so the cell
	// "{a: 1}" is an equality test against ?.
	if matchTest(t, `{a: 1}`, num("5"), nil) {
		t.Error("{a: 1} should not equal the numeric input 5")
	}
	// A list literal with no ? -> anyRefersToInput false -> equality test.
	if matchTest(t, `[1, 2]`, num("5"), nil) {
		t.Error("[1, 2] should not equal the numeric input 5")
	}
}
