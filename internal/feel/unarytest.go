package feel

import "github.com/pblumer/temis/internal/value"

// InputVar is the name of the implicit input value inside a decision-table unary
// test. In FEEL a cell like "< 18" means "? < 18", where ? is the value being
// tested. The decision-table compiler (WP-09) binds this variable per input
// column.
const InputVar = "?"

// CompileUnaryTest compiles a decision-table input-entry cell into a boolean
// CompiledExpr over env. env must define InputVar ("?"); a cell may also refer
// to other decision variables (e.g. "< limit"). An empty cell or "-" always
// matches.
func CompileUnaryTest(src string, env *Env) (CompiledExpr, error) {
	expr, err := parseUnaryTest(src)
	if err != nil {
		return nil, err
	}
	return Compile(expr, env)
}

// Matches evaluates a compiled unary test against scope and reports whether it
// matched (evaluated to true). A null or non-boolean result is not a match.
func Matches(test CompiledExpr, s *Scope) (bool, error) {
	v, err := test(s)
	if err != nil {
		return false, err
	}
	return v == value.True, nil
}

// parseUnaryTest parses a unary-test cell into a boolean expression that
// references InputVar. It reuses the expression parser for operands.
func parseUnaryTest(src string) (expr Expr, err error) {
	p := &parser{toks: Tokenize(src), maxDepth: DefaultMaxParseDepth}
	defer func() {
		if r := recover(); r != nil {
			pe, ok := r.(*ParseError)
			if !ok {
				panic(r)
			}
			expr, err = nil, pe
		}
	}()
	e := p.unaryTests()
	if p.cur().Kind != EOF {
		p.fail("unexpected %s after unary test", describe(p.cur()))
	}
	return e, nil
}

// unaryTests parses a full cell: "-" or empty (always true), "not(...)"
// (negation) or a comma-separated list of positive unary tests.
func (p *parser) unaryTests() Expr {
	switch {
	case p.cur().Kind == EOF:
		return alwaysTrue(p.cur())
	case p.cur().Kind == Minus && p.peek(1).Kind == EOF:
		return alwaysTrue(p.advance())
	case p.cur().Kind == Not && p.peek(1).Kind == LParen:
		t := p.advance() // not
		p.advance()      // (
		inner := p.positiveUnaryTests()
		p.expect(RParen)
		// not(<tests>) via the boolean built-in
		ref := &NameRef{baseNode: base(t), Name: "not", Parts: []string{"not"}}
		return &CallExpr{baseNode: base(t), Fn: ref, Args: []Arg{{Value: inner}}}
	default:
		return p.positiveUnaryTests()
	}
}

// positiveUnaryTests folds a comma-separated list into a disjunction: the cell
// matches if any single test matches.
func (p *parser) positiveUnaryTests() Expr {
	left := p.positiveUnaryTest()
	for p.cur().Kind == Comma {
		op := p.advance()
		right := p.positiveUnaryTest()
		left = &BinaryExpr{baseNode: base(op), Op: "or", X: left, Y: right}
	}
	return left
}

// positiveUnaryTest parses one test: a leading-operator comparison against the
// input, an interval (containment), an expression that already refers to the
// input, or a value compared by implicit equality.
func (p *parser) positiveUnaryTest() Expr {
	t := p.cur()
	if op, ok := comparisonOps[t.Kind]; ok && (t.Kind == Lt || t.Kind == Lte || t.Kind == Gt || t.Kind == Gte) {
		p.advance()
		operand := p.parseAdd()
		return &BinaryExpr{baseNode: base(t), Op: op, X: inputRef(t), Y: operand}
	}

	e := p.parseExpr()
	switch {
	case isInterval(e):
		return &InExpr{baseNode: baseAt(e.Pos()), X: inputRef(t), Tests: []Expr{e}}
	case refersToInput(e):
		return e
	default:
		return &BinaryExpr{baseNode: baseAt(e.Pos()), Op: "=", X: inputRef(t), Y: e}
	}
}

func alwaysTrue(t Token) Expr { return &BoolLit{baseNode: base(t), Value: true} }

func inputRef(t Token) *NameRef {
	return &NameRef{baseNode: base(t), Name: InputVar, Parts: []string{InputVar}}
}

func baseAt(p Position) baseNode { return baseNode{P: p} }

func isInterval(e Expr) bool {
	_, ok := e.(*IntervalLit)
	return ok
}

// refersToInput reports whether the expression mentions the implicit input
// variable, in which case it is already a complete boolean test.
func refersToInput(e Expr) bool {
	switch n := e.(type) {
	case *NameRef:
		return n.Name == InputVar
	case *UnaryExpr:
		return refersToInput(n.X)
	case *BinaryExpr:
		return refersToInput(n.X) || refersToInput(n.Y)
	case *BetweenExpr:
		return refersToInput(n.X) || refersToInput(n.Low) || refersToInput(n.High)
	case *InExpr:
		if refersToInput(n.X) {
			return true
		}
		return anyRefersToInput(n.Tests)
	case *InstanceOfExpr:
		return refersToInput(n.X)
	case *CmpTest:
		return refersToInput(n.Y)
	case *IfExpr:
		return refersToInput(n.Cond) || refersToInput(n.Then) || refersToInput(n.Else)
	case *PathExpr:
		return refersToInput(n.X)
	case *FilterExpr:
		return refersToInput(n.X) || refersToInput(n.Filter)
	case *CallExpr:
		if refersToInput(n.Fn) {
			return true
		}
		for _, a := range n.Args {
			if refersToInput(a.Value) {
				return true
			}
		}
		return false
	case *ListLit:
		return anyRefersToInput(n.Elements)
	case *ContextLit:
		for _, en := range n.Entries {
			if refersToInput(en.Value) {
				return true
			}
		}
		return false
	case *IntervalLit:
		return refersToInput(n.Low) || refersToInput(n.High)
	default:
		return false
	}
}

func anyRefersToInput(es []Expr) bool {
	for _, e := range es {
		if refersToInput(e) {
			return true
		}
	}
	return false
}
