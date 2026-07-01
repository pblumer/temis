package service

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
)

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

// rawContents serves a file for the GitHub contents API: raw bytes for a raw
// Accept (ReadFile), a JSON metadata object otherwise (ListFiles / blob-sha).
func rawContents(content []byte, name, path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Accept"), "raw") {
			_, _ = w.Write(content)
			return
		}
		_, _ = fmt.Fprintf(w, `{"name":%q,"path":%q,"sha":"blob-%s","size":%d,"type":"file"}`, name, path, name, len(content))
	}
}

func TestGitFlows_listAndLoad(t *testing.T) {
	riskXML := mustRead(t, "../flow/testdata/risk.dmn")
	loanXML := mustRead(t, "../flow/testdata/loan.dmn")
	riskID, loanID := modelID(riskXML), modelID(loanXML)
	desc := loanFlowDescriptor(riskID, loanID)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/pblumer/temis/contents/models/risk.dmn", rawContents(riskXML, "risk.dmn", "models/risk.dmn"))
	mux.HandleFunc("GET /repos/pblumer/temis/contents/models/loan.dmn", rawContents(loanXML, "loan.dmn", "models/loan.dmn"))
	mux.HandleFunc("GET /repos/pblumer/temis/contents/flows/loan.flow.json", rawContents(desc, "loan.flow.json", "flows/loan.flow.json"))
	mux.HandleFunc("GET /repos/pblumer/temis/contents/flows", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `[
			{"name":"loan.flow.json","path":"flows/loan.flow.json","sha":"f1","type":"file"},
			{"name":"notes.md","path":"flows/notes.md","type":"file"}
		]`)
	})
	h := gitTestServer(t, mux)

	// List: only the *.flow.json descriptor.
	rec := doGit(t, h, "GET", "/v1/git/flows?owner=pblumer&repo=temis&dir=flows", "tok", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list flows = %d (body %s)", rec.Code, rec.Body)
	}
	flows := decode[gitFlowsResponse](t, rec)
	if flows.Count != 1 || flows.Flows[0].Path != "flows/loan.flow.json" {
		t.Errorf("flows = %+v, want only loan.flow.json", flows)
	}

	// Load the referenced models first, so the flow validates cleanly.
	for _, p := range []string{"models/risk.dmn", "models/loan.dmn"} {
		if r := doGit(t, h, "POST", "/v1/git/load", "tok",
			fmt.Sprintf(`{"owner":"pblumer","repo":"temis","ref":"main","path":%q}`, p)); r.Code != http.StatusOK {
			t.Fatalf("load %s = %d (body %s)", p, r.Code, r.Body)
		}
	}

	// Load the flow from git → flowId, no diagnostics.
	rec = doGit(t, h, "POST", "/v1/git/load-flow", "tok",
		`{"owner":"pblumer","repo":"temis","ref":"main","path":"flows/loan.flow.json"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("load-flow = %d (body %s)", rec.Code, rec.Body)
	}
	fr := decode[gitLoadFlowResponse](t, rec)
	if fr.FlowID == "" || fr.Name != "loan-decisioning" {
		t.Fatalf("unexpected load-flow response: %+v", fr)
	}
	if len(fr.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", fr.Diagnostics)
	}
	if fr.Repo != "pblumer/temis" || fr.Path != "flows/loan.flow.json" {
		t.Errorf("provenance = %+v", fr)
	}

	// The git-loaded flow evaluates by id through the normal /v1/flows route.
	ev := doGit(t, h, "POST", "/v1/flows/"+fr.FlowID+"/evaluate", "",
		`{"input":{"Credit Score":750,"Applicant Age":30}}`)
	if ev.Code != http.StatusOK {
		t.Fatalf("evaluate = %d (body %s)", ev.Code, ev.Body)
	}
	if out := decode[evaluateResponse](t, ev); out.Outputs["Decision"] != "approve" {
		t.Errorf("Decision = %v, want approve", out.Outputs["Decision"])
	}
}

func TestGitLoadFlow_malformedIs400(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/pblumer/temis/contents/flows/bad.flow.json",
		rawContents([]byte(`{not json`), "bad.flow.json", "flows/bad.flow.json"))
	h := gitTestServer(t, mux)

	rec := doGit(t, h, "POST", "/v1/git/load-flow", "tok",
		`{"owner":"pblumer","repo":"temis","ref":"main","path":"flows/bad.flow.json"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed load-flow = %d, want 400", rec.Code)
	}
	if p := decode[problem](t, rec); p.Code != "FLOW_MALFORMED" {
		t.Errorf("code = %q, want FLOW_MALFORMED", p.Code)
	}
}
