package feel

import (
	"fmt"
	"strings"
)

// NameSet is an optional oracle of known names. When supplied to the parser it
// enables longest-match assembly of multi-word names, including names that
// contain keywords (e.g. the builtin "date and time"). Without it, the parser
// greedily merges consecutive plain name fragments, which covers all names that
// do not embed a keyword (see ADR / docs/30-feel-spec.md §2).
type NameSet interface {
	// Has reports whether name (fragments joined by single spaces) is known.
	Has(name string) bool
}

// ParseError is a syntax error with its source position.
type ParseError struct {
	Msg  string
	Line int
	Col  int
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%d:%d: %s", e.Line, e.Col, e.Msg)
}

// Parse lexes and parses a FEEL expression into an AST. Multi-word names are
// assembled greedily from plain fragments.
func Parse(src string) (Expr, error) {
	return ParseWithNames(src, nil)
}

// DefaultMaxParseDepth bounds the syntactic nesting the parser will descend
// into, turning pathologically deep input (e.g. millions of prefix `-` or
// nested `(`/`[`) into a *ParseError instead of a fatal stack overflow that
// would crash the whole process (audit finding K1, ADR-0008). It sits far above
// any realistic DMN model, whose FEEL nesting is in the low tens.
const DefaultMaxParseDepth = 10_000

// ParseWithNames parses src using names as the oracle for multi-word name
// assembly (may be nil).
func ParseWithNames(src string, names NameSet) (expr Expr, err error) {
	p := &parser{toks: Tokenize(src), names: names, maxDepth: DefaultMaxParseDepth}
	defer func() {
		if r := recover(); r != nil {
			pe, ok := r.(*ParseError)
			if !ok {
				panic(r)
			}
			expr, err = nil, pe
		}
	}()
	e := p.parseExpr()
	if p.cur().Kind != EOF {
		p.fail("unexpected %s after expression", describe(p.cur()))
	}
	return e, nil
}

type parser struct {
	toks     []Token
	pos      int
	names    NameSet
	depth    int // current syntactic recursion depth (K1 guard)
	maxDepth int // hard ceiling on depth; 0 means unbounded
	// noFilter suppresses the postfix [ ] filter while parsing an interval
	// endpoint, so that the interval's closing bracket in forms like ]1..10[
	// is not mistaken for a filter. It is cleared again inside any balanced
	// sub-expression (parseExpr), where filters are unambiguous.
	noFilter bool
}

func (p *parser) cur() Token { return p.toks[p.pos] }

func (p *parser) peek(n int) Token {
	i := p.pos + n
	if i >= len(p.toks) {
		return p.toks[len(p.toks)-1] // EOF
	}
	return p.toks[i]
}

func (p *parser) advance() Token {
	t := p.toks[p.pos]
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	return t
}

func (p *parser) expect(k Kind) Token {
	if p.cur().Kind != k {
		p.fail("expected %s, got %s", k, describe(p.cur()))
	}
	return p.advance()
}

// enter increments the recursion depth and aborts with a *ParseError once the
// configured ceiling is crossed. Paired with a deferred leave, it is placed on
// the two functions every descent passes through — parseUnary (prefix chains)
// and parsePrimary (parenthesised/bracketed/braced nesting) — so no recursive
// path can grow the native stack without bound (K1).
func (p *parser) enter() {
	p.depth++
	if p.maxDepth > 0 && p.depth > p.maxDepth {
		p.fail("expression nesting too deep (limit %d)", p.maxDepth)
	}
}

func (p *parser) leave() { p.depth-- }

func (p *parser) fail(format string, args ...any) {
	t := p.cur()
	panic(&ParseError{Msg: fmt.Sprintf(format, args...), Line: t.Line, Col: t.Col})
}

// describe renders a token for error messages.
func describe(t Token) string {
	switch t.Kind {
	case EOF:
		return "end of input"
	case Error:
		return fmt.Sprintf("invalid token (%s)", t.Value)
	case Number, Name:
		return fmt.Sprintf("%s %q", t.Kind, t.Text)
	default:
		return fmt.Sprintf("%q", t.Kind.String())
	}
}

