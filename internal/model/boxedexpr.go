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

// ContextExpr is a boxed context: an ordered list of entries. When the final
// entry has no name it is the result cell, whose expression — evaluated with the
// preceding entries' variables in scope — becomes the context's value. Otherwise
// the value is a context keyed by the entry names.
type ContextExpr struct {
	ID      string         `json:",omitempty"`
	Entries []ContextEntry `json:",omitempty"`
}

// ContextEntry is one entry of a boxed context. Name is empty for the result
// cell.
type ContextEntry struct {
	Name  string `json:",omitempty"`
	Value Expression
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
