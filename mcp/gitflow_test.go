package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
)

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

// rawContents serves a file for the GitHub contents API: raw bytes for a raw
// Accept, a JSON metadata object otherwise.
func rawContents(content []byte, name, path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Accept"), "raw") {
			_, _ = w.Write(content)
			return
		}
		_, _ = fmt.Fprintf(w, `{"name":%q,"path":%q,"sha":"blob","type":"file"}`, name, path)
	}
}

func TestGitFlowToolsAdvertised(t *testing.T) {
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
	for _, want := range []string{"git_list_flows", "git_load_flow"} {
		if !got[want] {
			t.Errorf("tools/list missing %q", want)
		}
	}
}

func TestGitLoadFlowTool(t *testing.T) {
	risk := mustReadFile(t, "../flow/testdata/risk.dmn")
	loan := mustReadFile(t, "../flow/testdata/loan.dmn")
	riskID, loanID := modelID(risk), modelID(loan)
	desc := flowDesc(riskID, loanID)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/pblumer/temis/contents/flows/loan.flow.json",
		rawContents([]byte(desc), "loan.flow.json", "flows/loan.flow.json"))
	mux.HandleFunc("GET /repos/pblumer/temis/contents/flows", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `[{"name":"loan.flow.json","path":"flows/loan.flow.json","type":"file"},{"name":"x.md","path":"flows/x.md","type":"file"}]`)
	})
	s := gitServer(t, mux)

	// Load the referenced models into the shared store.
	loadFlowModel(t, s, "../flow/testdata/risk.dmn")
	loadFlowModel(t, s, "../flow/testdata/loan.dmn")

	// git_list_flows: only the *.flow.json descriptor.
	lst := callTool(t, s, 1, "git_list_flows", map[string]any{"owner": "pblumer", "repo": "temis", "dir": "flows"}).payload(t)
	if flows, _ := lst["flows"].([]any); len(flows) != 1 {
		t.Errorf("flows = %v, want 1", lst["flows"])
	}

	// git_load_flow → flowId, no diagnostics (models are loaded).
	ld := callTool(t, s, 2, "git_load_flow", map[string]any{
		"owner": "pblumer", "repo": "temis", "ref": "main", "path": "flows/loan.flow.json",
	}).payload(t)
	flowID, _ := ld["flowId"].(string)
	if flowID == "" || ld["flow"] != "loan-decisioning" {
		t.Fatalf("git_load_flow = %v", ld)
	}
	if diags, _ := ld["diagnostics"].([]any); len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", ld["diagnostics"])
	}

	// The git-loaded flow evaluates by id.
	out := callTool(t, s, 3, "evaluate_flow", map[string]any{
		"flowId": flowID, "input": map[string]any{"Credit Score": 750, "Applicant Age": 30},
	}).payload(t)
	if outputs, _ := out["outputs"].(map[string]any); outputs["Decision"] != "approve" {
		t.Errorf("Decision = %v, want approve", out["outputs"])
	}
}
