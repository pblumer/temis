package dmn_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/pblumer/temis/dmn"
)

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func recompile(t *testing.T, src []byte) *dmn.Definitions {
	t.Helper()
	defs, diags, err := dmn.New().Compile(context.Background(), src)
	if err != nil || diags.HasErrors() {
		t.Fatalf("compile: err=%v diags=%+v", err, diags)
	}
	return defs
}

// TestLogicView_BKMBoxedBodies reads every boxed kind as a BKM body through the
// anchored logic view (the modeler's read path for a BKM's boxed body, WP-66).
func TestLogicView_BKMBoxedBodies(t *testing.T) {
	defs := compileModel(t, "bkm_boxed_bodies_16.dmn")
	for _, c := range []struct{ id, kind string }{
		{"id_ctx", "context"}, {"id_lst", "list"}, {"id_rel", "relation"},
		{"id_inv", "invocation"}, {"id_iter", "iterator"}, {"id_cond", "conditional"},
		{"id_filt", "filter"}, {"id_tab", "table"},
	} {
		v, ok := defs.LogicView(dmn.Anchor{Kind: "bkm", ID: c.id}, c.kind)
		if !ok || v == nil {
			t.Errorf("LogicView(bkm %s, %s) = (%v, %v), want a view", c.id, c.kind, v, ok)
		}
	}

	// The BKM view reports the boxed body's kind so the modeler opens that editor.
	if b, _ := defs.BKMFunction("id_iter"); b.Simple || b.BodyKind != "iterator" {
		t.Errorf("Iter BKM view = %+v, want boxed iterator", b)
	}
	// A boxed context reads its entries and result.
	if v, ok := defs.LogicView(dmn.Anchor{Kind: "bkm", ID: "id_ctx"}, "context"); !ok || v.(dmn.ContextView).Result != "p * factor" {
		t.Errorf("Ctx view = %+v (ok=%v)", v, ok)
	}

	// Wrong kind, unknown anchor kind, unknown id and unknown kind all fail.
	if _, ok := defs.LogicView(dmn.Anchor{Kind: "bkm", ID: "id_tab"}, "context"); ok {
		t.Error("a table body must not read as a context")
	}
	if _, ok := defs.LogicView(dmn.Anchor{Kind: "nope", ID: "id_tab"}, "table"); ok {
		t.Error("unknown anchor kind must fail")
	}
	if _, ok := defs.LogicView(dmn.Anchor{Kind: "bkm", ID: "nope"}, "table"); ok {
		t.Error("unknown bkm must fail")
	}
	if _, ok := defs.LogicView(dmn.Anchor{Kind: "bkm", ID: "id_tab"}, "bogus"); ok {
		t.Error("unknown kind must fail")
	}
}

// TestSetLogic_BKMBoxedBodies rewrites every boxed kind as a BKM body and confirms
// the model still compiles — exercising every buildBodyExpr branch.
func TestSetLogic_BKMBoxedBodies(t *testing.T) {
	src := readModel(t, "bkm_boxed_bodies_16.dmn")
	for _, e := range []struct {
		id, kind string
		edit     any
	}{
		{"id_ctx", "context", dmn.ContextEdit{Entries: []dmn.ContextEntryView{{Name: "factor", Text: "3"}}, Result: "p * factor"}},
		{"id_lst", "list", dmn.ListEdit{Items: []string{"p", "p * 3"}}},
		{"id_rel", "relation", dmn.RelationEdit{Columns: []string{"a", "b"}, Rows: [][]string{{"p", "p + 2"}}}},
		{"id_inv", "invocation", dmn.InvocationEdit{Called: "Ctx", Bindings: []dmn.InvocationBindingView{{Parameter: "p", Value: "p"}}}},
		{"id_iter", "iterator", dmn.IteratorEdit{Kind: "for", Variable: "x", In: "[1, 2, 3]", Body: "x * p"}},
		{"id_cond", "conditional", dmn.ConditionalEdit{If: "p > 0", Then: "2", Else: "0"}},
		{"id_filt", "filter", dmn.FilterEdit{In: "[1, 2, 3]", Match: "item > p"}},
		{"id_tab", "table", dmn.TableEdit{Rules: []dmn.TableRule{{InputEntries: []string{">= 90"}, OutputEntries: []string{`"A"`}}, {InputEntries: []string{"< 90"}, OutputEntries: []string{`"B"`}}}}},
	} {
		out, err := dmn.SetLogic(src, dmn.Anchor{Kind: "bkm", ID: e.id}, e.kind, mustJSON(t, e.edit))
		if err != nil {
			t.Errorf("SetLogic(bkm %s, %s): %v", e.id, e.kind, err)
			continue
		}
		recompile(t, out)
	}

	// Editing the table body changes Root's evaluation and preserves the parameter.
	out, err := dmn.SetLogic(src, dmn.Anchor{Kind: "bkm", ID: "id_tab"}, "table",
		mustJSON(t, dmn.TableEdit{Rules: []dmn.TableRule{{InputEntries: []string{">= 80"}, OutputEntries: []string{`"top"`}}, {InputEntries: []string{"< 80"}, OutputEntries: []string{`"B"`}}}}))
	if err != nil {
		t.Fatal(err)
	}
	dec, _ := recompile(t, out).Decision("Root")
	res, err := dec.Evaluate(context.Background(), dmn.Input{"In": 85})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.Outputs["Root"] != "top" {
		t.Errorf("Root(85) = %v, want top after editing the BKM table body", res.Outputs["Root"])
	}
	if b, _ := recompile(t, out).BKMFunction("id_tab"); len(b.Params) != 1 {
		t.Errorf("BKM params dropped: %+v", b)
	}
}