// --- Expression grammar (precedence climbing) ---

// parseExpr parses a full expression. Entering it marks a balanced context, so
// the interval-endpoint filter suppression does not leak into sub-expressions.
func (p *parser) parseExpr() Expr {
	saved := p.noFilter
	p.noFilter = false
	e := p.parseOr()
	p.noFilter = saved
	return e
}

// parseEndpoint parses an interval bound with the postfix filter suppressed.
func (p *parser) parseEndpoint() Expr {
	saved := p.noFilter
	p.noFilter = true
	e := p.parseAdd()
	p.noFilter = saved
	return e
}

func (p *parser) parseOr() Expr {
	left := p.parseAnd()
	for p.cur().Kind == Or {
		op := p.advance()
		right := p.parseAnd()
		left = &BinaryExpr{baseNode: base(op), Op: "or", X: left, Y: right}
	}
	return left
}

func (p *parser) parseAnd() Expr {
	left := p.parseComparison()
	for p.cur().Kind == And {
		op := p.advance()
		right := p.parseComparison()
		left = &BinaryExpr{baseNode: base(op), Op: "and", X: left, Y: right}
	}
	return left
}

var comparisonOps = map[Kind]string{
	Eq: "=", Neq: "!=", Lt: "<", Lte: "<=", Gt: ">", Gte: ">=",
}

func (p *parser) parseComparison() Expr {
	left := p.parseAdd()
	for {
		t := p.cur()
		if op, ok := comparisonOps[t.Kind]; ok {
			p.advance()
			left = &BinaryExpr{baseNode: base(t), Op: op, X: left, Y: p.parseAdd()}
			continue
		}
		switch t.Kind {
		case Between:
			p.advance()
			low := p.parseAdd()
			p.expect(And)
			high := p.parseAdd()
			return &BetweenExpr{baseNode: base(t), X: left, Low: low, High: high}
		case In:
			p.advance()
			return &InExpr{baseNode: base(t), X: left, Tests: p.parseInTests()}
		case Instance:
			p.advance()
			p.expect(Of)
			return &InstanceOfExpr{baseNode: base(t), X: left, Type: p.parseTypeName()}
		default:
			return left
		}
	}
}

// parseInTests parses the right-hand side of `in`: a parenthesised, comma
// separated list of tests, or a single test expression. Operator-prefixed unary
// tests (e.g. `in < 5`) are handled by the unary-test parser in WP-08.
func (p *parser) parseInTests() []Expr {
	if p.cur().Kind == LParen {
		p.advance()
		var tests []Expr
		for p.cur().Kind != RParen {
			tests = append(tests, p.parseExpr())
			if p.cur().Kind != Comma {
				break
			}
			p.advance()
		}
		p.expect(RParen)
		return tests
	}
	return []Expr{p.parseAdd()}
}

func (p *parser) parseAdd() Expr {
	left := p.parseMul()
	for {
		t := p.cur()
		if t.Kind != Plus && t.Kind != Minus {
			return left
		}
		p.advance()
		op := "+"
		if t.Kind == Minus {
			op = "-"
		}
		left = &BinaryExpr{baseNode: base(t), Op: op, X: left, Y: p.parseMul()}
	}
}

func (p *parser) parseMul() Expr {
	left := p.parseUnary()
	for {
		t := p.cur()
		if t.Kind != Star && t.Kind != Slash {
			return left
		}
		p.advance()
		op := "*"
		if t.Kind == Slash {
			op = "/"
		}
		left = &BinaryExpr{baseNode: base(t), Op: op, X: left, Y: p.parseUnary()}
	}
}

