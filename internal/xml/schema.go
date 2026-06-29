package xml

import "encoding/xml"

// Definitions mirrors the DMN <definitions> root element. Field tags use local
// names only (no namespace), so the same structs decode DMN 1.3, 1.4 and 1.5
// documents regardless of their namespace URI (see decode.go). The concrete
// version is derived from XMLName.Space by the model mapper.
//
// Definitions is the round-trip carrier: Decode fills it and Encode writes it
// back. The DMNDI diagram interchange subtree is preserved verbatim via Raw.
type Definitions struct {
	XMLName xml.Name `xml:"definitions"`
	// Xmlns carries the DMN model namespace as a literal attribute. Go's
	// encoding/xml does not emit XMLName.Space when marshalling, so Encode
	// populates this field to round-trip the default namespace; on decode the
	// declaration is consumed as a namespace and this field stays empty.
	Xmlns     string        `xml:"xmlns,attr,omitempty"`
	ID        string        `xml:"id,attr,omitempty"`
	Name      string        `xml:"name,attr,omitempty"`
	Namespace string        `xml:"namespace,attr,omitempty"`
	ExprLang  string        `xml:"expressionLanguage,attr,omitempty"`
	TypeLang  string        `xml:"typeLanguage,attr,omitempty"`
	ItemDefs  []ItemDef     `xml:"itemDefinition"`
	InputData []InputData   `xml:"inputData"`
	BKMs      []BKM         `xml:"businessKnowledgeModel"`
	Decisions []Decision    `xml:"decision"`
	DMNDI     *Raw          `xml:"DMNDI"`
	Unknown   []UnknownElem `xml:",any"`
}

// ItemDef mirrors <itemDefinition>. Nested components are captured one level
// deep; deeper structure is added when the type system lands (WP-31).
type ItemDef struct {
	ID            string    `xml:"id,attr,omitempty"`
	Name          string    `xml:"name,attr,omitempty"`
	IsCollection  bool      `xml:"isCollection,attr,omitempty"`
	TypeRef       string    `xml:"typeRef"`
	AllowedValues *Text     `xml:"allowedValues>text"`
	Components    []ItemDef `xml:"itemComponent"`
}

// InputData mirrors <inputData>.
type InputData struct {
	ID       string    `xml:"id,attr,omitempty"`
	Name     string    `xml:"name,attr,omitempty"`
	Variable *Variable `xml:"variable"`
}

// Variable mirrors an <variable> information item (the typed result of a
// decision or input data).
type Variable struct {
	Name    string `xml:"name,attr,omitempty"`
	TypeRef string `xml:"typeRef,attr,omitempty"`
}

// BKM mirrors <businessKnowledgeModel>. Its encapsulated logic is a function
// definition (formal parameters + body) callable by invocation or by name from
// another expression (WP-23/WP-24).
type BKM struct {
	ID                string              `xml:"id,attr,omitempty"`
	Name              string              `xml:"name,attr,omitempty"`
	Variable          *Variable           `xml:"variable"`
	EncapsulatedLogic *FunctionDefinition `xml:"encapsulatedLogic"`
	KnowledgeRequirts []KnowledgeRequirt  `xml:"knowledgeRequirement"`
}

// Decision mirrors <decision>. Its logic is exactly one boxed expression (a
// literal expression, decision table, context, invocation or function
// definition); all may be nil for an undecided decision.
type Decision struct {
	ID                  string               `xml:"id,attr,omitempty"`
	Name                string               `xml:"name,attr,omitempty"`
	Variable            *Variable            `xml:"variable"`
	InformationRequirts []InformationRequirt `xml:"informationRequirement"`
	KnowledgeRequirts   []KnowledgeRequirt   `xml:"knowledgeRequirement"`
	Expression
}

// Expression carries the mutually-exclusive boxed-expression children that may
// appear as a decision's logic, a context entry's value, an invocation's called
// function, a binding's argument or a function body. At most one field is
// non-nil. Embedding it lets every such position decode the same element set
// without repeating the tags.
type Expression struct {
	LiteralExpression  *LiteralExpression  `xml:"literalExpression"`
	DecisionTable      *DecisionTable      `xml:"decisionTable"`
	Context            *Context            `xml:"context"`
	Invocation         *Invocation         `xml:"invocation"`
	FunctionDefinition *FunctionDefinition `xml:"functionDefinition"`
	List               *List               `xml:"list"`
	Relation           *Relation           `xml:"relation"`
	Conditional        *Conditional        `xml:"conditional"`
	For                *Iterator           `xml:"for"`
	Every              *Iterator           `xml:"every"`
	Some               *Iterator           `xml:"some"`
	Filter             *Filter             `xml:"filter"`
}

