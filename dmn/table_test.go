package dmn_test

import (
	"context"
	"testing"

	"github.com/pblumer/temis/dmn"
)

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

// evalDish compiles xml and evaluates the Dish decision for the given season and
// guest count, returning the resulting dish name.
func evalDish(t *testing.T, xml []byte, season string, guests float64) string {
	t.Helper()
	defs, diags, err := dmn.New().Compile(context.Background(), xml)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("compile diagnostics: %+v", diags)
	}
	dec, err := defs.Decision("Dish")
	if err != nil {
		t.Fatalf("decision: %v", err)
	}
	res, err := dec.Evaluate(context.Background(), dmn.Input{"Season": season, "Guest Count": guests})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	s, _ := res.Outputs["Dish"].(string)
	return s
}

// TestApplyTableEditCell edits an output cell and checks the recompiled table
// evaluates to the new value, while a row it did not touch is unchanged.
func TestApplyTableEditCell(t *testing.T) {
	src := readModel(t, "dish_15.dmn")
	tv, _ := mustTable(t, src)

	// Rewrite the "Winter / <= 8 -> Roastbeef" rule's output to "Lobster".
	rules := toEdit(tv)
	for i := range rules {
		if rules[i].OutputEntries[0] == `"Roastbeef"` {
			rules[i].OutputEntries[0] = `"Lobster"`
		}
	}
	out, err := dmn.ApplyTableEdit(src, tv.DecisionID, dmn.TableEdit{Rules: rules})
	if err != nil {
		t.Fatalf("ApplyTableEdit: %v", err)
	}
	if got := evalDish(t, out, "Winter", 4); got != "Lobster" {
		t.Errorf("Winter/4 = %q, want Lobster", got)
	}
	if got := evalDish(t, out, "Fall", 4); got != "Spareribs" {
		t.Errorf("untouched Fall/4 = %q, want Spareribs", got)
	}
}

// TestApplyTableEditAddRule appends a rule and checks it now matches.
func TestApplyTableEditAddRule(t *testing.T) {
	src := readModel(t, "dish_15.dmn")
	tv, _ := mustTable(t, src)

	rules := toEdit(tv)
	rules = append(rules, dmn.TableRule{InputEntries: []string{`"Summer"`, `<= 8`}, OutputEntries: []string{`"Salad"`}})
	out, err := dmn.ApplyTableEdit(src, tv.DecisionID, dmn.TableEdit{Rules: rules})
	if err != nil {
		t.Fatalf("ApplyTableEdit: %v", err)
	}
	if got := evalDish(t, out, "Summer", 4); got != "Salad" {
		t.Errorf("added Summer/4 = %q, want Salad", got)
	}
}

// TestApplyTableEditEmptyInputIsAny checks an empty input cell is stored as "-"
// (matches any), so the rule fires regardless of that column.
func TestApplyTableEditEmptyInputIsAny(t *testing.T) {
	src := readModel(t, "dish_15.dmn")
	tv, _ := mustTable(t, src)

	// One catch-all rule: any season, any guests -> "Bread".
	out, err := dmn.ApplyTableEdit(src, tv.DecisionID, dmn.TableEdit{Rules: []dmn.TableRule{
		{InputEntries: []string{"", ""}, OutputEntries: []string{`"Bread"`}},
	}})
	if err != nil {
		t.Fatalf("ApplyTableEdit: %v", err)
	}
	if got := evalDish(t, out, "Whatever", 99); got != "Bread" {
		t.Errorf("catch-all = %q, want Bread", got)
	}
}

// TestApplyTableEditNoTable errors for a decision without a decision table.
func TestApplyTableEditNoTable(t *testing.T) {
	src := readModel(t, "pricing_15.dmn") // "Net Total" is a literal expression
	if _, err := dmn.ApplyTableEdit(src, "id_net", dmn.TableEdit{}); err == nil {
		t.Error("expected error editing a non-table decision")
	}
}

func mustTable(t *testing.T, src []byte) (dmn.TableView, bool) {
	t.Helper()
	defs, _, err := dmn.New().Compile(context.Background(), src)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	tv, ok := defs.DecisionTable("Dish")
	if !ok {
		t.Fatal("Dish has no table")
	}
	return tv, ok
}

// toEdit converts a TableView's rows into editable TableRules.
func toEdit(tv dmn.TableView) []dmn.TableRule {
	rules := make([]dmn.TableRule, len(tv.Rules))
	for i, r := range tv.Rules {
		rules[i] = dmn.TableRule{
			InputEntries:  append([]string(nil), r.InputEntries...),
			OutputEntries: append([]string(nil), r.OutputEntries...),
			Annotations:   append([]string(nil), r.Annotations...),
		}
	}
	return rules
}

// TestCreateDecisionTable adds an undecided decision wired to two inputs, gives it
// a table, and checks the table's input columns are derived from its requirements
// (with a single output named after the decision) and that it compiles cleanly.
func TestCreateDecisionTable(t *testing.T) {
	src := readModel(t, "dish_15.dmn")

	// Add an undecided "Pairing" decision requiring Season and Guest Count.
	wired := graphEdit(t, src)
	wired.Nodes = append(wired.Nodes, dmn.GraphNodeEdit{ID: "id_pairing", Type: "decision", Name: "Pairing", X: 600, Y: 260, Width: 150, Height: 70})
	wired.Edges = append(wired.Edges,
		dmn.GraphEdgeEdit{Type: "informationRequirement", Source: "id_season", Target: "id_pairing"},
		dmn.GraphEdgeEdit{Type: "informationRequirement", Source: "id_guests", Target: "id_pairing"},
	)
	withDecision, err := dmn.ApplyGraph(src, wired)
	if err != nil {
		t.Fatalf("ApplyGraph: %v", err)
	}

	out, err := dmn.CreateDecisionTable(withDecision, "id_pairing")
	if err != nil {
		t.Fatalf("CreateDecisionTable: %v", err)
	}

	defs, diags, err := dmn.New().Compile(context.Background(), out)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("new table has compile errors: %+v", diags)
	}
	tv, ok := defs.DecisionTable("Pairing")
	if !ok {
		t.Fatal("Pairing has no table after create")
	}
	if tv.HitPolicy != "U" {
		t.Errorf("hit policy = %q, want U", tv.HitPolicy)
	}
	gotInputs := []string{}
	for _, in := range tv.Inputs {
		gotInputs = append(gotInputs, in.Expression)
	}
	if len(gotInputs) != 2 || gotInputs[0] != "Season" || gotInputs[1] != "Guest Count" {
		t.Errorf("input columns = %v, want [Season, Guest Count] from requirements", gotInputs)
	}
	if len(tv.Outputs) != 1 || tv.Outputs[0].Name != "Pairing" {
		t.Errorf("outputs = %+v, want one named Pairing", tv.Outputs)
	}
	if len(tv.Rules) != 0 {
		t.Errorf("new table should start with no rules, got %d", len(tv.Rules))
	}
}

// TestCreateDecisionTableRejectsDecided errors when the decision already has logic.
func TestCreateDecisionTableRejectsDecided(t *testing.T) {
	src := readModel(t, "dish_15.dmn")
	if _, err := dmn.CreateDecisionTable(src, "id_dish"); err == nil {
		t.Error("expected error creating a table for a decision that already has one")
	}
	if _, err := dmn.CreateDecisionTable(src, "nope"); err == nil {
		t.Error("expected error for unknown decision")
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
