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
	ID     string  `json:"id"`
	Type   string  `json:"type"`
	Name   string  `json:"name"`
	X      float64 `json:"x,omitempty"`
	Y      float64 `json:"y,omitempty"`
	Width  float64 `json:"width,omitempty"`
	Height float64 `json:"height,omitempty"`
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
	add := func(id, typ, name string) {
		if id == "" {
			return
		}
		known[id] = true
		n := GraphNode{ID: id, Type: typ, Name: name}
		if b, ok := shapes[id]; ok {
			n.X, n.Y, n.Width, n.Height = b.X, b.Y, b.Width, b.Height
		}
		g.Nodes = append(g.Nodes, n)
	}
	for _, in := range d.model.InputData {
		add(in.ID, "inputData", in.Name)
	}
	for _, b := range d.model.BKMs {
		add(b.ID, "businessKnowledgeModel", b.Name)
	}
	for _, dec := range d.model.Decisions {
		add(dec.ID, "decision", dec.Name)
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
