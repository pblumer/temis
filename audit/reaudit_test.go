package audit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pblumer/temis/dmn"
)

const dishModelPath = "../dmn/testdata/models/dish_15.dmn"

func dishXML(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(dishModelPath)
	if err != nil {
		t.Fatalf("read dish model: %v", err)
	}
	return b
}

// eventLine renders one NDJSON decision event for the audit stream.
func eventLine(t *testing.T, typ, id, modelID, decision, subject string, input, outputs map[string]any) string {
	t.Helper()
	m := map[string]any{
		"id":      id,
		"type":    typ,
		"subject": subject,
		"data": map[string]any{
			"modelId":  modelID,
			"decision": decision,
			"input":    input,
			"outputs":  outputs,
		},
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	return string(b)
}

func TestReAuditClassifiesEachEvent(t *testing.T) {
	xml := dishXML(t)
	id := ModelID(xml)
	models := MapModelSource{id: xml}
	in := map[string]any{"Season": "Winter", "Guest Count": 8}

	lines := []string{
		// reproduces exactly
		eventLine(t, DecisionEventType, "e1", id, "Dish", "/decisions/Dish", in, map[string]any{"Dish": "Roastbeef"}),
		// recorded output was tampered with -> discrepancy
		eventLine(t, DecisionEventType, "e2", id, "Dish", "/decisions/Dish", in, map[string]any{"Dish": "Spareribs"}),
		// model not in the source -> inconclusive
		eventLine(t, DecisionEventType, "e3", "sha256:deadbeef", "Dish", "/decisions/Dish", in, map[string]any{"Dish": "Roastbeef"}),
		// unknown decision -> eval error
		eventLine(t, DecisionEventType, "e4", id, "Nope", "/decisions/Nope", in, map[string]any{}),
		// not a decision event -> ignored entirely
		eventLine(t, "com.example.other.v1", "e5", id, "Dish", "/x", in, map[string]any{"Dish": "Roastbeef"}),
	}
	stream := strings.NewReader(strings.Join(lines, "\n"))

	rep, err := ReAudit(context.Background(), dmn.New(), stream, models)
	if err != nil {
		t.Fatalf("ReAudit: %v", err)
	}
	if rep.Total != 4 {
		t.Errorf("Total = %d, want 4 (the non-decision event is ignored)", rep.Total)
	}
	if rep.OK != 1 {
		t.Errorf("OK = %d, want 1", rep.OK)
	}
	if rep.Discrepancies != 1 {
		t.Errorf("Discrepancies = %d, want 1", rep.Discrepancies)
	}
	if rep.Unavailable != 1 {
		t.Errorf("Unavailable = %d, want 1", rep.Unavailable)
	}
	if rep.EvalErrors != 1 {
		t.Errorf("EvalErrors = %d, want 1", rep.EvalErrors)
	}
	if rep.Reproduced() {
		t.Error("Reproduced() = true, want false (there are discrepancies/errors)")
	}

	byID := map[string]Outcome{}
	for _, o := range rep.Outcomes {
		byID[o.EventID] = o
	}
	if got := byID["e2"].Status; got != Discrepancy {
		t.Errorf("e2 status = %q, want discrepancy", got)
	}
	if !strings.Contains(byID["e2"].Detail, "Spareribs") || !strings.Contains(byID["e2"].Detail, "Roastbeef") {
		t.Errorf("e2 detail = %q, want recorded/got both shown", byID["e2"].Detail)
	}
	if got := byID["e3"].Status; got != ModelUnavailable {
		t.Errorf("e3 status = %q, want model_unavailable", got)
	}
	if got := byID["e4"].Status; got != EvalError {
		t.Errorf("e4 status = %q, want eval_error", got)
	}
	if _, ok := byID["e1"]; ok {
		t.Error("reproduced event e1 should not be retained in Outcomes")
	}
}

func TestReAuditCleanHistoryReproduces(t *testing.T) {
	xml := dishXML(t)
	id := ModelID(xml)
	models := MapModelSource{id: xml}

	// Two genuine decisions, both recorded faithfully.
	lines := []string{
		eventLine(t, DecisionEventType, "a", id, "Dish", "/d/1",
			map[string]any{"Season": "Winter", "Guest Count": 8}, map[string]any{"Dish": "Roastbeef"}),
		eventLine(t, DecisionEventType, "b", id, "Dish", "/d/2",
			map[string]any{"Season": "Summer", "Guest Count": 3}, mustEval(t, xml, map[string]any{"Season": "Summer", "Guest Count": 3})),
	}
	rep, err := ReAudit(context.Background(), dmn.New(), strings.NewReader(strings.Join(lines, "\n")), models)
	if err != nil {
		t.Fatalf("ReAudit: %v", err)
	}
	if !rep.Reproduced() {
		t.Errorf("Reproduced() = false, want true; outcomes: %+v", rep.Outcomes)
	}
	if rep.Total != 2 || rep.OK != 2 {
		t.Errorf("Total/OK = %d/%d, want 2/2", rep.Total, rep.OK)
	}
}

func TestReAuditMalformedStreamErrors(t *testing.T) {
	_, err := ReAudit(context.Background(), dmn.New(), strings.NewReader("{not json"), MapModelSource{})
	if err == nil {
		t.Fatal("ReAudit on malformed stream = nil error, want error")
	}
}

func TestDirModelSourceMatchesServiceID(t *testing.T) {
	dir := t.TempDir()
	xml := dishXML(t)
	if err := os.WriteFile(filepath.Join(dir, "dish.dmn"), xml, 0o644); err != nil {
		t.Fatal(err)
	}
	// a non-model file must be ignored
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	src, err := NewDirModelSource(dir)
	if err != nil {
		t.Fatalf("NewDirModelSource: %v", err)
	}
	if src.Len() != 1 {
		t.Errorf("indexed %d models, want 1", src.Len())
	}
	if _, ok := src.Model(ModelID(xml)); !ok {
		t.Errorf("model %s not resolvable from dir source", ModelID(xml))
	}
	if _, ok := src.Model("sha256:nope"); ok {
		t.Error("unknown id resolved unexpectedly")
	}
}

// mustEval evaluates the dish model for input and returns the outputs as a
// JSON-round-tripped map, so the recorded form matches what the sink would have
// stored (the same shape ReAudit compares against).
func mustEval(t *testing.T, xml []byte, input map[string]any) map[string]any {
	t.Helper()
	defs, _, err := dmn.New().Compile(context.Background(), xml)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	dc, err := defs.Decision("Dish")
	if err != nil {
		t.Fatalf("decision: %v", err)
	}
	res, err := dc.Evaluate(context.Background(), dmn.Input(input))
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	b, _ := json.Marshal(res.Outputs)
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("round-trip outputs: %v", err)
	}
	return out
}