// parseUnary handles prefix minus. Exponentiation binds tighter than unary minus
// (docs/30-feel-spec.md §3), so a unary operand is a power expression.
func (p *parser) parseUnary() Expr {
	p.enter()
	defer p.leave()
	if t := p.cur(); t.Kind == Minus {
		p.advance()
		return &UnaryExpr{baseNode: base(t), Op: "-", X: p.parseUnary()}
	}
	return p.parsePow()
}

// parsePow is right-associative: 2 ** 3 ** 2 == 2 ** (3 ** 2).
func (p *parser) parsePow() Expr {
	left := p.parsePostfix()
	if t := p.cur(); t.Kind == Pow {
		p.advance()
		return &BinaryExpr{baseNode: base(t), Op: "**", X: left, Y: p.parseUnary()}
	}
	return left
}

func (p *parser) parsePostfix() Expr {
	x := p.parsePrimary()
	for {
		switch p.cur().Kind {
		case Dot:
			t := p.advance()
			name := p.expect(Name)
			x = &PathExpr{baseNode: base(t), X: x, Name: name.Text}
		case LBracket:
			if p.noFilter {
				return x
			}
			t := p.advance()
			filter := p.parseExpr()
			p.expect(RBracket)
			x = &FilterExpr{baseNode: base(t), X: x, Filter: filter}
		case LParen:
			x = p.parseCall(x)
		default:
			return x
		}
	}
}

func (p *parser) parseCall(fn Expr) Expr {
	t := p.expect(LParen)
	call := &CallExpr{baseNode: base(t), Fn: fn}
	for p.cur().Kind != RParen {
		var arg Arg
		if p.cur().Kind == Name && p.peek(p.nameRunLen(p.pos)).Kind == Colon {
			arg.Name = p.assembleNameString()
			p.expect(Colon)
		}
		arg.Value = p.parseExpr()
		call.Args = append(call.Args, arg)
		if p.cur().Kind != Comma {
			break
		}
		p.advance()
	}
	p.expect(RParen)
	return call
}

func (p *parser) parsePrimary() Expr {
	p.enter()
	defer p.leave()
	t := p.cur()
	switch t.Kind {
	case Number:
		p.advance()
		return &NumberLit{baseNode: base(t), Text: t.Text}
	case String:
		p.advance()
		return &StringLit{baseNode: base(t), Value: t.Value}
	case At:
		p.advance()
		return &AtLit{baseNode: base(t), Value: t.Value}
	case True:
		p.advance()
		return &BoolLit{baseNode: base(t), Value: true}
	case False:
		p.advance()
		return &BoolLit{baseNode: base(t), Value: false}
	case Null:
		p.advance()
		return &NullLit{baseNode: base(t)}
	case Name:
		return p.parseName()
	case Not:
		// `not` is the builtin function not(...); treat it as a name so the
		// postfix call rule turns `not(x)` into a CallExpr.
		p.advance()
		return &NameRef{baseNode: base(t), Name: "not", Parts: []string{"not"}}
	case LParen:
		return p.parseParenOrInterval()
	case LBracket:
		return p.parseListOrInterval()
	case RBracket:
		return p.parseOpenLowInterval()
	case LBrace:
		return p.parseContext()
	case If:
		return p.parseIf()
	case For:
		return p.parseFor()
	case Some, Every:
		return p.parseQuantified()
	case Function:
		return p.parseFunctionDef()
	default:
		p.fail("unexpected %s", describe(t))
		return nil // unreachable
	}
}

// --- Names ---

// nameRunLen returns the number of consecutive plain Name tokens starting at i.
func (p *parser) nameRunLen(i int) int {
	n := 0
	for i+n < len(p.toks) && p.toks[i+n].Kind == Name {
		n++
	}
	return n
}

// isNameableKeyword reports whether a keyword may appear inside a multi-word
// name (true/false/null are excluded so literals are never swallowed).
func isNameableKeyword(k Kind) bool { return k >= And && k <= External }

func (p *parser) parseName() Expr {
	start := p.cur()
	parts := p.takeNameRun()
	return &NameRef{baseNode: base(start), Name: strings.Join(parts, " "), Parts: parts}
}

