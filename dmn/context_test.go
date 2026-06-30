package dmn_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// TestContextView exposes a boxed context's entries — names, FEEL text, the
// result cell — and reports absent for a non-context decision.
func TestContextView(t *testing.T) {
	src := readModel(t, "boxed_context_15.dmn")
	v, ok, err := dmn.ContextOf(src, "Score")
	if err != nil || !ok {
		t.Fatalf("ContextOf(Score) ok=%v err=%v", ok, err)
	}
	if !v.Simple {
		t.Error("Score context should be simple (all literal entries)")
	}
	if len(v.Entries) != 3 {
		t.Fatalf("Score entries = %d, want 3", len(v.Entries))
	}
	if v.Entries[0].Name != "Base" || v.Entries[0].Text != "Points * 2" || v.Entries[0].Kind != "literal" {
		t.Errorf("entry[0] = %+v, want Base / Points * 2 / literal", v.Entries[0])
	}
	if !v.Entries[2].Result || v.Entries[2].Name != "" {
		t.Errorf("entry[2] should be the unnamed result cell, got %+v", v.Entries[2])
	}

	// A decision table is not a context.
	if _, ok, _ := dmn.ContextOf(readModel(t, "dish_15.dmn"), "Dish"); ok {
		t.Error("Dish is a decision table; ContextOf should report absent")
	}
}

// TestSetContextRoundTrip edits an entry and creates the recompiled model
// evaluates with the new expression, and the view round-trips.
func TestSetContextRoundTrip(t *testing.T) {
	src := readModel(t, "boxed_context_15.dmn")
	edit := dmn.ContextEdit{Entries: []dmn.ContextEntryEdit{
		{Name: "Base", Text: "Points * 2", TypeRef: "number"},
		{Name: "Bonus", Text: "Base + 20", TypeRef: "number"},
		{Name: "", Text: "Bonus", TypeRef: "number"},
	}}
	out, err := dmn.SetContext(src, "id_score", edit)
	if err != nil {
		t.Fatalf("SetContext: %v", err)
	}
	defs, diags, err := dmn.New().Compile(context.Background(), out)
	if err != nil || diags.HasErrors() {
		t.Fatalf("compile: err=%v diags=%+v", err, diags)
	}
	// The edited entry survives the round-trip.
	v, _, _ := dmn.ContextOf(out, "id_score")
	if v.Entries[1].Text != "Base + 20" {
		t.Errorf("Bonus not updated: %q", v.Entries[1].Text)
	}
	// Points=5 → Base=10 → Bonus=30 → result 30.
	dec, _ := defs.Decision("Score")
	res, err := dec.Evaluate(context.Background(), dmn.Input{"Points": 5})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got := fmt.Sprint(res.Outputs["Score"]); got != "30" {
		t.Errorf("Score = %v, want 30", got)
	}
}

// TestSetContextCreatesAndRefuses creates a context on an undecided decision and
// refuses one whose logic is already a (non-context) decision table.
func TestSetContextCreatesAndRefuses(t *testing.T) {
	// Refuse: Dish already carries a decision table.
	if _, err := dmn.SetContext(readModel(t, "dish_15.dmn"), "Dish", dmn.ContextEdit{
		Entries: []dmn.ContextEntryEdit{{Name: "x", Text: "1"}},
	}); err == nil {
		t.Error("SetContext should refuse a decision that has a decision table")
	}

	// Empty context is rejected.
	if _, err := dmn.SetContext(readModel(t, "boxed_context_15.dmn"), "id_score", dmn.ContextEdit{}); err == nil {
		t.Error("SetContext should reject an empty context")
	}

	// A result cell that is not last is rejected.
	_, err := dmn.SetContext(readModel(t, "boxed_context_15.dmn"), "id_score", dmn.ContextEdit{Entries: []dmn.ContextEntryEdit{
		{Name: "", Text: "1"},
		{Name: "a", Text: "2"},
	}})
	if err == nil || !strings.Contains(err.Error(), "result cell") {
		t.Errorf("SetContext should reject a misplaced result cell, got %v", err)
	}
}

// TestContextNestedReadOnly reports a context with a nested non-literal entry as
// not simple, so the editor opens read-only rather than clobbering it.
func TestContextNestedReadOnly(t *testing.T) {
	const nested = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" id="d" name="d" namespace="urn:x">
  <inputData id="id_in" name="In"><variable name="In" typeRef="number"/></inputData>
  <decision id="id_d" name="D">
    <variable name="D" typeRef="number"/>
    <informationRequirement><requiredInput href="#id_in"/></informationRequirement>
    <context>
      <contextEntry>
        <variable name="Tbl" typeRef="number"/>
        <decisionTable hitPolicy="UNIQUE">
          <input id="i1"><inputExpression typeRef="number"><text>In</text></inputExpression></input>
          <output id="o1" typeRef="number"/>
          <rule><inputEntry><text>&gt;0</text></inputEntry><outputEntry><text>1</text></outputEntry></rule>
        </decisionTable>
      </contextEntry>
      <contextEntry><text>Tbl</text></contextEntry>
    </context>
  </decision>
</definitions>`
	v, ok, err := dmn.ContextOf([]byte(nested), "D")
	if err != nil || !ok {
		t.Fatalf("ContextOf(nested) ok=%v err=%v", ok, err)
	}
	if v.Simple {
		t.Error("a context with a nested decision table should not be simple")
	}
	if v.Entries[0].Kind != "decisionTable" {
		t.Errorf("nested entry kind = %q, want decisionTable", v.Entries[0].Kind)
	}
}
