package model

// Decision is a single DMN decision. Its logic is carried by exactly one boxed
// expression field (all nil means the decision has no logic yet); Logic returns
// whichever is set. Requirement fields hold the local identifiers of referenced
// elements, resolved into a graph in WP-28.
type Decision struct {
	ID              string
	Name            string
	VariableTypeRef string `json:",omitempty"`

	RequiredDecisions []string `json:",omitempty"`
	RequiredInputs    []string `json:",omitempty"`
	RequiredKnowledge []string `json:",omitempty"`

	LiteralExpression *LiteralExpression `json:",omitempty"`
	DecisionTable     *DecisionTable     `json:",omitempty"`
	Context           *ContextExpr       `json:",omitempty"`
	Invocation        *Invocation        `json:",omitempty"`
	FunctionDef       *FunctionDef       `json:",omitempty"`
}

// Logic returns the decision's executable logic, or nil if it has none.
func (d *Decision) Logic() Expression {
	switch {
	case d.LiteralExpression != nil:
		return d.LiteralExpression
	case d.DecisionTable != nil:
		return d.DecisionTable
	case d.Context != nil:
		return d.Context
	case d.Invocation != nil:
		return d.Invocation
	case d.FunctionDef != nil:
		return d.FunctionDef
	default:
		return nil
	}
}

// InputData is an input data node feeding one or more decisions.
type InputData struct {
	ID      string
	Name    string
	TypeRef string `json:",omitempty"`
}

// BKM is a business knowledge model node: a reusable function callable by
// invocation or by name. EncapsulatedLogic carries its parameters and body
// (nil when the model declares none).
type BKM struct {
	ID                string
	Name              string
	VariableTypeRef   string       `json:",omitempty"`
	EncapsulatedLogic *FunctionDef `json:",omitempty"`
	RequiredKnowledge []string     `json:",omitempty"`
}
