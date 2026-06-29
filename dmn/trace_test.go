package dmn_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pblumer/temis/dmn"
)

func compileDecision(t *testing.T, file, decision string) *dmn.CompiledDecision {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "models", file))
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	defs, diags, err := dmn.New().Compile(context.Background(), data)
	if err != nil {
		t.Fatalf("compile %s: %v", file, err)
	}
	if diags.HasErrors() {
		t.Fatalf("compile %s: %+v", file, diags)
	}
	dec, err := defs.Decision(decision)
	if err != nil {
		t.Fatalf("decision %q: %v", decision, err)
	}
	return dec
}

func TestEvaluateNoTraceByDefault(t *testing.T) {
	dec := compileDecision(t, "dish_15.dmn", "Dish")
	res, err := dec.Evaluate(context.Background(), dmn.Input{"Season": "Winter", "Guest Count": 8})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.Trace != nil {
		t.Errorf("Trace should be nil without WithTrace, got %+v", res.Trace)
	}
}

func TestEvaluateTraceUnique(t *testing.T) {
	dec := compileDecision(t, "dish_15.dmn", "Dish")
	res, err := dec.Evaluate(context.Background(),
		dmn.Input{"Season": "Winter", "Guest Count": 8}, dmn.WithTrace())
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.Outputs["Dish"] != "Roastbeef" {
		t.Fatalf("Dish = %v, want Roastbeef", res.Outputs["Dish"])
	}
	if res.Trace == nil || len(res.Trace.Tables) != 1 {
		t.Fatalf("want exactly one traced table, got %+v", res.Trace)
	}
	tbl := res.Trace.Tables[0]

	if tbl.HitPolicy != "U" || tbl.Aggregation != "" {
		t.Errorf("hit policy = %q agg = %q, want U/empty", tbl.HitPolicy, tbl.Aggregation)
	}
	if len(tbl.Inputs) != 2 ||
		tbl.Inputs[0].Expression != "Season" || tbl.Inputs[0].Value != "Winter" ||
		tbl.Inputs[1].Expression != "Guest Count" || tbl.Inputs[1].Value != "8" {
		t.Errorf("inputs = %+v, want Season=Winter, Guest Count=8", tbl.Inputs)
	}
	if len(tbl.Matched) != 1 || tbl.Matched[0] != 1 {
		t.Errorf("matched = %v, want [1] (rule r2)", tbl.Matched)
	}
	if len(tbl.Rules) != 4 {
		t.Fatalf("want 4 rule traces, got %d", len(tbl.Rules))
	}

	// Exactly one rule matched: r2 (Winter, <=8) → Roastbeef, both conditions true.
	var matchedCount int
	for _, r := range tbl.Rules {
		if !r.Matched {
			// A non-matching rule must show the condition that ruled it out as the
			// last one recorded (short-circuit).
			last := r.Conditions[len(r.Conditions)-1]
			if last.Matched {
				t.Errorf("rule %d not matched but last condition matched: %+v", r.Index, r.Conditions)
			}
			if len(r.Outputs) != 0 {
				t.Errorf("non-matching rule %d should have no outputs, got %v", r.Index, r.Outputs)
			}
			continue
		}
		matchedCount++
		if r.ID != "r2" {
			t.Errorf("matched rule id = %q, want r2", r.ID)
		}
		if len(r.Conditions) != 2 || !r.Conditions[0].Matched || !r.Conditions[1].Matched {
			t.Errorf("matched rule conditions = %+v, want two satisfied", r.Conditions)
		}
		if len(r.Outputs) != 1 || r.Outputs[0] != "Roastbeef" {
			t.Errorf("matched rule outputs = %v, want [Roastbeef]", r.Outputs)
		}
	}
	if matchedCount != 1 {
		t.Errorf("want exactly one matched rule trace, got %d", matchedCount)
	}
}

func TestEvaluateTraceCollectSum(t *testing.T) {
	dec := compileDecision(t, "risk_15.dmn", "Risk Score")
	res, err := dec.Evaluate(context.Background(),
		dmn.Input{"Has Debt": true, "Is New Customer": true}, dmn.WithTrace())
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	// All three rules fire; SUM(30,20,5) = 55.
	if res.Outputs["Risk Score"] != "55" {
		t.Fatalf("Risk Score = %v, want 55", res.Outputs["Risk Score"])
	}
	if res.Trace == nil || len(res.Trace.Tables) != 1 {
		t.Fatalf("want one traced table, got %+v", res.Trace)
	}
	tbl := res.Trace.Tables[0]
	if tbl.HitPolicy != "C" || tbl.Aggregation != "SUM" {
		t.Errorf("hit policy = %q agg = %q, want C/SUM", tbl.HitPolicy, tbl.Aggregation)
	}
	if len(tbl.Matched) != 3 {
		t.Errorf("matched = %v, want all three rules", tbl.Matched)
	}
	// Every matched rule records its own contributed output.
	want := map[string]string{"r1": "30", "r2": "20", "r3": "5"}
	for _, r := range tbl.Rules {
		if !r.Matched {
			t.Errorf("rule %s should have matched", r.ID)
			continue
		}
		if len(r.Outputs) != 1 || r.Outputs[0] != want[r.ID] {
			t.Errorf("rule %s outputs = %v, want [%s]", r.ID, r.Outputs, want[r.ID])
		}
	}
}

func TestTraceOnLiteralHasNoTables(t *testing.T) {
	// A literal-expression decision is traceable but produces no table entries.
	const model = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" id="d1" name="Lit" namespace="ex">
  <inputData id="a" name="A"/>
  <decision id="dx" name="Double">
    <informationRequirement><requiredInput href="#a"/></informationRequirement>
    <literalExpression><text>A * 2</text></literalExpression>
  </decision>
</definitions>`
	defs, diags, err := dmn.New().Compile(context.Background(), []byte(model))
	if err != nil || diags.HasErrors() {
		t.Fatalf("compile: %v %+v", err, diags)
	}
	dec, _ := defs.Decision("Double")
	res, err := dec.Evaluate(context.Background(), dmn.Input{"A": 21}, dmn.WithTrace())
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.Outputs["Double"] != "42" {
		t.Errorf("Double = %v, want 42", res.Outputs["Double"])
	}
	if res.Trace == nil {
		t.Fatalf("Trace requested, should be non-nil")
	}
	if len(res.Trace.Tables) != 0 {
		t.Errorf("literal decision should trace no tables, got %+v", res.Trace.Tables)
	}
}
