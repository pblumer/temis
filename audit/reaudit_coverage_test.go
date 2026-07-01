package audit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// discountModel is a literal-expression model that compiles but errors at
// evaluation when a required input is omitted (mirrors dmn's evalerror tests).
// It is used to drive the EvalError-on-Evaluate branch of verify.
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

// TestReAuditNilEngineDefaults exercises the eng == nil branch of ReAudit: a nil
// engine must be replaced with a fresh dmn.New() so the run still works.
func TestReAuditNilEngineDefaults(t *testing.T) {
	xml := dishXML(t)
	id := ModelID(xml)
	models := MapModelSource{id: xml}
	in := map[string]any{"Season": "Winter", "Guest Count": 8}
	line := eventLine(t, DecisionEventType, "n1", id, "Dish", "/d", in, map[string]any{"Dish": "Roastbeef"})

	rep, err := ReAudit(context.Background(), nil, strings.NewReader(line), models)
	if err != nil {
		t.Fatalf("ReAudit with nil engine: %v", err)
	}
	if !rep.Reproduced() || rep.Total != 1 || rep.OK != 1 {
		t.Errorf("nil-engine run: Reproduced=%v Total=%d OK=%d, want true/1/1", rep.Reproduced(), rep.Total, rep.OK)
	}
}

// TestVerifyCompileError drives the compile-failure branch of verify: the model
// source resolves the id to bytes that are not valid DMN, so eng.Compile fails.
func TestVerifyCompileError(t *testing.T) {
	bad := []byte("<definitions>not valid dmn")
	id := ModelID(bad)
	models := MapModelSource{id: bad}
	in := map[string]any{"x": 1}
	line := eventLine(t, DecisionEventType, "c1", id, "Dish", "/d", in, map[string]any{})

	rep, err := ReAudit(context.Background(), dmn.New(), strings.NewReader(line), models)
	if err != nil {
		t.Fatalf("ReAudit: %v", err)
	}
	if rep.EvalErrors != 1 || len(rep.Outcomes) != 1 {
		t.Fatalf("EvalErrors=%d outcomes=%d, want 1/1", rep.EvalErrors, len(rep.Outcomes))
	}
	o := rep.Outcomes[0]
	if o.Status != EvalError {
		t.Errorf("status = %q, want eval_error", o.Status)
	}
	if !strings.HasPrefix(o.Detail, "compile:") {
		t.Errorf("detail = %q, want compile: prefix", o.Detail)
	}
}

// TestVerifyEvaluateError drives the Evaluate-failure branch of verify: the model
// compiles and the decision exists, but a required input is omitted so Evaluate
// returns an error.
func TestVerifyEvaluateError(t *testing.T) {
	xml := []byte(discountModel)
	id := ModelID(xml)
	models := MapModelSource{id: xml}
	// Member omitted -> required input missing -> Evaluate errors.
	in := map[string]any{"Amount": 200}
	line := eventLine(t, DecisionEventType, "ev1", id, "Discount", "/d", in, map[string]any{"Discount": 20})

	rep, err := ReAudit(context.Background(), dmn.New(), strings.NewReader(line), models)
	if err != nil {
		t.Fatalf("ReAudit: %v", err)
	}
	if rep.EvalErrors != 1 || len(rep.Outcomes) != 1 {
		t.Fatalf("EvalErrors=%d outcomes=%d, want 1/1", rep.EvalErrors, len(rep.Outcomes))
	}
	o := rep.Outcomes[0]
	if o.Status != EvalError {
		t.Errorf("status = %q, want eval_error", o.Status)
	}
	if !strings.HasPrefix(o.Detail, "evaluate:") {
		t.Errorf("detail = %q, want evaluate: prefix", o.Detail)
	}
}

// TestVerifyReusesCompiledModel covers the compiled-cache hit path: two events
// for the same model compile only once and both verify against the cached defs.
func TestVerifyReusesCompiledModel(t *testing.T) {
	xml := dishXML(t)
	id := ModelID(xml)
	models := MapModelSource{id: xml}
	in := map[string]any{"Season": "Winter", "Guest Count": 8}
	lines := []string{
		eventLine(t, DecisionEventType, "r1", id, "Dish", "/d/1", in, map[string]any{"Dish": "Roastbeef"}),
		eventLine(t, DecisionEventType, "r2", id, "Dish", "/d/2", in, map[string]any{"Dish": "Roastbeef"}),
	}
	rep, err := ReAudit(context.Background(), dmn.New(), strings.NewReader(strings.Join(lines, "\n")), models)
	if err != nil {
		t.Fatalf("ReAudit: %v", err)
	}
	if rep.Total != 2 || rep.OK != 2 {
		t.Errorf("Total/OK = %d/%d, want 2/2", rep.Total, rep.OK)
	}
}

