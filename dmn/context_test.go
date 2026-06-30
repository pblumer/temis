package dmn_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/pblumer/temis/dmn"
)

func TestBoxedContextView(t *testing.T) {
	defs := compileModel(t, "boxed_context_15.dmn")

	// Score: two named literal entries that build on each other, plus a result.
	v, ok := defs.BoxedContext("Score")
	if !ok {
		t.Fatal("Score has no boxed context")
	}
	if !v.Simple {
		t.Error("Score context should be simple (all literal entries)")
	}
	want := []dmn.ContextEntryView{
		{Name: "Base", Text: "Points * 2", TypeRef: "number"},
		{Name: "Bonus", Text: "Base + 10", TypeRef: "number"},
	}
	if !reflect.DeepEqual(v.Entries, want) {
		t.Errorf("entries = %+v, want %+v", v.Entries, want)
	}
	if v.Result != "Bonus" {
		t.Errorf("result = %q, want Bonus", v.Result)
	}

	// Profile: two entries, no result cell.
	p, ok := defs.BoxedContext("Profile")
	if !ok {
		t.Fatal("Profile has no boxed context")
	}
	if p.Result != "" {
		t.Errorf("Profile should have no result cell, got %q", p.Result)
	}
	if len(p.Entries) != 2 {
		t.Errorf("Profile entries = %d, want 2", len(p.Entries))
	}

	// A decision table is not a boxed context.
	if _, ok := compileModel(t, "dish_15.dmn").BoxedContext("Dish"); ok {
		t.Error("a decision table must not be reported as a boxed context")
	}
}

func TestSetBoxedContextRoundTrip(t *testing.T) {
	src := readModel(t, "boxed_context_15.dmn")

	// Re-base Score on triple the points: Base = Points*3 → Bonus = Base+10.
	edit := dmn.ContextEdit{
		Entries: []dmn.ContextEntryView{
			{Name: "Base", Text: "Points * 3", TypeRef: "number"},
			{Name: "Bonus", Text: "Base + 10", TypeRef: "number"},
		},
		Result: "Bonus",
	}
	out, err := dmn.SetBoxedContext(src, "id_score", edit)
	if err != nil {
		t.Fatalf("SetBoxedContext: %v", err)
	}

	defs, diags, err := dmn.New().Compile(context.Background(), out)
	if err != nil {
		t.Fatalf("compile edited model: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("edited context has compile errors: %+v", diags)
	}
	dec, err := defs.Decision("Score")
	if err != nil {
		t.Fatalf("decision: %v", err)
	}
	res, err := dec.Evaluate(context.Background(), dmn.Input{"Points": 5})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got := res.Outputs["Score"]; got != "25" { // (5*3)+10
		t.Errorf("Score(Points=5) = %v, want 25 after edit", got)
	}

	// The edit round-trips back through the view.
	v, ok := defs.BoxedContext("Score")
	if !ok || len(v.Entries) != 2 || v.Entries[0].Text != "Points * 3" || v.Entries[0].TypeRef != "number" {
		t.Errorf("view after edit = %+v (ok=%v)", v, ok)
	}
}

func TestCreateBoxedContext(t *testing.T) {
	src := readModel(t, "boxed_context_15.dmn")

	// Add an undecided "Tier" decision requiring Points, then give it a context.
	wired := graphEdit(t, src)
	wired.Nodes = append(wired.Nodes, dmn.GraphNodeEdit{ID: "id_tier", Type: "decision", Name: "Tier", X: 600, Y: 260, Width: 150, Height: 70})
	wired.Edges = append(wired.Edges, dmn.GraphEdgeEdit{Type: "informationRequirement", Source: "id_points", Target: "id_tier"})
	withDecision, err := dmn.ApplyGraph(src, wired)
	if err != nil {
		t.Fatalf("ApplyGraph: %v", err)
	}

	out, err := dmn.CreateBoxedContext(withDecision, "id_tier")
	if err != nil {
		t.Fatalf("CreateBoxedContext: %v", err)
	}
	defs, diags, err := dmn.New().Compile(context.Background(), out)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("new context has compile errors: %+v", diags)
	}
	v, ok := defs.BoxedContext("Tier")
	if !ok || len(v.Entries) != 1 {
		t.Fatalf("Tier context = %+v (ok=%v), want one entry", v, ok)
	}
	// A context of named entries (no result) evaluates to a context value.
	dec, _ := defs.Decision("Tier")
	res, err := dec.Evaluate(context.Background(), dmn.Input{"Points": 5})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got, want := res.Outputs["Tier"], map[string]any{"Eintrag 1": "0"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Tier = %#v, want %#v", got, want)
	}
}

func TestSetBoxedContextRejects(t *testing.T) {
	// Non-context logic (a decision table) is rejected.
	if _, err := dmn.SetBoxedContext(readModel(t, "dish_15.dmn"), "id_dish",
		dmn.ContextEdit{Entries: []dmn.ContextEntryView{{Name: "x", Text: "1"}}}); err == nil {
		t.Error("expected error setting a context on a decision-table decision")
	}
	// An entry without a name or without an expression is rejected.
	src := readModel(t, "boxed_context_15.dmn")
	if _, err := dmn.SetBoxedContext(src, "id_score", dmn.ContextEdit{Entries: []dmn.ContextEntryView{{Name: "", Text: "1"}}}); err == nil {
		t.Error("expected error for an unnamed entry")
	}
	if _, err := dmn.SetBoxedContext(src, "id_score", dmn.ContextEdit{Entries: []dmn.ContextEntryView{{Name: "x", Text: ""}}}); err == nil {
		t.Error("expected error for an empty expression")
	}
}
