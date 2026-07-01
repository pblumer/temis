package dmn_test

import (
	"context"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// bkmBoxedBodyModel has a BKM whose encapsulated logic body is a boxed
// expression (a decision table), not a literal — so the simple BKM view reports
// Simple=false (the default arm of BKMFunction's body switch). A second BKM with
// a knowledge requirement on it drives the BKM knowledge-edge branch of Graph,
// and a decision with a dangling required-decision reference drives Graph's
// unknown-endpoint edge skip.
const bkmBoxedBodyModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/bkmboxed" name="BkmBoxed" id="def_bkmboxed">
  <inputData id="i_n" name="N"><variable name="N" typeRef="number"/></inputData>
  <businessKnowledgeModel id="bkm_boxed" name="Boxed">
    <encapsulatedLogic>
      <formalParameter name="x" typeRef="number"/>
      <decisionTable hitPolicy="UNIQUE">
        <input id="bi1" label="x"><inputExpression typeRef="number"><text>x</text></inputExpression></input>
        <output id="bo1" name="r" typeRef="string"/>
        <rule><inputEntry><text>-</text></inputEntry><outputEntry><text>"any"</text></outputEntry></rule>
      </decisionTable>
    </encapsulatedLogic>
  </businessKnowledgeModel>
  <businessKnowledgeModel id="bkm_user" name="User">
    <knowledgeRequirement><requiredKnowledge href="#bkm_boxed"/></knowledgeRequirement>
    <encapsulatedLogic><literalExpression><text>1</text></literalExpression></encapsulatedLogic>
  </businessKnowledgeModel>
  <decision id="d_n" name="UseN">
    <informationRequirement><requiredInput href="#i_n"/></informationRequirement>
    <literalExpression><text>N + 1</text></literalExpression>
  </decision>
</definitions>`

// TestBKMBoxedBodyAndGraphEdges covers BKMFunction's boxed-body (not-simple) arm
// and Graph's knowledge-requirement edge between two BKMs.
func TestBKMBoxedBodyAndGraphEdges(t *testing.T) {
	defs, diags, err := dmn.New().Compile(context.Background(), []byte(bkmBoxedBodyModel))
	if err != nil {
		t.Fatal(err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected compile errors: %+v", diags)
	}
	v, ok := defs.BKMFunction("Boxed")
	if !ok {
		t.Fatal("Boxed BKM not found")
	}
	if v.Simple {
		t.Error("a boxed-body BKM should report Simple=false")
	}

	// Graph has a knowledge-requirement edge from Boxed to User.
	g := defs.Graph()
	foundEdge := false
	for _, e := range g.Edges {
		if e.Type == "knowledgeRequirement" && e.Source == "bkm_boxed" && e.Target == "bkm_user" {
			foundEdge = true
		}
	}
	if !foundEdge {
		t.Errorf("want a knowledgeRequirement edge bkm_boxed→bkm_user, got %+v", g.Edges)
	}
}

// danglingRefModel has a decision whose required-decision reference points at an
// id no node carries, so Graph must skip the dangling edge.
const danglingRefModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/dangling" name="Dangling" id="def_dangling">
  <inputData id="i_n" name="N"><variable name="N" typeRef="number"/></inputData>
  <decision id="d_n" name="UseN">
    <informationRequirement><requiredInput href="#i_n"/></informationRequirement>
    <informationRequirement><requiredDecision href="#ghost"/></informationRequirement>
    <literalExpression><text>N + 1</text></literalExpression>
  </decision>
</definitions>`

// TestGraphSkipsDanglingEdge covers Graph's unknown-endpoint edge skip.
func TestGraphSkipsDanglingEdge(t *testing.T) {
	defs, _, err := dmn.New().Compile(context.Background(), []byte(danglingRefModel))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range defs.Graph().Edges {
		if e.Source == "ghost" || e.Target == "ghost" {
			t.Errorf("dangling edge to ghost should be skipped, got %+v", e)
		}
	}
}

// dupInputModel lists the same input data twice in a decision's requirements, so
// buildInputSchema (and envNames/reqInputNames) must dedup it.
const dupInputModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/dup" name="Dup" id="def_dup">
  <inputData id="i_n" name="N"><variable name="N" typeRef="number"/></inputData>
  <decision id="d_n" name="UseN">
    <informationRequirement><requiredInput href="#i_n"/></informationRequirement>
    <informationRequirement><requiredInput href="#i_n"/></informationRequirement>
    <literalExpression><text>N + 1</text></literalExpression>
  </decision>
</definitions>`

// TestDuplicateRequiredInputDeduped covers the duplicate-name skip in
// buildInputSchema, envNames and reqInputNames: the repeated input appears once.
func TestDuplicateRequiredInputDeduped(t *testing.T) {
	defs, diags, err := dmn.New().Compile(context.Background(), []byte(dupInputModel))
	if err != nil {
		t.Fatal(err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected compile errors: %+v", diags)
	}
	dec, err := defs.Decision("UseN")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, f := range dec.InputSchema() {
		if f.Name == "N" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("N should appear once in the schema, got %d", n)
	}
	// And it still evaluates (the env has N).
	res, err := dec.Evaluate(context.Background(), dmn.Input{"N": 1})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.Outputs["UseN"] != "2" {
		t.Errorf("UseN(1) = %v, want 2", res.Outputs["UseN"])
	}
}

// selfRefModel wires a decision to require itself, which wireRequirements must
// drop (req == cd) before cycle detection — covering that guard.
const selfRefModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/self" name="Self" id="def_self">
  <inputData id="i_n" name="N"><variable name="N" typeRef="number"/></inputData>
  <decision id="d_n" name="UseN">
    <informationRequirement><requiredInput href="#i_n"/></informationRequirement>
    <informationRequirement><requiredDecision href="#d_n"/></informationRequirement>
    <literalExpression><text>N + 1</text></literalExpression>
  </decision>
</definitions>`

// TestSelfRequirementDropped covers wireRequirements's self-reference guard: a
// decision requiring itself is dropped, so it has no cycle diagnostic and still
// evaluates.
func TestSelfRequirementDropped(t *testing.T) {
	defs, diags, err := dmn.New().Compile(context.Background(), []byte(selfRefModel))
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range diags {
		if d.Code == "DECISION_CYCLE" {
			t.Errorf("a dropped self-reference should not be a cycle: %+v", d)
		}
	}
	dec, err := defs.Decision("UseN")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dec.Evaluate(context.Background(), dmn.Input{"N": 1}); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
}
