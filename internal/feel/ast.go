package feel

import (
	"fmt"
	"strconv"
	"strings"
)

// Position is a 1-based source location (line and column in runes).
type Position struct {
	Line int
	Col  int
}

// Expr is a node in the FEEL abstract syntax tree. Every node carries its source
// position for diagnostics and renders to a compact S-expression via String,
// which is the basis for the parser's table tests.
type Expr interface {
	Pos() Position
	String() string
	exprNode()
}

type baseNode struct {
	P Position
}

func (n baseNode) Pos() Position { return n.P }
func (baseNode) exprNode()       {}

func base(t Token) baseNode { return baseNode{P: Position{Line: t.Line, Col: t.Col}} }

// --- Literals and references ---

// NumberLit is a numeric literal. The source text is kept verbatim; decimal
// parsing happens in WP-05.
type NumberLit struct {
	baseNode
	Text string
}

func (n *NumberLit) String() string { return n.Text }

// StringLit is a string literal with escapes already resolved into Value.
type StringLit struct {
	baseNode
	Value string
}

func (n *StringLit) String() string { return strconv.Quote(n.Value) }

// BoolLit is a true/false literal.
type BoolLit struct {
	baseNode
	Value bool
}

func (n *BoolLit) String() string { return strconv.FormatBool(n.Value) }

// NullLit is the null literal.
type NullLit struct{ baseNode }

func (n *NullLit) String() string { return "null" }

// AtLit is a temporal literal such as @"2024-01-01"; Value holds the content.
type AtLit struct {
	baseNode
	Value string
}

func (n *AtLit) String() string { return "@" + strconv.Quote(n.Value) }

// NameRef is a (possibly multi-word) name reference. Parts holds the original
// fragments; Name is them joined by single spaces.
type NameRef struct {
	baseNode
	Name  string
	Parts []string
}

func (n *NameRef) String() string { return n.Name }

// --- Compound literals ---

// ListLit is a list literal [e1, e2, ...].
type ListLit struct {
	baseNode
	Elements []Expr
}

func (n *ListLit) String() string { return "(list" + joinExprs(n.Elements) + ")" }

// ContextEntry is a single key/value pair of a context literal.
type ContextEntry struct {
	Key    string
	KeyPos Position
	Value  Expr
}

// ContextLit is a context literal { k: v, ... } with ordered entries.
type ContextLit struct {
	baseNode
	Entries []ContextEntry
}

func (n *ContextLit) String() string {
	var b strings.Builder
	b.WriteString("(context")
	for _, e := range n.Entries {
		fmt.Fprintf(&b, " (%s: %s)", e.Key, e.Value)
	}
	b.WriteString(")")
	return b.String()
}

// IntervalLit is a range literal whose endpoints may be open or closed, e.g.
// [1..10], (1..10], ]1..10[.
type IntervalLit struct {
	baseNode
	LowClosed  bool
	Low        Expr
	High       Expr
	HighClosed bool
}

func (n *IntervalLit) String() string {
	lo, hi := "(", ")"
	if n.LowClosed {
		lo = "["
	}
	if n.HighClosed {
		hi = "]"
	}
	return fmt.Sprintf("%s%s..%s%s", lo, n.Low, n.High, hi)
}

// --- Operators ---

// UnaryExpr is a prefix operation; Op is currently always "-".
type UnaryExpr struct {
	baseNode
	Op string
	X  Expr
}

func (n *UnaryExpr) String() string { return fmt.Sprintf("(%s %s)", n.Op, n.X) }

// BinaryExpr is an infix operation (arithmetic, boolean or comparison).
type BinaryExpr struct {
	baseNode
	Op   string
	X, Y Expr
}

func (n *BinaryExpr) String() string { return fmt.Sprintf("(%s %s %s)", n.Op, n.X, n.Y) }

// BetweenExpr is `X between Low and High`.
type BetweenExpr struct {
	baseNode
	X, Low, High Expr
}

func (n *BetweenExpr) String() string {
	return fmt.Sprintf("(between %s %s %s)", n.X, n.Low, n.High)
}

