package model

// Decision is a single DMN decision. Its logic is carried by exactly one boxed
// expression field (all nil means the decision has no logic yet); Logic returns
// whichever is set. Requirement fields hold the local identifiers of referenced
// elements, resolved into a graph in WP-28.
type Decision struct {
	ID   string
	Name string
	// VariableName is the decision's output-variable name — how other decisions
	// reference its result in FEEL. Empty when the model declares no <variable>;
	// tools then fall back to the decision name (see RefName). This is the FEEL
	// identifier, kept distinct from Name, which is the free-form display label.
	VariableName    string `json:",omitempty"`
	VariableTypeRef string `json:",omitempty"`

	RequiredDecisions []string `json:",omitempty"`
	RequiredInputs    []string `json:",omitempty"`
	RequiredKnowledge []string `json:",omitempty"`

	LiteralExpression *LiteralExpression `json:",omitempty"`
	DecisionTable     *DecisionTable     `json:",omitempty"`
	Context           *ContextExpr       `json:",omitempty"`
	Invocation        *Invocation        `json:",omitempty"`
	FunctionDef       *FunctionDef       `json:",omitempty"`
	List              *ListExpr          `json:",omitempty"`
	Relation          *RelationExpr      `json:",omitempty"`
	Conditional       *Conditional       `json:",omitempty"`
	For               *ForExpr           `json:",omitempty"`
	Quantified        *Quantified        `json:",omitempty"`
	Filter            *FilterExpr        `json:",omitempty"`
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
	case d.List != nil:
		return d.List
	case d.Relation != nil:
		return d.Relation
	case d.Conditional != nil:
		return d.Conditional
	case d.For != nil:
		return d.For
	case d.Quantified != nil:
		return d.Quantified
	case d.Filter != nil:
		return d.Filter
	default:
		return nil
	}
}

// RefName is the FEEL identifier a decision's result is bound to and referenced
// by: its declared output-variable name, or — when the model declares none — its
// display name. Name stays the free-form display label; RefName is what the
// engine binds, the schema keys, and other expressions reference (DMN §7:
// decision/variable/@name). For the common model, where no separate variable is
// declared, the two coincide.
func (d *Decision) RefName() string {
	if d.VariableName != "" {
		return d.VariableName
	}
	return d.Name
}

// InputData is an input data node feeding one or more decisions.
type InputData struct {
	ID   string
	Name string
	// VariableName is the input's FEEL identifier (inputData/variable/@name) when
	// declared, distinct from the free-form display Name; empty falls back to Name
	// (see RefName).
	VariableName string `json:",omitempty"`
	TypeRef      string `json:",omitempty"`
}

// RefName is the FEEL identifier this input is bound under and referenced by: its
// declared variable name, or its display name when none is declared. It mirrors
// Decision.RefName so the engine, schema and expressions key inputs consistently.
func (in *InputData) RefName() string {
	if in.VariableName != "" {
		return in.VariableName
	}
	return in.Name
}

// DecisionService is a reusable evaluation unit: it exposes its OutputDecisions,
// evaluates EncapsulatedDecisions internally and expects InputDecisions and
// InputData from the caller. All fields hold local element identifiers.
type DecisionService struct {
	ID                    string
	Name                  string
	VariableTypeRef       string   `json:",omitempty"`
	OutputDecisions       []string `json:",omitempty"`
	EncapsulatedDecisions []string `json:",omitempty"`
	InputDecisions        []string `json:",omitempty"`
	InputData             []string `json:",omitempty"`
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
