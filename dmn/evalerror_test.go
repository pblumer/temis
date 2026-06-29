package dmn

import (
	"context"
	"errors"
	"testing"
)

// discountModel mirrors the literal model used in engine_test: a Discount
// decision requiring inputs Amount and Member.
const discountModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" id="d1" name="Discount" namespace="ex">
  <inputData id="id_amount" name="Amount"/>
  <inputData id="id_member" name="Member"/>
  <decision id="id_discount" name="Discount">
    <informationRequirement><requiredInput href="#id_amount"/></informationRequirement>
    <informationRequirement><requiredInput href="#id_member"/></informationRequirement>
    <literalExpression>
      <text>if Member then Amount * 0.1 else 0</text>
    </literalExpression>
  </decision>
</definitions>`

func TestEvaluateMissingRequiredInput(t *testing.T) {
	eng := New()
	defs, diags, err := eng.Compile(context.Background(), []byte(discountModel))
	if err != nil || diags.HasErrors() {
		t.Fatalf("compile: err=%v diags=%+v", err, diags)
	}
	dec, err := defs.Decision("Discount")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}

	// Member is omitted: a required input is missing → hard error.
	_, err = dec.Evaluate(context.Background(), Input{"Amount": 200})
	var ee *EvalError
	if !errors.As(err, &ee) {
		t.Fatalf("error = %v (%T), want *EvalError", err, err)
	}
	if ee.Code != CodeMissingInput {
		t.Errorf("Code = %q, want %q", ee.Code, CodeMissingInput)
	}
	if ee.DecisionID != "id_discount" {
		t.Errorf("DecisionID = %q, want id_discount", ee.DecisionID)
	}
}

func TestEvaluateNotExecutable(t *testing.T) {
	// A decision with nil expr is not executable. The public Decision() lookup
	// rejects these, so construct one directly to exercise Evaluate's guard.
	cd := &CompiledDecision{id: "id_bad", name: "Bad"}
	_, err := cd.Evaluate(context.Background(), Input{})
	var ee *EvalError
	if !errors.As(err, &ee) {
		t.Fatalf("error = %v (%T), want *EvalError", err, err)
	}
	if ee.Code != CodeNotExecutable {
		t.Errorf("Code = %q, want %q", ee.Code, CodeNotExecutable)
	}
}

func TestEvaluateContextCancelled(t *testing.T) {
	eng := New()
	defs, _, err := eng.Compile(context.Background(), []byte(discountModel))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	dec, err := defs.Decision("Discount")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = dec.Evaluate(ctx, Input{"Amount": 200, "Member": true})
	var ee *EvalError
	if !errors.As(err, &ee) {
		t.Fatalf("error = %v (%T), want *EvalError", err, err)
	}
	if ee.Code != CodeRuntime {
		t.Errorf("Code = %q, want %q", ee.Code, CodeRuntime)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("errors.Is(err, context.Canceled) = false; want wrapped cause")
	}
}

// uniqueModel is a single-input UNIQUE decision table with two overlapping
// rules, so any input below 20 matches both rules — a UNIQUE violation.
const uniqueModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" id="d1" name="U" namespace="ex">
  <inputData id="id_x" name="x"/>
  <decision id="id_pick" name="Pick">
    <informationRequirement><requiredInput href="#id_x"/></informationRequirement>
    <decisionTable hitPolicy="UNIQUE">
      <input id="i1"><inputExpression><text>x</text></inputExpression></input>
      <output id="o1" name="out"/>
      <rule id="r1"><inputEntry><text>&lt; 10</text></inputEntry><outputEntry><text>"a"</text></outputEntry></rule>
      <rule id="r2"><inputEntry><text>&lt; 20</text></inputEntry><outputEntry><text>"b"</text></outputEntry></rule>
    </decisionTable>
  </decision>
</definitions>`

func TestEvaluateUniqueMultipleMatch(t *testing.T) {
	eng := New()
	defs, diags, err := eng.Compile(context.Background(), []byte(uniqueModel))
	if err != nil || diags.HasErrors() {
		t.Fatalf("compile: err=%v diags=%+v", err, diags)
	}
	dec, err := defs.Decision("Pick")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}

	_, err = dec.Evaluate(context.Background(), Input{"x": 5})
	var ee *EvalError
	if !errors.As(err, &ee) {
		t.Fatalf("error = %v (%T), want *EvalError", err, err)
	}
	if ee.Code != CodeUniqueMultiple {
		t.Errorf("Code = %q, want %q", ee.Code, CodeUniqueMultiple)
	}
}

// TestCompileDiagnosticCodes checks that the public Compile path surfaces the
// stable error-class codes (not the old MODEL_<severity> form) for the
// diagnostics internal/model emits.
func TestCompileDiagnosticCodes(t *testing.T) {
	// Unknown namespace + a decision with no logic.
	const m = `<?xml version="1.0"?>
<definitions xmlns="urn:bogus" id="d" name="X" namespace="ex">
  <decision id="id_nologic" name="NoLogic"/>
</definitions>`
	eng := New()
	_, diags, err := eng.Compile(context.Background(), []byte(m))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	want := map[string]bool{CodeUnknownNamespace: false, CodeNoLogic: false}
	for _, d := range diags {
		if _, ok := want[d.Code]; ok {
			want[d.Code] = true
		}
		if len(d.Code) >= 6 && d.Code[:6] == "MODEL_" {
			t.Errorf("diagnostic still uses legacy MODEL_ code: %q", d.Code)
		}
	}
	for code, seen := range want {
		if !seen {
			t.Errorf("missing diagnostic with code %q in %+v", code, diags)
		}
	}
}