// TestSetLogic_Errors covers the rejection paths of the anchored write route.
func TestSetLogic_Errors(t *testing.T) {
	src := readModel(t, "bkm_boxed_bodies_16.dmn")
	cases := []struct {
		name   string
		anchor dmn.Anchor
		kind   string
		edit   json.RawMessage
	}{
		{"unknown anchor kind", dmn.Anchor{Kind: "nope", ID: "id_lst"}, "list", mustJSON(t, dmn.ListEdit{Items: []string{"1"}})},
		{"unknown kind", dmn.Anchor{Kind: "bkm", ID: "id_lst"}, "bogus", nil},
		{"empty list", dmn.Anchor{Kind: "bkm", ID: "id_lst"}, "list", mustJSON(t, dmn.ListEdit{})},
		{"empty context", dmn.Anchor{Kind: "bkm", ID: "id_ctx"}, "context", mustJSON(t, dmn.ContextEdit{})},
		{"context missing name", dmn.Anchor{Kind: "bkm", ID: "id_ctx"}, "context", mustJSON(t, dmn.ContextEdit{Entries: []dmn.ContextEntryView{{Text: "1"}}})},
		{"relation no column", dmn.Anchor{Kind: "bkm", ID: "id_rel"}, "relation", mustJSON(t, dmn.RelationEdit{})},
		{"invocation no callee", dmn.Anchor{Kind: "bkm", ID: "id_inv"}, "invocation", mustJSON(t, dmn.InvocationEdit{})},
		{"iterator bad kind", dmn.Anchor{Kind: "bkm", ID: "id_iter"}, "iterator", mustJSON(t, dmn.IteratorEdit{Kind: "loop", Variable: "x", In: "[1]", Body: "x"})},
		{"conditional empty branch", dmn.Anchor{Kind: "bkm", ID: "id_cond"}, "conditional", mustJSON(t, dmn.ConditionalEdit{If: "true", Then: "1"})},
		{"filter empty", dmn.Anchor{Kind: "bkm", ID: "id_filt"}, "filter", mustJSON(t, dmn.FilterEdit{In: "[1]"})},
		{"kind switch rejected", dmn.Anchor{Kind: "bkm", ID: "id_tab"}, "list", mustJSON(t, dmn.ListEdit{Items: []string{"1"}})},
		{"unknown bkm", dmn.Anchor{Kind: "bkm", ID: "nope"}, "list", mustJSON(t, dmn.ListEdit{Items: []string{"1"}})},
	}
	for _, c := range cases {
		if _, err := dmn.SetLogic(src, c.anchor, c.kind, c.edit); err == nil {
			t.Errorf("%s: expected an error", c.name)
		}
	}
}

// TestLogicView_DecisionAnchor confirms the anchored route also serves a decision's
// own logic (the same views the decision routes return).
func TestLogicView_DecisionAnchor(t *testing.T) {
	if v, ok := compileModel(t, "boxed_context_15.dmn").LogicView(dmn.Anchor{Kind: "decision", ID: "Score"}, "context"); !ok || v.(dmn.ContextView).Result != "Bonus" {
		t.Errorf("decision context view = %+v (ok=%v)", v, ok)
	}
	if _, ok := compileModel(t, "dish_15.dmn").LogicView(dmn.Anchor{Kind: "decision", ID: "Dish"}, "table"); !ok {
		t.Error("dish table not viewable via the logic route")
	}
	if _, ok := compileModel(t, "dish_15.dmn").LogicView(dmn.Anchor{Kind: "decision", ID: "nope"}, "table"); ok {
		t.Error("unknown decision must fail")
	}
}

// TestSetLogic_DecisionDelegation confirms the decision anchor delegates to the
// existing per-kind setters (a round-trip evaluation proves it).
func TestSetLogic_DecisionDelegation(t *testing.T) {
	out, err := dmn.SetLogic(readModel(t, "boxed_context_15.dmn"), dmn.Anchor{Kind: "decision", ID: "id_score"}, "context",
		mustJSON(t, dmn.ContextEdit{Entries: []dmn.ContextEntryView{{Name: "Base", Text: "Points * 3"}, {Name: "Bonus", Text: "Base + 10"}}, Result: "Bonus"}))
	if err != nil {
		t.Fatal(err)
	}
	dec, _ := recompile(t, out).Decision("Score")
	res, _ := dec.Evaluate(context.Background(), dmn.Input{"Points": 5})
	if res.Outputs["Score"] != "25" {
		t.Errorf("Score(Points=5) = %v, want 25", res.Outputs["Score"])
	}
}
