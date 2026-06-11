package model

// Decision is a single DMN decision. Its logic is carried by exactly one of
// LiteralExpression or DecisionTable (both nil means the decision has no logic
// yet). Requirement fields hold the local identifiers of referenced elements,
// resolved into a graph in WP-28.
type Decision struct {
	ID              string
	Name            string
	VariableTypeRef string `json:",omitempty"`

	RequiredDecisions []string `json:",omitempty"`
	RequiredInputs    []string `json:",omitempty"`
	RequiredKnowledge []string `json:",omitempty"`

	LiteralExpression *LiteralExpression `json:",omitempty"`
	DecisionTable     *DecisionTable     `json:",omitempty"`
}

// InputData is an input data node feeding one or more decisions.
type InputData struct {
	ID      string
	Name    string
	TypeRef string `json:",omitempty"`
}

// BKM is a business knowledge model node. Only its identity is modelled in
// WP-02; its encapsulated function is added in WP-24.
type BKM struct {
	ID   string
	Name string
}
