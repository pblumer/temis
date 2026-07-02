package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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

func TestFlowCatalogAndDetail(t *testing.T) {
	h := newTestServer(t)
	riskID := uploadModel(t, h, "../flow/testdata/risk.dmn")
	loanID := uploadModel(t, h, "../flow/testdata/loan.dmn")
	reg := do(t, h, "POST", "/v1/flows", "application/json", loanFlowDescriptor(riskID, loanID))
	if reg.Code != http.StatusCreated {
		t.Fatalf("register = %d: %s", reg.Code, reg.Body)
	}
	fr := decode[flowResponse](t, reg)

	// Catalog.
	rec := do(t, h, "GET", "/v1/flows", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d: %s", rec.Code, rec.Body)
	}
	list := decode[flowListResponse](t, rec)
	if list.Count != 1 || len(list.Flows) != 1 {
		t.Fatalf("catalog = %+v, want 1 flow", list)
	}
	if fs := list.Flows[0]; fs.FlowID != fr.FlowID || fs.Name != "loan-decisioning" || fs.Steps != 2 || len(fs.Inputs) != 2 {
		t.Errorf("summary = %+v", fs)
	}

	// Detail (drawing data).
	rec = do(t, h, "GET", "/v1/flows/"+fr.FlowID, "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("detail = %d: %s", rec.Code, rec.Body)
	}
	d := decode[flowDetail](t, rec)
	if d.Name != "loan-decisioning" || len(d.Steps) != 2 || len(d.Inputs) != 2 {
		t.Fatalf("detail = %+v", d)
	}
	if d.Steps[0].ID == "" || d.Steps[0].Decision == "" || d.Steps[0].Model == "" {
		t.Errorf("step missing fields: %+v", d.Steps[0])
	}
	if d.Output["Decision"] != "decide.Loan Decision" {
		t.Errorf("output = %v", d.Output)
	}
	if len(d.Diagnostics) != 0 {
		t.Errorf("models loaded → expected no diagnostics, got %v", d.Diagnostics)
	}

	// Unknown flow.
	if rec := do(t, h, "GET", "/v1/flows/sha256:nope", "", nil); rec.Code != http.StatusNotFound {
		t.Errorf("unknown flow detail = %d, want 404", rec.Code)
	}
}

// TestFlowStoreLoadsFromDir verifies WithFlowStore loads *.flow.json descriptors
// into the catalog at startup (ADR-0032): valid flows are registered, a malformed
// descriptor is skipped rather than blocking startup, and non-flow files are
// ignored. It also confirms the load validates against models that are present.
func TestFlowStoreLoadsFromDir(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, b []byte) {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	// A structurally valid flow (unknown model ids → registers with diagnostics),
	// a malformed descriptor (skipped), and a non-flow file (ignored).
	write("loan.flow.json", loanFlowDescriptor("sha256:aaa", "sha256:bbb"))
	write("broken.flow.json", []byte("{ not json"))
	write("notes.txt", []byte("ignore me"))

	h := NewServer(nil, WithFlowStore(dir)).Handler()

	rec := do(t, h, "GET", "/v1/flows", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d: %s", rec.Code, rec.Body)
	}
	list := decode[flowListResponse](t, rec)
	if list.Count != 1 || len(list.Flows) != 1 {
		t.Fatalf("catalog = %+v, want exactly 1 loaded flow", list)
	}
	if fs := list.Flows[0]; fs.Name != "loan-decisioning" || fs.Steps != 2 {
		t.Errorf("summary = %+v", fs)
	}
	// The loaded flow is retrievable by its content id.
	if rec := do(t, h, "GET", "/v1/flows/"+list.Flows[0].FlowID, "", nil); rec.Code != http.StatusOK {
		t.Errorf("detail = %d: %s", rec.Code, rec.Body)
	}

	// A missing directory disables the store without blocking startup.
	h2 := NewServer(nil, WithFlowStore(filepath.Join(dir, "does-not-exist"))).Handler()
	if rec := do(t, h2, "GET", "/v1/flows", "", nil); rec.Code != http.StatusOK || decode[flowListResponse](t, rec).Count != 0 {
		t.Errorf("missing dir should yield empty catalog, got %d: %s", rec.Code, rec.Body)
	}
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