// TestOutputsEqualEncodeErrors covers both un-encodable branches of outputsEqual
// by passing maps holding a channel, which encoding/json cannot marshal.
func TestOutputsEqualEncodeErrors(t *testing.T) {
	bad := map[string]any{"x": make(chan int)}
	good := map[string]any{"x": 1}

	if same, detail := outputsEqual(bad, good); same || !strings.Contains(detail, "recorded outputs not encodable") {
		t.Errorf("recorded-unencodable: same=%v detail=%q", same, detail)
	}
	if same, detail := outputsEqual(good, bad); same || !strings.Contains(detail, "re-evaluated outputs not encodable") {
		t.Errorf("got-unencodable: same=%v detail=%q", same, detail)
	}
}

// TestNewDirModelSourceReadDirError covers the ReadDir-failure branch: a path
// that does not exist cannot be read.
func TestNewDirModelSourceReadDirError(t *testing.T) {
	_, err := NewDirModelSource(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("NewDirModelSource on missing dir = nil error, want error")
	}
	if !strings.Contains(err.Error(), "read model dir") {
		t.Errorf("error = %v, want read model dir wrap", err)
	}
}

// TestNewDirModelSourceSkipsSubdir covers the IsDir-skip branch: a subdirectory
// (even one named with a model extension) must be ignored.
func TestNewDirModelSourceSkipsSubdir(t *testing.T) {
	dir := t.TempDir()
	xml := dishXML(t)
	if err := os.WriteFile(filepath.Join(dir, "dish.dmn"), xml, 0o644); err != nil {
		t.Fatal(err)
	}
	// A subdirectory with a .dmn name must be skipped by the IsDir guard.
	if err := os.Mkdir(filepath.Join(dir, "nested.dmn"), 0o755); err != nil {
		t.Fatal(err)
	}
	src, err := NewDirModelSource(dir)
	if err != nil {
		t.Fatalf("NewDirModelSource: %v", err)
	}
	if src.Len() != 1 {
		t.Errorf("indexed %d models, want 1 (subdir skipped)", src.Len())
	}
}

// TestNewDirModelSourceReadFileError covers the ReadFile-failure branch: a
// dangling symlink with a .dmn extension is a non-dir entry whose contents
// cannot be read.
func TestNewDirModelSourceReadFileError(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "broken.dmn")
	if err := os.Symlink(filepath.Join(dir, "missing-target"), link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	_, err := NewDirModelSource(dir)
	if err == nil {
		t.Fatal("NewDirModelSource over dangling symlink = nil error, want error")
	}
	if !strings.Contains(err.Error(), "broken.dmn") {
		t.Errorf("error = %v, want broken.dmn read failure", err)
	}
}

// TestSortOutcomes covers SortOutcomes: ordering by status, then decision, then
// subject, with stability across the three comparison branches.
func TestSortOutcomes(t *testing.T) {
	in := []Outcome{
		{Status: EvalError, Decision: "B", Subject: "s2"},
		{Status: Discrepancy, Decision: "B", Subject: "s1"},
		{Status: Discrepancy, Decision: "A", Subject: "s9"},
		{Status: Discrepancy, Decision: "B", Subject: "s0"},
		{Status: EvalError, Decision: "A", Subject: "s1"},
	}
	SortOutcomes(in)

	want := []Outcome{
		{Status: Discrepancy, Decision: "A", Subject: "s9"}, // status first
		{Status: Discrepancy, Decision: "B", Subject: "s0"}, // same status, decision, subject tiebreak
		{Status: Discrepancy, Decision: "B", Subject: "s1"},
		{Status: EvalError, Decision: "A", Subject: "s1"},
		{Status: EvalError, Decision: "B", Subject: "s2"},
	}
	for i := range want {
		if in[i] != want[i] {
			t.Errorf("position %d = %+v, want %+v", i, in[i], want[i])
		}
	}
}
