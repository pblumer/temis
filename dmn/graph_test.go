package dmn_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// TestDRGChaining covers WP-28: evaluating a decision automatically evaluates
// the decisions it requires and feeds their results in, so the caller supplies
// only the leaf input data.
func TestDRGChaining(t *testing.T) {
	defs := compileModel(t, "routing_13.dmn")
	dec, err := defs.Decision("Routing")
	if err != nil {
		t.Fatal(err)
	}

	// Only the input data is supplied; Eligibility is derived from it.
	res, err := dec.Evaluate(context.Background(), dmn.Input{"Applicant Age": 20})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outputs["Routing"] != "ACCEPT" {
		t.Errorf("Routing(age 20) = %v, want ACCEPT", res.Outputs["Routing"])
	}
	want := map[string]any{"Eligibility": "ELIGIBLE", "Routing": "ACCEPT"}
	if !reflect.DeepEqual(res.Decisions, want) {
		t.Errorf("Decisions = %#v, want %#v", res.Decisions, want)
	}

	// An under-age applicant chains through to a declined routing.
	res, _ = dec.Evaluate(context.Background(), dmn.Input{"Applicant Age": 16})
	if res.Outputs["Routing"] != "DECLINE" {
		t.Errorf("Routing(age 16) = %v, want DECLINE", res.Outputs["Routing"])
	}
}

// TestDRGSuppliedResultOverrides confirms a required decision supplied directly
// in the input is used as-is and is not recomputed (it stays absent from
// Decisions).
func TestDRGSuppliedResultOverrides(t *testing.T) {
	defs := compileModel(t, "routing_13.dmn")
	dec, _ := defs.Decision("Routing")

	res, err := dec.Evaluate(context.Background(), dmn.Input{"Eligibility": "ELIGIBLE"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outputs["Routing"] != "ACCEPT" {
		t.Errorf("Routing(Eligibility=ELIGIBLE) = %v, want ACCEPT", res.Outputs["Routing"])
	}
	if _, ok := res.Decisions["Eligibility"]; ok {
		t.Errorf("supplied Eligibility should not appear in Decisions: %#v", res.Decisions)
	}
}

const cyclicModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/cycle" name="Cycle" id="def_cycle">
  <decision id="a" name="A">
    <informationRequirement><requiredDecision href="#b"/></informationRequirement>
    <literalExpression><text>B</text></literalExpression>
  </decision>
  <decision id="b" name="B">
    <informationRequirement><requiredDecision href="#a"/></informationRequirement>
    <literalExpression><text>A</text></literalExpression>
  </decision>
</definitions>`

// TestDRGCycleDetected covers WP-28's cycle detection at compile time.
func TestDRGCycleDetected(t *testing.T) {
	_, diags, err := dmn.New().Compile(context.Background(), []byte(cyclicModel))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range diags {
		if d.Code == "DECISION_CYCLE" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a DECISION_CYCLE diagnostic, got %+v", diags)
	}
}

const diamondModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/diamond" name="Diamond" id="def_diamond">
  <inputData id="x" name="X"><variable name="X" typeRef="number"/></inputData>
  <decision id="dbl" name="Doubled">
    <informationRequirement><requiredInput href="#x"/></informationRequirement>
    <literalExpression><text>X * 2</text></literalExpression>
  </decision>
  <decision id="plus" name="Plus">
    <informationRequirement><requiredDecision href="#dbl"/></informationRequirement>
    <literalExpression><text>Doubled + 1</text></literalExpression>
  </decision>
  <decision id="times" name="Times">
    <informationRequirement><requiredDecision href="#dbl"/></informationRequirement>
    <literalExpression><text>Doubled * 10</text></literalExpression>
  </decision>
  <decision id="total" name="Total">
    <informationRequirement><requiredDecision href="#plus"/></informationRequirement>
    <informationRequirement><requiredDecision href="#times"/></informationRequirement>
    <literalExpression><text>Plus + Times</text></literalExpression>
  </decision>
</definitions>`

// TestDRGDiamond covers a shared sub-decision reached by two paths: it is
// evaluated once (memoised) and its result feeds both dependents.
func TestDRGDiamond(t *testing.T) {
	defs, diags, err := dmn.New().Compile(context.Background(), []byte(diamondModel))
	if err != nil {
		t.Fatal(err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	dec, _ := defs.Decision("Total")
	res, err := dec.Evaluate(context.Background(), dmn.Input{"X": 5})
	if err != nil {
		t.Fatal(err)
	}
	// Doubled=10, Plus=11, Times=100, Total=111.
	want := map[string]any{"Doubled": "10", "Plus": "11", "Times": "100", "Total": "111"}
	if !reflect.DeepEqual(res.Decisions, want) {
		t.Errorf("Decisions = %#v, want %#v", res.Decisions, want)
	}
}

// TestDRGCycleRuntimeError confirms evaluating a cyclic decision fails rather
// than looping forever.
func TestDRGCycleRuntimeError(t *testing.T) {
	defs, _, err := dmn.New().Compile(context.Background(), []byte(cyclicModel))
	if err != nil {
		t.Fatal(err)
	}
	dec, err := defs.Decision("A")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dec.Evaluate(context.Background(), nil); err == nil {
		t.Error("evaluating a cyclic decision should error")
	}
}
