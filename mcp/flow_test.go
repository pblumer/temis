package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

func loadFlowModel(t *testing.T, s *Server, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	xml, _ := json.Marshal(string(b))
	id, _ := run(t, s, call(1, "load_model", `{"xml":`+string(xml)+`}`))[0].payload(t)["modelId"].(string)
	if id == "" {
		t.Fatalf("no modelId for %s", path)
	}
	return id
}

func flowDesc(riskID, loanID string) string {
	return fmt.Sprintf(`{"flow":"loan-decisioning",`+
		`"inputs":[{"name":"Credit Score","type":"number"},{"name":"Applicant Age","type":"number"}],`+
		`"steps":[`+
		`{"id":"risk","model":%q,"decision":"Risk Level","in":{"Credit Score":"Credit Score"}},`+
		`{"id":"decide","model":%q,"decision":"Loan Decision","in":{"Risk":"risk.Risk Level","Applicant Age":"Applicant Age"}}`+
		`],"output":{"Decision":"decide.Loan Decision"}}`, riskID, loanID)
}

func TestFlowToolsAdvertised(t *testing.T) {
	resps := run(t, newServer(), req(1, "tools/list", ""))
	var res struct {
		Tools []toolSpec `json:"tools"`
	}
	if err := json.Unmarshal(resps[0].Result, &res); err != nil {
		t.Fatalf("decode tools/list: %v", err)
	}
	got := map[string]bool{}
	for _, tl := range res.Tools {
		got[tl.Name] = true
	}
	for _, want := range []string{"load_flow", "describe_flow", "evaluate_flow"} {
		if !got[want] {
			t.Errorf("tools/list missing %q", want)
		}
	}
}

func TestFlowLoadDescribeEvaluate(t *testing.T) {
	s := newServer()
	riskID := loadFlowModel(t, s, "../flow/testdata/risk.dmn")
	loanID := loadFlowModel(t, s, "../flow/testdata/loan.dmn")

	// load_flow → flowId, name, no diagnostics.
	reg := run(t, s, call(1, "load_flow", `{"flow":`+flowDesc(riskID, loanID)+`}`))[0].payload(t)
	flowID, _ := reg["flowId"].(string)
	if flowID == "" || reg["name"] != "loan-decisioning" {
		t.Fatalf("unexpected load_flow result: %v", reg)
	}
	if _, ok := reg["diagnostics"]; ok {
		t.Fatalf("unexpected diagnostics: %v", reg["diagnostics"])
	}

	// describe_flow → name, two steps, two inputs.
	desc := run(t, s, call(2, "describe_flow", `{"flowId":"`+flowID+`"}`))[0].payload(t)
	if desc["name"] != "loan-decisioning" {
		t.Errorf("name = %v", desc["name"])
	}
	if steps, _ := desc["steps"].([]any); len(steps) != 2 {
		t.Errorf("steps = %v, want 2", desc["steps"])
	}
	if inputs, _ := desc["inputs"].([]any); len(inputs) != 2 {
		t.Errorf("inputs = %v, want 2", desc["inputs"])
	}

	// evaluate_flow by id.
	for _, tc := range []struct {
		score, age int
		want       string
	}{{750, 30, "approve"}, {550, 30, "decline"}, {650, 40, "review"}} {
		args := fmt.Sprintf(`{"flowId":"%s","input":{"Credit Score":%d,"Applicant Age":%d}}`, flowID, tc.score, tc.age)
		out := run(t, s, call(3, "evaluate_flow", args))[0].payload(t)
		outputs, _ := out["outputs"].(map[string]any)
		if outputs["Decision"] != tc.want {
			t.Errorf("score=%d: Decision = %v, want %q", tc.score, outputs["Decision"], tc.want)
		}
	}
}

func TestFlowEvaluateInlineAndExplain(t *testing.T) {
	s := newServer()
	riskID := loadFlowModel(t, s, "../flow/testdata/risk.dmn")
	loanID := loadFlowModel(t, s, "../flow/testdata/loan.dmn")

	// Inline descriptor, no prior registration.
	inline := fmt.Sprintf(`{"flow":%s,"input":{"Credit Score":550,"Applicant Age":30}}`, flowDesc(riskID, loanID))
	out := run(t, s, call(1, "evaluate_flow", inline))[0].payload(t)
	if outputs, _ := out["outputs"].(map[string]any); outputs["Decision"] != "decline" {
		t.Errorf("Decision = %v, want decline", out["outputs"])
	}

	// explain → aggregated trace of both decision steps.
	exp := fmt.Sprintf(`{"flow":%s,"input":{"Credit Score":750,"Applicant Age":30},"explain":true}`, flowDesc(riskID, loanID))
	out = run(t, s, call(2, "evaluate_flow", exp))[0].payload(t)
	trace, ok := out["trace"].(map[string]any)
	if !ok {
		t.Fatalf("trace missing: %v", out["trace"])
	}
	if tables, _ := trace["tables"].([]any); len(tables) != 2 {
		t.Errorf("tables = %v, want 2", trace["tables"])
	}
}

func TestFlowToolErrors(t *testing.T) {
	s := newServer()

	// Unknown flowId.
	if !run(t, s, call(1, "evaluate_flow", `{"flowId":"sha256:nope","input":{}}`))[0].call(t).IsError {
		t.Errorf("unknown flowId should be an isError result")
	}

	// A flow whose model is not loaded: load_flow surfaces the diagnostic,
	// evaluate_flow refuses with the structured problem.
	desc := `{"flow":"x","inputs":[{"name":"Credit Score","type":"number"}],"steps":[` +
		`{"id":"risk","model":"sha256:missing","decision":"Risk Level","in":{"Credit Score":"Credit Score"}}]}`
	reg := run(t, s, call(2, "load_flow", `{"flow":`+desc+`}`))[0].payload(t)
	flowID, _ := reg["flowId"].(string)
	if diags, _ := reg["diagnostics"].([]any); len(diags) == 0 {
		t.Fatalf("expected a diagnostic for the unresolved model")
	}
	cr := run(t, s, call(3, "evaluate_flow", `{"flowId":"`+flowID+`","input":{"Credit Score":700}}`))[0].call(t)
	if !cr.IsError || !strings.Contains(cr.Content[0].Text, "FLOW_MODEL_UNRESOLVED") {
		t.Errorf("expected FLOW_MODEL_UNRESOLVED isError, got %+v", cr)
	}
}