// InExpr is `X in (t1, t2, ...)` or `X in t`.
type InExpr struct {
	baseNode
	X     Expr
	Tests []Expr
}

func (n *InExpr) String() string { return "(in " + n.X.String() + joinExprs(n.Tests) + ")" }

// InstanceOfExpr is `X instance of Type`.
type InstanceOfExpr struct {
	baseNode
	X    Expr
	Type string
}

func (n *InstanceOfExpr) String() string { return fmt.Sprintf("(instance-of %s %s)", n.X, n.Type) }

// --- Control / iteration ---

// IfExpr is `if Cond then Then else Else` (else is mandatory in FEEL).
type IfExpr struct {
	baseNode
	Cond, Then, Else Expr
}

func (n *IfExpr) String() string { return fmt.Sprintf("(if %s %s %s)", n.Cond, n.Then, n.Else) }

// Iterator is one `name in domain` clause of a for/some/every expression.
type Iterator struct {
	Name    string
	NamePos Position
	In      Expr
}

func (i Iterator) String() string { return fmt.Sprintf("(%s in %s)", i.Name, i.In) }

// ForExpr is `for it1, it2, ... return Return` (iterators are cartesian).
type ForExpr struct {
	baseNode
	Iterators []Iterator
	Return    Expr
}

func (n *ForExpr) String() string {
	return fmt.Sprintf("(for (%s) %s)", joinIterators(n.Iterators), n.Return)
}

// QuantifiedExpr is `some|every it1, ... satisfies Satisfies`.
type QuantifiedExpr struct {
	baseNode
	Quant     string // "some" or "every"
	Iterators []Iterator
	Satisfies Expr
}

func (n *QuantifiedExpr) String() string {
	return fmt.Sprintf("(%s (%s) %s)", n.Quant, joinIterators(n.Iterators), n.Satisfies)
}

// --- Postfix ---

// PathExpr is member access `X.Name`.
type PathExpr struct {
	baseNode
	X    Expr
	Name string
}

func (n *PathExpr) String() string { return fmt.Sprintf("(. %s %s)", n.X, n.Name) }

// FilterExpr is `X[Filter]`.
type FilterExpr struct {
	baseNode
	X      Expr
	Filter Expr
}

func (n *FilterExpr) String() string { return fmt.Sprintf("(filter %s %s)", n.X, n.Filter) }

// Arg is a function-call argument. Name is empty for positional arguments.
type Arg struct {
	Name  string
	Value Expr
}

func (a Arg) String() string {
	if a.Name != "" {
		return fmt.Sprintf("(%s: %s)", a.Name, a.Value)
	}
	return a.Value.String()
}

// CallExpr is a function call `Fn(args...)`.
type CallExpr struct {
	baseNode
	Fn   Expr
	Args []Arg
}

func (n *CallExpr) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "(call %s", n.Fn)
	for _, a := range n.Args {
		b.WriteString(" ")
		b.WriteString(a.String())
	}
	b.WriteString(")")
	return b.String()
}

// Param is a formal parameter of a function definition.
type Param struct {
	Name string
	Type string // optional type reference, empty if unspecified
}

func (p Param) String() string {
	if p.Type != "" {
		return p.Name + ":" + p.Type
	}
	return p.Name
}

// FunctionDefExpr is `function(params) body` or `function(params) external body`.
type FunctionDefExpr struct {
	baseNode
	Params   []Param
	External bool
	Body     Expr
}

func (n *FunctionDefExpr) String() string {
	parts := make([]string, len(n.Params))
	for i, p := range n.Params {
		parts[i] = p.String()
	}
	kw := "function"
	if n.External {
		kw = "function-ext"
	}
	return fmt.Sprintf("(%s (%s) %s)", kw, strings.Join(parts, " "), n.Body)
}

func joinExprs(es []Expr) string {
	var b strings.Builder
	for _, e := range es {
		b.WriteString(" ")
		b.WriteString(e.String())
	}
	return b.String()
}

func joinIterators(its []Iterator) string {
	parts := make([]string, len(its))
	for i, it := range its {
		parts[i] = it.String()
	}
	return strings.Join(parts, " ")
}