// takeNameRun consumes one (possibly multi-word) name starting at the cursor and
// returns its fragments, advancing past them. It greedily merges plain Name
// fragments; when a name oracle is set it also extends across nameable keywords
// (e.g. the "and" in "days and time duration") while the joined run is a name
// the oracle knows.
func (p *parser) takeNameRun() []string {
	start := p.cur()
	run := []Token{start}
	for j := p.pos + 1; j < len(p.toks); j++ {
		k := p.toks[j].Kind
		if k == Name || (p.names != nil && isNameableKeyword(k)) {
			run = append(run, p.toks[j])
			continue
		}
		break
	}

	take := p.nameRunLen(p.pos) // plain-fragment greedy default
	if p.names != nil {
		joined := start.Text
		for i := 1; i < len(run); i++ {
			joined += " " + run[i].Text
			if p.names.Has(joined) {
				take = i + 1
			}
		}
	}
	if take < 1 {
		take = 1
	}

	parts := make([]string, take)
	for i := 0; i < take; i++ {
		parts[i] = run[i].Text
	}
	p.pos += take
	return parts
}

// assembleNameString consumes a greedy run of plain Name fragments and returns
// them joined; used for context keys and named-argument labels.
func (p *parser) assembleNameString() string {
	n := p.nameRunLen(p.pos)
	if n < 1 {
		n = 1
	}
	parts := make([]string, n)
	for i := 0; i < n; i++ {
		parts[i] = p.advance().Text
	}
	return strings.Join(parts, " ")
}

// parseTypeName parses a type reference: a name run, optionally followed by a
// balanced <...> generic captured verbatim. The full type grammar lands in WP-31.
func (p *parser) parseTypeName() string {
	if p.cur().Kind != Name {
		p.fail("expected a type name, got %s", describe(p.cur()))
	}
	name := strings.Join(p.takeNameRun(), " ")
	if p.cur().Kind == Lt {
		name += p.captureGeneric()
	}
	return name
}

func (p *parser) captureGeneric() string {
	var b strings.Builder
	depth := 0
	for {
		t := p.cur()
		switch t.Kind {
		case Lt:
			depth++
		case Gt:
			depth--
		case EOF:
			p.fail("unterminated type parameter list")
		}
		b.WriteString(t.Text)
		p.advance()
		if depth == 0 {
			return b.String()
		}
	}
}

// --- Bracketed forms ---

func (p *parser) parseParenOrInterval() Expr {
	t := p.expect(LParen)
	e := p.parseExpr()
	if p.cur().Kind == DotDot {
		p.advance()
		high := p.parseEndpoint()
		return &IntervalLit{baseNode: base(t), LowClosed: false, Low: e, High: high, HighClosed: p.parseHighBracket()}
	}
	p.expect(RParen)
	return e
}

func (p *parser) parseListOrInterval() Expr {
	t := p.expect(LBracket)
	if p.cur().Kind == RBracket { // empty list
		p.advance()
		return &ListLit{baseNode: base(t)}
	}
	first := p.parseExpr()
	if p.cur().Kind == DotDot {
		p.advance()
		high := p.parseEndpoint()
		return &IntervalLit{baseNode: base(t), LowClosed: true, Low: first, High: high, HighClosed: p.parseHighBracket()}
	}
	list := &ListLit{baseNode: base(t), Elements: []Expr{first}}
	for p.cur().Kind == Comma {
		p.advance()
		if p.cur().Kind == RBracket {
			break
		}
		list.Elements = append(list.Elements, p.parseExpr())
	}
	p.expect(RBracket)
	return list
}

// parseOpenLowInterval handles intervals whose lower bound is open via a leading
// `]`, e.g. ]1..10[.
func (p *parser) parseOpenLowInterval() Expr {
	t := p.expect(RBracket)
	low := p.parseExpr()
	p.expect(DotDot)
	high := p.parseEndpoint()
	return &IntervalLit{baseNode: base(t), LowClosed: false, Low: low, High: high, HighClosed: p.parseHighBracket()}
}

