package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
)

func uploadModel(t *testing.T, h http.Handler, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	rec := do(t, h, "POST", "/v1/models", "application/xml", b)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload %s = %d: %s", path, rec.Code, rec.Body)
	}
	return decode[modelResponse](t, rec).ModelID
}

func loanFlowDescriptor(riskID, loanID string) []byte {
	return []byte(fmt.Sprintf(`{
      "flow": "loan-decisioning",
      "inputs": [{"name":"Credit Score","type":"number"},{"name":"Applicant Age","type":"number"}],
      "steps": [
        {"id":"risk","model":%q,"decision":"Risk Level","in":{"Credit Score":"Credit Score"}},
        {"id":"decide","model":%q,"decision":"Loan Decision","in":{"Risk":"risk.Risk Level","Applicant Age":"Applicant Age"}}
      ],
      "output": {"Decision":"decide.Loan Decision"}
    }`, riskID, loanID))
}

func TestFlowRegisterAndEvaluate(t *testing.T) {
	h := newTestServer(t)
	riskID := uploadModel(t, h, "../flow/testdata/risk.dmn")
	loanID := uploadModel(t, h, "../flow/testdata/loan.dmn")

	rec := do(t, h, "POST", "/v1/flows", "application/json", loanFlowDescriptor(riskID, loanID))
	if rec.Code != http.StatusCreated {
		t.Fatalf("register flow = %d: %s", rec.Code, rec.Body)
	}
	fr := decode[flowResponse](t, rec)
	if fr.FlowID == "" || fr.Name != "loan-decisioning" {
		t.Fatalf("unexpected flow response: %+v", fr)
	}
	if len(fr.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", fr.Diagnostics)
	}

	// Evaluate by id: several scenarios end-to-end.
	for _, tc := range []struct {
		score, age int
		want       string
	}{
		{750, 30, "approve"},
		{550, 30, "decline"},
		{650, 40, "review"},
	} {
		body := []byte(fmt.Sprintf(`{"input":{"Credit Score":%d,"Applicant Age":%d}}`, tc.score, tc.age))
		rec = do(t, h, "POST", "/v1/flows/"+fr.FlowID+"/evaluate", "application/json", body)
		if rec.Code != http.StatusOK {
			t.Fatalf("evaluate (score=%d) = %d: %s", tc.score, rec.Code, rec.Body)
		}
		er := decode[evaluateResponse](t, rec)
		if er.Outputs["Decision"] != tc.want {
			t.Fatalf("score=%d: Decision = %v, want %q", tc.score, er.Outputs["Decision"], tc.want)
		}
	}
}

func TestFlowEvaluateExplain(t *testing.T) {
	h := newTestServer(t)
	riskID := uploadModel(t, h, "../flow/testdata/risk.dmn")
	loanID := uploadModel(t, h, "../flow/testdata/loan.dmn")
	rec := do(t, h, "POST", "/v1/flows", "application/json", loanFlowDescriptor(riskID, loanID))
	fr := decode[flowResponse](t, rec)

	body := []byte(`{"input":{"Credit Score":750,"Applicant Age":30},"explain":true}`)
	rec = do(t, h, "POST", "/v1/flows/"+fr.FlowID+"/evaluate", "application/json", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("evaluate = %d: %s", rec.Code, rec.Body)
	}
	er := decode[evaluateResponse](t, rec)
	if er.Trace == nil || len(er.Trace.Tables) != 2 {
		t.Fatalf("expected 2 table traces, got %+v", er.Trace)
	}
}

func TestFlowStatelessInline(t *testing.T) {
	h := newTestServer(t)
	riskID := uploadModel(t, h, "../flow/testdata/risk.dmn")
	loanID := uploadModel(t, h, "../flow/testdata/loan.dmn")

	inline := []byte(fmt.Sprintf(`{"flow": %s, "input": {"Credit Score":550,"Applicant Age":30}}`,
		loanFlowDescriptor(riskID, loanID)))
	rec := do(t, h, "POST", "/v1/flow/evaluate", "application/json", inline)
	if rec.Code != http.StatusOK {
		t.Fatalf("inline evaluate = %d: %s", rec.Code, rec.Body)
	}
	er := decode[evaluateResponse](t, rec)
	if er.Outputs["Decision"] != "decline" {
		t.Fatalf("Decision = %v, want decline", er.Outputs["Decision"])
	}
}