// Context mirrors a boxed <context>: an ordered list of context entries. A final
// entry without a variable is the result cell (DMN 1.5 §7.3.4).
type Context struct {
	ID      string         `xml:"id,attr,omitempty"`
	Entries []ContextEntry `xml:"contextEntry"`
}

// ContextEntry mirrors <contextEntry>: an optional named variable bound to the
// entry's expression value.
type ContextEntry struct {
	Variable *Variable `xml:"variable"`
	Expression
}

// Invocation mirrors a boxed <invocation>: a called function (the embedded
// expression, typically a literal expression naming a BKM) plus the parameter
// bindings supplying its arguments.
type Invocation struct {
	ID string `xml:"id,attr,omitempty"`
	Expression
	Bindings []Binding `xml:"binding"`
}

// Binding mirrors an invocation <binding>: a formal parameter bound to an
// argument expression.
type Binding struct {
	Parameter *Parameter `xml:"parameter"`
	Expression
}

// Parameter mirrors a binding's <parameter> information item (its name selects
// the formal parameter of the called function).
type Parameter struct {
	Name    string `xml:"name,attr,omitempty"`
	TypeRef string `xml:"typeRef,attr,omitempty"`
}

// FunctionDefinition mirrors <functionDefinition>: formal parameters and a body
// expression. Kind defaults to FEEL; non-FEEL kinds (Java, PMML) are not
// executable.
type FunctionDefinition struct {
	ID         string            `xml:"id,attr,omitempty"`
	Kind       string            `xml:"kind,attr,omitempty"`
	Parameters []FormalParameter `xml:"formalParameter"`
	Expression
}

// FormalParameter mirrors a function's <formalParameter> information item.
type FormalParameter struct {
	Name    string `xml:"name,attr,omitempty"`
	TypeRef string `xml:"typeRef,attr,omitempty"`
}

// InformationRequirt mirrors <informationRequirement>: a dependency on another
// decision or on input data, referenced by href.
type InformationRequirt struct {
	ID               string `xml:"id,attr,omitempty"`
	RequiredDecision *Ref   `xml:"requiredDecision"`
	RequiredInput    *Ref   `xml:"requiredInput"`
}

// KnowledgeRequirt mirrors <knowledgeRequirement>: a dependency on a BKM.
type KnowledgeRequirt struct {
	ID                string `xml:"id,attr,omitempty"`
	RequiredKnowledge *Ref   `xml:"requiredKnowledge"`
}

// Ref is an href reference to another DMN element (e.g. "#id_decision").
type Ref struct {
	Href string `xml:"href,attr,omitempty"`
}

// LiteralExpression mirrors <literalExpression>. The FEEL text is stored
// verbatim; it is not parsed until the FEEL compiler (WP-03ff).
type LiteralExpression struct {
	ID       string `xml:"id,attr,omitempty"`
	TypeRef  string `xml:"typeRef,attr,omitempty"`
	ExprLang string `xml:"expressionLanguage,attr,omitempty"`
	Text     string `xml:"text"`
}

// DecisionTable mirrors <decisionTable> with its inputs, outputs and rules.
type DecisionTable struct {
	ID          string   `xml:"id,attr,omitempty"`
	HitPolicy   string   `xml:"hitPolicy,attr,omitempty"`
	Aggregation string   `xml:"aggregation,attr,omitempty"`
	Inputs      []Input  `xml:"input"`
	Outputs     []Output `xml:"output"`
	Rules       []Rule   `xml:"rule"`
}

// Input mirrors a decision table <input> column.
type Input struct {
	ID              string          `xml:"id,attr,omitempty"`
	Label           string          `xml:"label,attr,omitempty"`
	InputExpression InputExpression `xml:"inputExpression"`
	AllowedValues   *Text           `xml:"inputValues>text"`
}

// InputExpression mirrors a decision table <inputExpression>: the FEEL text
// whose value each rule tests, with an optional type reference.
type InputExpression struct {
	TypeRef string `xml:"typeRef,attr,omitempty"`
	Text    string `xml:"text"`
}

// Output mirrors a decision table <output> column.
type Output struct {
	ID            string `xml:"id,attr,omitempty"`
	Name          string `xml:"name,attr,omitempty"`
	Label         string `xml:"label,attr,omitempty"`
	TypeRef       string `xml:"typeRef,attr,omitempty"`
	AllowedValues *Text  `xml:"outputValues>text"`
}

// Rule mirrors a decision table <rule> row.
type Rule struct {
	ID            string   `xml:"id,attr,omitempty"`
	InputEntries  []string `xml:"inputEntry>text"`
	OutputEntries []string `xml:"outputEntry>text"`
	Annotations   []string `xml:"annotationEntry>text"`
}

// Text is a thin wrapper around a <text> element's character data.
type Text struct {
	Value string `xml:",chardata"`
}
