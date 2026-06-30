package model

// Expression is a decision's (or a nested) executable logic: exactly one of the
// boxed-expression forms. The compiler (internal/boxed) dispatches on the
// concrete type. LiteralExpression and DecisionTable also implement it.
type Expression interface{ isExpression() }

func (*LiteralExpression) isExpression() {}
func (*DecisionTable) isExpression()     {}
func (*ContextExpr) isExpression()       {}
func (*Invocation) isExpression()        {}
func (*FunctionDef) isExpression()       {}
func (*ListExpr) isExpression()          {}
func (*RelationExpr) isExpression()      {}
func (*Conditional) isExpression()       {}
func (*ForExpr) isExpression()           {}
func (*Quantified) isExpression()        {}
func (*FilterExpr) isExpression()        {}

// ContextExpr is a boxed context: an ordered list of entries. When the final
// entry has no name it is the result cell, whose expression — evaluated with the
// preceding entries' variables in scope — becomes the context's value. Otherwise
// the value is a context keyed by the entry names.
type ContextExpr struct {
	ID      string         `json:",omitempty"`
	Entries []ContextEntry `json:",omitempty"`
}

// ContextEntry is one entry of a boxed context. Name is empty for the result
// cell. TypeRef is the bound variable's declared FEEL type, if any.
type ContextEntry struct {
	Name    string `json:",omitempty"`
	TypeRef string `json:",omitempty"`
	Value   Expression
}

// Invocation calls a function — typically a business knowledge model named by
// Called — binding each argument to a formal parameter by name.
type Invocation struct {
	ID       string `json:",omitempty"`
	Called   Expression
	Bindings []Binding `json:",omitempty"`
}

// Binding binds an invocation argument to a formal parameter by name.
type Binding struct {
	Parameter string
	Value     Expression
}

// FunctionDef is a boxed function definition: formal parameters and a body. Kind
// is the body language; only FEEL (the default) is executable.
type FunctionDef struct {
	ID         string `json:",omitempty"`
	Kind       string `json:",omitempty"`
	Parameters []FunctionParam
	Body       Expression
}

// FunctionParam is a function's formal parameter.
type FunctionParam struct {
	Name    string
	TypeRef string `json:",omitempty"`
}

// ListExpr is a boxed list: its items evaluate to the elements of a FEEL list,
// in order.
type ListExpr struct {
	ID    string       `json:",omitempty"`
	Items []Expression `json:",omitempty"`
}

// RelationExpr is a boxed relation: a table of named columns whose rows evaluate
// to a list of contexts (one per row, keyed by column name).
type RelationExpr struct {
	ID      string        `json:",omitempty"`
	Columns []string      `json:",omitempty"`
	Rows    []RelationRow `json:",omitempty"`
}

// RelationRow is one relation row: cell expressions aligned with the columns.
type RelationRow struct {
	Cells []Expression `json:",omitempty"`
}

// Conditional is a boxed if/then/else (DMN 1.4+).
type Conditional struct {
	ID   string `json:",omitempty"`
	If   Expression
	Then Expression
	Else Expression
}

// ForExpr is a boxed iterator (DMN 1.4+): IteratorVariable ranges over In,
// collecting Return into a list.
type ForExpr struct {
	ID               string `json:",omitempty"`
	IteratorVariable string
	In               Expression
	Return           Expression
}

// Quantified is a boxed some/every (DMN 1.4+): IteratorVariable ranges over In,
// testing Satisfies. Kind is "some" or "every".
type Quantified struct {
	ID               string `json:",omitempty"`
	Kind             string
	IteratorVariable string
	In               Expression
	Satisfies        Expression
}

// FilterExpr is a boxed filter (DMN 1.4+): the In collection filtered by Match.
type FilterExpr struct {
	ID    string `json:",omitempty"`
	In    Expression
	Match Expression
}
