package dmn_test

import "testing"

// TestGraphChaining checks Graph() exposes the DRG nodes (with their kinds) and
// the requirement edges, including a decision→decision chain.
func TestGraphChaining(t *testing.T) {
	defs := compileModel(t, "routing_13.dmn")
	g := defs.Graph()

	kind := map[string]string{}
	id := map[string]string{}
	for _, n := range g.Nodes {
		kind[n.Name] = n.Type
		id[n.Name] = n.ID
	}
	if len(g.Nodes) != 3 {
		t.Fatalf("nodes = %d, want 3 (%+v)", len(g.Nodes), g.Nodes)
	}
	if kind["Applicant Age"] != "inputData" {
		t.Errorf("Applicant Age kind = %q, want inputData", kind["Applicant Age"])
	}
	if kind["Eligibility"] != "decision" || kind["Routing"] != "decision" {
		t.Errorf("Eligibility/Routing kinds = %q/%q, want decision", kind["Eligibility"], kind["Routing"])
	}

	type edge struct{ src, tgt, typ string }
	have := map[edge]bool{}
	for _, e := range g.Edges {
		have[edge{e.Source, e.Target, e.Type}] = true
	}
	if len(g.Edges) != 2 {
		t.Fatalf("edges = %d, want 2 (%+v)", len(g.Edges), g.Edges)
	}
	if !have[edge{id["Applicant Age"], id["Eligibility"], "informationRequirement"}] {
		t.Error("missing informationRequirement Applicant Age → Eligibility")
	}
	if !have[edge{id["Eligibility"], id["Routing"], "informationRequirement"}] {
		t.Error("missing informationRequirement Eligibility → Routing (decision chain)")
	}
}

// TestGraphKnowledgeRequirement checks BKM nodes and the knowledgeRequirement
// edge kind appear in the graph.
func TestGraphKnowledgeRequirement(t *testing.T) {
	defs := compileModel(t, "bkm_invocation_15.dmn")
	g := defs.Graph()

	var bkm string
	for _, n := range g.Nodes {
		if n.Type == "businessKnowledgeModel" {
			bkm = n.ID
		}
	}
	if bkm == "" {
		t.Fatalf("no businessKnowledgeModel node in %+v", g.Nodes)
	}
	var hasKnowledge bool
	for _, e := range g.Edges {
		if e.Type == "knowledgeRequirement" && e.Source == bkm {
			hasKnowledge = true
		}
	}
	if !hasKnowledge {
		t.Errorf("no knowledgeRequirement edge from BKM %q in %+v", bkm, g.Edges)
	}
}
