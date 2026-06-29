package dmn_test

import "testing"

// TestDecisionTableView checks the decision-table view exposes hit policy, the
// input/output columns and the rule rows for a table decision.
func TestDecisionTableView(t *testing.T) {
	defs := compileModel(t, "dish_15.dmn")
	tv, ok := defs.DecisionTable("Dish")
	if !ok {
		t.Fatal("Dish should have a decision table")
	}
	if tv.HitPolicy == "" {
		t.Error("hit policy missing")
	}
	if len(tv.Inputs) == 0 || len(tv.Outputs) == 0 {
		t.Fatalf("inputs=%d outputs=%d, want both non-empty", len(tv.Inputs), len(tv.Outputs))
	}
	if len(tv.Rules) == 0 {
		t.Fatal("no rule rows")
	}
	for i, r := range tv.Rules {
		if len(r.InputEntries) != len(tv.Inputs) {
			t.Errorf("rule %d has %d input entries, want %d (aligned with inputs)", i, len(r.InputEntries), len(tv.Inputs))
		}
	}
}

// TestDecisionTableViewAbsent checks a non-table decision (or unknown id) yields
// ok=false so the modeler can fall back.
func TestDecisionTableViewAbsent(t *testing.T) {
	defs := compileModel(t, "pricing_15.dmn")
	// "Net Total" in pricing is a literal expression, not a decision table.
	if _, ok := defs.DecisionTable("Net Total"); ok {
		t.Error("Net Total is a literal expression; DecisionTable should report ok=false")
	}
	if _, ok := defs.DecisionTable("does-not-exist"); ok {
		t.Error("unknown decision should report ok=false")
	}
}

// TestGraphMarksTableDecisions checks the graph flags decisions whose logic is a
// decision table, so the client knows which nodes open a table on double-click.
func TestGraphMarksTableDecisions(t *testing.T) {
	defs := compileModel(t, "dish_15.dmn")
	var sawTable bool
	for _, n := range defs.Graph().Nodes {
		if n.Name == "Dish" {
			sawTable = n.HasTable
		}
		if n.Type != "decision" && n.HasTable {
			t.Errorf("non-decision %q marked hasTable", n.Name)
		}
	}
	if !sawTable {
		t.Error("Dish decision not marked hasTable")
	}
}
