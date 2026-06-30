package dmn

import "github.com/pblumer/temis/internal/model"

// Graph is the decision requirements graph (DRG) of a model: its nodes and the
// requirement edges between them, for tooling that draws the diagram — notably
// the own modeler frontend (ADR-0016), which renders this directly rather than
// parsing DMN XML in the browser. It carries JSON tags as part of that wire
// contract.
type Graph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// GraphNode is one DRG element. Type is one of "decision", "inputData" or
// "businessKnowledgeModel". X/Y/Width/Height carry the authored DMNDI bounds
// when the model has a diagram (omitted otherwise, so the client falls back to
// auto-layout).
type GraphNode struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Name string `json:"name"`
	// DataType is the node's resolved FEEL type (the InputData's type, or a
	// decision's output type), for showing the data contract. "" when unknown.
	DataType string `json:"dataType,omitempty"`
	// VarName is a decision's output-variable name (how its result is referenced
	// downstream); defaults to the decision name. Empty for non-decisions.
	VarName string `json:"varName,omitempty"`
	// HasTable marks a decision whose logic is a decision table, so the modeler can
	// offer to open it (double-click). False for non-decisions and for decisions
	// with other logic (literal expression, context, …).
	HasTable bool `json:"hasTable,omitempty"`
	// HasLiteral marks a decision whose logic is a literal FEEL expression, so the
	// modeler opens the expression editor on double-click.
	HasLiteral bool `json:"hasLiteral,omitempty"`
	// HasContext marks a decision whose logic is a boxed context, so the modeler
	// opens the context editor on double-click.
	HasContext bool `json:"hasContext,omitempty"`
	// HasLogic marks a decision that has ANY executable logic (a table, a literal
	// or another boxed expression), so the modeler can show a table icon vs a
	// boxed-expression icon vs an "undecided" (no logic) icon.
	HasLogic bool    `json:"hasLogic,omitempty"`
	X        float64 `json:"x,omitempty"`
	Y        float64 `json:"y,omitempty"`
	Width    float64 `json:"width,omitempty"`
	Height   float64 `json:"height,omitempty"`
}

// GraphEdge is one requirement, directed from the required (upstream) element to
// the element that requires it — matching the DMN arrow direction. Type is
// "informationRequirement" (data/decision dependency) or "knowledgeRequirement"
// (BKM dependency).
type GraphEdge struct {
	Type   string `json:"type"`
	Source string `json:"source"`
	Target string `json:"target"`
}

// Graph returns the model's decision requirements graph. Node ids are the local
// DMN element identifiers; edges reference them. Edges whose endpoint is not a
// known node (dangling references) are skipped.
func (d *Definitions) Graph() Graph {
	g := Graph{}
	known := map[string]bool{}
	var shapes map[string]model.Bounds
	if d.model.Diagram != nil {
		shapes = d.model.Diagram.Shapes
	}
	add := func(id, typ, name, dataType, varName string, hasTable, hasLiteral, hasContext, hasLogic bool) {
		if id == "" {
			return
		}
		known[id] = true
		n := GraphNode{ID: id, Type: typ, Name: name, DataType: dataType, VarName: varName, HasTable: hasTable, HasLiteral: hasLiteral, HasContext: hasContext, HasLogic: hasLogic}
		if b, ok := shapes[id]; ok {
			n.X, n.Y, n.Width, n.Height = b.X, b.Y, b.Width, b.Height
		}
		g.Nodes = append(g.Nodes, n)
	}

	// Resolve input types from what the decisions actually expect (typeRef or
	// the decision-table input clause) — the same resolution as the typed input
	// schema (WP-52), so a type declared only on a table column still shows up.
	inputType := map[string]string{}
	for _, cd := range d.order {
		for _, f := range cd.inputs {
			if f.Type != "" {
				inputType[f.Name] = f.Type
			}
		}
	}

	for _, in := range d.model.InputData {
		t := inputType[in.Name]
		if t == "" {
			t = canonicalType(in.TypeRef)
		}
		add(in.ID, "inputData", in.Name, t, "", false, false, false, false)
	}
	for _, b := range d.model.BKMs {
		add(b.ID, "businessKnowledgeModel", b.Name, canonicalType(b.VariableTypeRef), "", false, false, false, false)
	}
	for _, dec := range d.model.Decisions {
		varName := dec.VariableName
		if varName == "" {
			varName = dec.Name // DMN convention: result referenced by the decision name
		}
		add(dec.ID, "decision", dec.Name, decisionOutputType(dec), varName, dec.DecisionTable != nil, dec.LiteralExpression != nil, dec.Context != nil, dec.Logic() != nil)
	}

	edge := func(typ, source, target string) {
		if source == "" || target == "" || !known[source] || !known[target] {
			return
		}
		g.Edges = append(g.Edges, GraphEdge{Type: typ, Source: source, Target: target})
	}
	for _, dec := range d.model.Decisions {
		for _, src := range dec.RequiredInputs {
			edge("informationRequirement", src, dec.ID)
		}
		for _, src := range dec.RequiredDecisions {
			edge("informationRequirement", src, dec.ID)
		}
		for _, src := range dec.RequiredKnowledge {
			edge("knowledgeRequirement", src, dec.ID)
		}
	}
	for _, b := range d.model.BKMs {
		for _, src := range b.RequiredKnowledge {
			edge("knowledgeRequirement", src, b.ID)
		}
	}
	return g
}

// decisionOutputType resolves a decision's result type for display: the declared
// variable type, else a single decision-table output's type, else a literal
// expression's type. Multi-output tables (a context result) yield "".
func decisionOutputType(dec *model.Decision) string {
	if dec.VariableTypeRef != "" {
		return canonicalType(dec.VariableTypeRef)
	}
	if dt := dec.DecisionTable; dt != nil && len(dt.Outputs) == 1 {
		return canonicalType(dt.Outputs[0].TypeRef)
	}
	if dec.LiteralExpression != nil {
		return canonicalType(dec.LiteralExpression.TypeRef)
	}
	return ""
}
