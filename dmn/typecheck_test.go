package dmn_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// typeWarnings returns the type-check (TYPE_ERROR) diagnostics in diags.
func typeWarnings(diags dmn.Diagnostics) []dmn.Diagnostic {
	var out []dmn.Diagnostic
	for _, d := range diags {
		if d.Code == "TYPE_ERROR" {
			out = append(out, d)
		}
	}
	return out
}

// TestNoTypeWarningsOnValidModels guards against false positives: none of the
// real testdata models is ill-typed, so the checker must stay silent on them.
func TestNoTypeWarningsOnValidModels(t *testing.T) {
	dir := filepath.Join("testdata", "models")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".dmn" {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			_, diags, err := dmn.New().Compile(context.Background(), data)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if w := typeWarnings(diags); len(w) != 0 {
				t.Errorf("unexpected type warnings on a valid model: %+v", w)
			}
		})
	}
}

const mismatchModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/tc" name="TC" id="def_tc">
  <inputData id="i_score" name="Score"><variable name="Score" typeRef="number"/></inputData>
  <decision id="d_bad" name="Bad">
    <informationRequirement><requiredInput href="#i_score"/></informationRequirement>
    <literalExpression><text>Score + "oops"</text></literalExpression>
  </decision>
</definitions>`

// TestTypeMismatchWarning covers WP-30: a provable mismatch is a positioned
// warning, not a hard error — the decision still compiles and evaluates (to null
// per FEEL semantics).
func TestTypeMismatchWarning(t *testing.T) {
	defs, diags, err := dmn.New().Compile(context.Background(), []byte(mismatchModel))
	if err != nil {
		t.Fatal(err)
	}
	if diags.HasErrors() {
		t.Fatalf("type mismatch should be a warning, not an error: %+v", diags)
	}
	w := typeWarnings(diags)
	if len(w) != 1 {
		t.Fatalf("got %d type warnings, want 1: %+v", len(w), diags)
	}
	if w[0].Line == 0 || w[0].DecisionID != "d_bad" {
		t.Errorf("warning lacks position/decision: %+v", w[0])
	}

	// The decision is still executable; number + string yields null.
	dec, err := defs.Decision("Bad")
	if err != nil {
		t.Fatal(err)
	}
	res, err := dec.Evaluate(context.Background(), dmn.Input{"Score": 5})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outputs["Bad"] != nil {
		t.Errorf("Bad = %v, want nil (number + string)", res.Outputs["Bad"])
	}
}

const instanceOfModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/io" name="IO" id="def_io">
  <inputData id="i_v" name="Value"><variable name="Value"/></inputData>
  <decision id="d_isnum" name="IsNumber">
    <informationRequirement><requiredInput href="#i_v"/></informationRequirement>
    <literalExpression><text>Value instance of number</text></literalExpression>
  </decision>
</definitions>`

// TestInstanceOfEndToEnd covers WP-30's instance-of operator through the public API.
func TestInstanceOfEndToEnd(t *testing.T) {
	defs, diags, err := dmn.New().Compile(context.Background(), []byte(instanceOfModel))
	if err != nil || diags.HasErrors() {
		t.Fatalf("compile: err=%v diags=%+v", err, diags)
	}
	dec, _ := defs.Decision("IsNumber")
	if got := mustEval(t, dec, dmn.Input{"Value": 42}); got != true {
		t.Errorf("IsNumber(42) = %v, want true", got)
	}
	if got := mustEval(t, dec, dmn.Input{"Value": "x"}); got != false {
		t.Errorf("IsNumber(\"x\") = %v, want false", got)
	}
}

const itemDefModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/id" name="ID" id="def_id">
  <itemDefinition id="t_person" name="Person">
    <itemComponent id="c_age" name="age"><typeRef>number</typeRef></itemComponent>
    <itemComponent id="c_name" name="pname"><typeRef>string</typeRef></itemComponent>
  </itemDefinition>
  <inputData id="i_p" name="P"><variable name="P" typeRef="Person"/></inputData>
  <decision id="d_ok" name="OkUse">
    <informationRequirement><requiredInput href="#i_p"/></informationRequirement>
    <literalExpression><text>P.age + 1</text></literalExpression>
  </decision>
  <decision id="d_bad" name="BadUse">
    <informationRequirement><requiredInput href="#i_p"/></informationRequirement>
    <literalExpression><text>P.pname + 1</text></literalExpression>
  </decision>
</definitions>`

// TestItemDefinitionFieldTypes covers WP-30's use of item definitions: a struct
// field's declared type flows into the checker, so an arithmetic use of a string
// field is flagged while a number field is clean.
func TestItemDefinitionFieldTypes(t *testing.T) {
	_, diags, err := dmn.New().Compile(context.Background(), []byte(itemDefModel))
	if err != nil {
		t.Fatal(err)
	}
	w := typeWarnings(diags)
	if len(w) != 1 {
		t.Fatalf("got %d type warnings, want 1 (only BadUse): %+v", len(w), w)
	}
	if w[0].DecisionID != "d_bad" {
		t.Errorf("warning on %q, want d_bad", w[0].DecisionID)
	}
}

func mustEval(t *testing.T, dec *dmn.CompiledDecision, in dmn.Input) any {
	t.Helper()
	res, err := dec.Evaluate(context.Background(), in)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	return res.Outputs[dec.Name()]
}
