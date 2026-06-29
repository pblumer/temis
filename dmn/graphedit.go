package dmn

import (
	"fmt"

	dmnxml "github.com/pblumer/temis/internal/xml"
)

// GraphEdit is the desired decision requirements graph for a structural save: the
// complete set of nodes and requirement edges the model should have. ApplyGraph
// reconciles the existing document to it — creating added elements, removing
// absent ones and updating the rest — so the modeler can persist add/delete, not
// just attribute edits (ADR-0016). Because reconciliation is to the FULL set, the
// client must send every node and edge currently on the canvas, not a delta.
type GraphEdit struct {
	Nodes []GraphNodeEdit `json:"nodes"`
	Edges []GraphEdgeEdit `json:"edges"`
}

// GraphNodeEdit is one desired node. Type is "inputData", "decision" or
// "businessKnowledgeModel". DataType (inputData only) sets the declared FEEL type.
// X/Y/Width/Height are the node's DMNDI shape bounds.
type GraphNodeEdit struct {
	ID       string  `json:"id"`
	Type     string  `json:"type"`
	Name     string  `json:"name"`
	DataType string  `json:"dataType,omitempty"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Width    float64 `json:"width"`
	Height   float64 `json:"height"`
}

// GraphEdgeEdit is one desired requirement edge, directed from Source (the
// required element) to Target (the requiring element). Type is
// "informationRequirement" or "knowledgeRequirement".
type GraphEdgeEdit struct {
	Type   string `json:"type"`
	Source string `json:"source"`
	Target string `json:"target"`
}

// ApplyGraph reconciles a DMN document to the desired graph and returns the
// updated XML. It patches in place, preserving every surviving decision's logic
// (decision tables, FEEL) and the rest of the document. New decisions are created
// without logic (undecided), to be filled in via the decision-table editor.
// Requirement edges and DMNDI shapes are reconciled: removed shapes/edges are
// dropped and new shapes use the supplied bounds (a model without DMNDI keeps no
// shapes, so the client auto-lays-out). Edges to or from an unknown node are
// ignored.
func ApplyGraph(src []byte, edit GraphEdit) ([]byte, error) {
	def, err := dmnxml.Decode(src)
	if err != nil {
		return nil, err
	}

	desired := make(map[string]bool, len(edit.Nodes))
	typeOf := make(map[string]string, len(edit.Nodes))
	for _, n := range edit.Nodes {
		if n.ID != "" {
			desired[n.ID] = true
			typeOf[n.ID] = n.Type
		}
	}

	// 1. Remove elements no longer present, collecting their DI refs (the element's
	//    own shape and the edges of every requirement that referenced it).
	var removedRefs []string
	for _, id := range def.ElementIDs() {
		if !desired[id] {
			reqIDs, _ := def.RemoveElement(id)
			removedRefs = append(removedRefs, id)
			removedRefs = append(removedRefs, reqIDs...)
		}
	}

	// 2. Create or update the desired nodes.
	for _, n := range edit.Nodes {
		if n.ID == "" {
			continue
		}
		switch n.Type {
		case "inputData":
			def.UpsertInputData(n.ID, n.Name, n.DataType)
		case "decision":
			def.UpsertDecision(n.ID, n.Name)
		case "businessKnowledgeModel":
			def.UpsertBKM(n.ID, n.Name)
		default:
			return nil, fmt.Errorf("dmn: unknown node type %q for %q", n.Type, n.ID)
		}
	}

	// 3. Reconcile requirement edges (those touching a known node only).
	var edges []dmnxml.ReqEdge
	for _, e := range edit.Edges {
		if !desired[e.Source] || !desired[e.Target] {
			continue
		}
		kind := e.Type
		if kind == "" {
			kind = "informationRequirement"
		}
		edges = append(edges, dmnxml.ReqEdge{Kind: kind, Source: e.Source, Target: e.Target})
	}
	removedRefs = append(removedRefs, def.ReconcileRequirements(edges, typeOf)...)

	// 4. Reconcile the diagram: drop removed shapes/edges, then set or add a shape
	//    for every desired node that carries bounds.
	if def.DMNDI != nil {
		dmnxml.RemoveDIRefs(def.DMNDI, removedRefs)
		for _, n := range edit.Nodes {
			if n.ID == "" || n.Width <= 0 || n.Height <= 0 {
				continue
			}
			dmnxml.UpsertShape(def.DMNDI, n.ID, n.X, n.Y, n.Width, n.Height)
		}
	}

	return dmnxml.Encode(def)
}