// parseHighBracket consumes the closing bracket of an interval and reports
// whether the upper bound is closed. `]` closes; `)` and `[` are open.
func (p *parser) parseHighBracket() bool {
	switch p.cur().Kind {
	case RBracket:
		p.advance()
		return true
	case RParen:
		p.advance()
		return false
	case LBracket:
		p.advance()
		return false
	default:
		p.fail("expected interval close bracket, got %s", describe(p.cur()))
		return false
	}
}

func (p *parser) parseContext() Expr {
	t := p.expect(LBrace)
	ctx := &ContextLit{baseNode: base(t)}
	for p.cur().Kind != RBrace {
		var entry ContextEntry
		key := p.cur()
		switch key.Kind {
		case String:
			entry.Key = key.Value
			entry.KeyPos = Position{Line: key.Line, Col: key.Col}
			p.advance()
		case Name:
			entry.KeyPos = Position{Line: key.Line, Col: key.Col}
			entry.Key = p.assembleNameString()
		default:
			p.fail("expected context key, got %s", describe(key))
		}
		p.expect(Colon)
		entry.Value = p.parseExpr()
		ctx.Entries = append(ctx.Entries, entry)
		if p.cur().Kind != Comma {
			break
		}
		p.advance()
	}
	p.expect(RBrace)
	return ctx
}

// --- Control / iteration ---

func (p *parser) parseIf() Expr {
	t := p.expect(If)
	cond := p.parseExpr()
	p.expect(Then)
	then := p.parseExpr()
	p.expect(Else) // else is mandatory in FEEL
	els := p.parseExpr()
	return &IfExpr{baseNode: base(t), Cond: cond, Then: then, Else: els}
}

func (p *parser) parseFor() Expr {
	t := p.expect(For)
	iters := p.parseIterators(In)
	p.expect(Return)
	return &ForExpr{baseNode: base(t), Iterators: iters, Return: p.parseExpr()}
}

func (p *parser) parseQuantified() Expr {
	t := p.advance() // some / every
	quant := "some"
	if t.Kind == Every {
		quant = "every"
	}
	iters := p.parseIterators(Satisfies)
	p.expect(Satisfies)
	return &QuantifiedExpr{baseNode: base(t), Quant: quant, Iterators: iters, Satisfies: p.parseExpr()}
}

// parseIterators parses one or more `name in domain` clauses separated by commas,
// stopping before the terminator keyword (return or satisfies).
func (p *parser) parseIterators(_ Kind) []Iterator {
	var iters []Iterator
	for {
		name := p.expect(Name)
		p.expect(In)
		domain := p.parseExpr()
		// In an iteration context FEEL allows a bare range `low..high`, which is
		// the closed range [low..high]; brackets are not required here.
		if p.cur().Kind == DotDot {
			dot := p.advance()
			domain = &IntervalLit{
				baseNode:   base(dot),
				LowClosed:  true,
				Low:        domain,
				High:       p.parseExpr(),
				HighClosed: true,
			}
		}
		iters = append(iters, Iterator{
			Name:    name.Text,
			NamePos: Position{Line: name.Line, Col: name.Col},
			In:      domain,
		})
		if p.cur().Kind != Comma {
			return iters
		}
		p.advance()
	}
}

func (p *parser) parseFunctionDef() Expr {
	t := p.expect(Function)
	p.expect(LParen)
	fn := &FunctionDefExpr{baseNode: base(t)}
	for p.cur().Kind != RParen {
		name := p.expect(Name)
		param := Param{Name: name.Text}
		if p.cur().Kind == Colon {
			p.advance()
			param.Type = p.parseTypeName()
		}
		fn.Params = append(fn.Params, param)
		if p.cur().Kind != Comma {
			break
		}
		p.advance()
	}
	p.expect(RParen)
	if p.cur().Kind == External {
		fn.External = true
		p.advance()
	}
	fn.Body = p.parseExpr()
	return fn
}
