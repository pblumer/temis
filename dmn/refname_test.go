package dmn_test

import (
	"context"
	"strings"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// divergentModel separates each element's free-form display name (@name) from its
// FEEL identifier (variable/@name): the display names carry a hyphen — illegal in
// FEEL — while the variable names are clean. Expressions, inputs and outputs must
// all key on the variable name, never the display label. A second decision
// references the first by its variable name to exercise the required-decision env.
const divergentModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" id="d1" name="Divergent" namespace="ex">
  <inputData id="id_amount" name="Betrag-Brutto">
    <variable name="BetragBrutto" typeRef="number"/>
  </inputData>
  <decision id="id_discount" name="Rabatt-Stufe">
    <variable name="RabattStufe" typeRef="number"/>
    <informationRequirement><requiredInput href="#id_amount"/></informationRequirement>
    <literalExpression>
      <text>BetragBrutto * 0.1</text>
    </literalExpression>
  </decision>
  <decision id="id_net" name="Netto-Preis">
    <variable name="NettoPreis" typeRef="number"/>
    <informationRequirement><requiredInput href="#id_amount"/></informationRequirement>
    <informationRequirement><requiredDecision href="#id_discount"/></informationRequirement>
    <literalExpression>
      <text>BetragBrutto - RabattStufe</text>
    </literalExpression>
  </decision>
</definitions>`

func TestDivergentNameBindsByVariable(t *testing.T) {
	eng := dmn.New()
	defs, diags, err := eng.Compile(context.Background(), []byte(divergentModel))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected compile errors: %+v", diags)
	}

	// The schema keys inputs by their FEEL variable name, not the display label.
	idx := defs.Index()
	if !contains(idx.Inputs, "BetragBrutto") || contains(idx.Inputs, "Betrag-Brutto") {
		t.Fatalf("Index Inputs = %v, want the variable name BetragBrutto (not the display label)", idx.Inputs)
	}
	if !contains(idx.Decisions, "RabattStufe") || !contains(idx.Decisions, "NettoPreis") {
		t.Fatalf("Index Decisions = %v, want the variable names RabattStufe/NettoPreis", idx.Decisions)
	}

	// The decision is reachable by its FEEL variable name.
	dec, err := defs.Decision("NettoPreis")
	if err != nil {
		t.Fatalf("lookup decision by variable name: %v", err)
	}

	// Inputs are supplied under the variable name; the whole chain evaluates and
	// each result is keyed by its variable name. 100 * 0.1 = 10 → 100 - 10 = 90.
	res, err := dec.Evaluate(context.Background(), dmn.Input{"BetragBrutto": 100})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got := res.Outputs["NettoPreis"]; got != "90" {
		t.Errorf("NettoPreis = %v (%T), want \"90\"", got, got)
	}
	if got := res.Decisions["RabattStufe"]; got != "10" {
		t.Errorf("RabattStufe = %v, want \"10\" (result keyed by variable name)", got)
	}
}

// followModel starts with the FEEL name following the display name (no <variable>),
// the common case. Renaming its display label to something FEEL-illegal via a node
// edit, while giving it a clean FEEL identifier, must write an explicit <variable>
// and keep the model evaluable under that identifier.
const followModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" id="d1" name="Follow" namespace="ex">
  <inputData id="id_x" name="X"><variable name="X" typeRef="number"/></inputData>
  <decision id="id_d" name="D">
    <informationRequirement><requiredInput href="#id_x"/></informationRequirement>
    <literalExpression><text>X + 1</text></literalExpression>
  </decision>
</definitions>`

func TestApplyEditsSeparatesDisplayAndFeelName(t *testing.T) {
	label, feelName := "D-Ergebnis", "DErgebnis"
	patched, err := dmn.ApplyEdits([]byte(followModel), []dmn.NodeEdit{
		{ID: "id_d", Name: &label, VarName: &feelName},
	})
	if err != nil {
		t.Fatalf("apply edits: %v", err)
	}

	eng := dmn.New()
	defs, diags, err := eng.Compile(context.Background(), patched)
	if err != nil {
		t.Fatalf("recompile: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected compile errors: %+v", diags)
	}

	// The result is keyed by the FEEL identifier, not the free-form display label.
	dec, err := defs.Decision("DErgebnis")
	if err != nil {
		t.Fatalf("lookup by FEEL name: %v", err)
	}
	res, err := dec.Evaluate(context.Background(), dmn.Input{"X": 41})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got := res.Outputs["DErgebnis"]; got != "42" {
		t.Errorf("DErgebnis = %v, want \"42\"", got)
	}

	// The graph carries the display label as Name and the FEEL identifier as VarName.
	var node dmn.GraphNode
	for _, n := range defs.Graph().Nodes {
		if n.ID == "id_d" {
			node = n
		}
	}
	if node.Name != "D-Ergebnis" || node.VarName != "DErgebnis" {
		t.Errorf("node = {Name:%q VarName:%q}, want {D-Ergebnis DErgebnis}", node.Name, node.VarName)
	}

	// A follow-up edit that makes the FEEL name equal the display name again drops
	// the now-redundant <variable> (the identifier follows the name).
	same := "Ergebnis"
	patched2, err := dmn.ApplyEdits(patched, []dmn.NodeEdit{{ID: "id_d", Name: &same, VarName: &same}})
	if err != nil {
		t.Fatalf("apply follow-up edit: %v", err)
	}
	if strings.Contains(string(patched2), "<variable") && strings.Contains(string(patched2), "DErgebnis") {
		t.Errorf("expected the redundant decision <variable> to be dropped; XML still carries it:\n%s", patched2)
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