func TestFlowErrors(t *testing.T) {
	h := newTestServer(t)

	// Unknown flow id.
	rec := do(t, h, "POST", "/v1/flows/sha256:nope/evaluate", "application/json", []byte(`{"input":{}}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown flow = %d, want 404", rec.Code)
	}

	// Malformed descriptor.
	rec = do(t, h, "POST", "/v1/flows", "application/json", []byte(`{not json`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed flow = %d, want 400", rec.Code)
	}
	if p := decode[problem](t, rec); p.Code != "FLOW_MALFORMED" {
		t.Fatalf("code = %q, want FLOW_MALFORMED", p.Code)
	}

	// A flow whose model is not loaded: registration surfaces the diagnostic and
	// evaluation refuses with a structured FLOW_INVALID problem.
	desc := []byte(`{"flow":"x","inputs":[{"name":"Credit Score","type":"number"}],"steps":[
        {"id":"risk","model":"sha256:missing","decision":"Risk Level","in":{"Credit Score":"Credit Score"}}]}`)
	rec = do(t, h, "POST", "/v1/flows", "application/json", desc)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register (unresolved model) = %d, want 201: %s", rec.Code, rec.Body)
	}
	fr := decode[flowResponse](t, rec)
	if !hasFlowCode(fr.Diagnostics, "FLOW_MODEL_UNRESOLVED") {
		t.Fatalf("expected FLOW_MODEL_UNRESOLVED, got %v", fr.Diagnostics)
	}
	rec = do(t, h, "POST", "/v1/flows/"+fr.FlowID+"/evaluate", "application/json", []byte(`{"input":{"Credit Score":700}}`))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("evaluate (unresolved) = %d, want 422: %s", rec.Code, rec.Body)
	}
	p := decode[problem](t, rec)
	if p.Code != "FLOW_INVALID" || !hasFlowCode(p.FlowProblems, "FLOW_MODEL_UNRESOLVED") {
		t.Fatalf("unexpected problem: %+v", p)
	}
}

func TestFlowClioAudit(t *testing.T) {
	clio := &captureClio{}
	h := auditServer(t, clio, nil)

	riskID := uploadModel(t, h, "../flow/testdata/risk.dmn")
	loanID := uploadModel(t, h, "../flow/testdata/loan.dmn")
	inline := []byte(fmt.Sprintf(`{"flow": %s, "input": {"Credit Score":750,"Applicant Age":30}}`,
		loanFlowDescriptor(riskID, loanID)))
	rec := do(t, h, "POST", "/v1/flow/evaluate", "application/json", inline)
	if rec.Code != http.StatusOK {
		t.Fatalf("evaluate = %d: %s", rec.Code, rec.Body)
	}

	raws := clio.rawBodies()
	if len(raws) != 1 {
		t.Fatalf("clio writes = %d, want 1", len(raws))
	}
	var req clioFlowWriteRequest
	if err := json.Unmarshal(raws[0], &req); err != nil {
		t.Fatalf("decode flow write: %v", err)
	}
	if len(req.Events) != 1 {
		t.Fatalf("events = %d, want 1", len(req.Events))
	}
	ev := req.Events[0]
	if ev.Type != FlowEventType {
		t.Errorf("type = %q, want %q", ev.Type, FlowEventType)
	}
	if ev.Data.Flow != "loan-decisioning" {
		t.Errorf("flow = %q", ev.Data.Flow)
	}
	if ev.Data.Outputs["Decision"] != "approve" {
		t.Errorf("outputs[Decision] = %v, want approve", ev.Data.Outputs["Decision"])
	}
	if len(ev.Data.Models) != 2 {
		t.Errorf("models = %v, want 2", ev.Data.Models)
	}
	if len(ev.Data.Descriptor) == 0 {
		t.Error("descriptor is empty; re-audit could not replay")
	}
	if ev.Data.InputHash == "" || ev.Data.FlowID == "" {
		t.Error("inputHash/flowId missing")
	}
}

func hasFlowCode(diags []flowDiagnosticDTO, code string) bool {
	for _, d := range diags {
		if d.Code == code {
			return true
		}
	}
	return false
}
