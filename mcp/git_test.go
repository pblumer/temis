package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// gitServer returns an MCP Server whose git_* tools talk to gh (a fake GitHub).
func gitServer(t *testing.T, gh http.Handler) *Server {
	t.Helper()
	fake := httptest.NewServer(gh)
	t.Cleanup(fake.Close)
	return NewServer(dmn.New(), WithVersion("test"), WithGitHubBaseURL(fake.URL))
}

// callTool invokes one tool and returns the single response.
func callTool(t *testing.T, s *Server, id int, name string, args map[string]any) testResp {
	t.Helper()
	b, err := json.Marshal(map[string]any{"name": name, "arguments": args})
	if err != nil {
		t.Fatalf("marshal tool call: %v", err)
	}
	resps := run(t, s, req(id, "tools/call", string(b)))
	if len(resps) != 1 {
		t.Fatalf("want 1 response, got %d", len(resps))
	}
	return resps[0]
}

func dishContents(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Accept"), "raw") {
			_, _ = fmt.Fprint(w, dishXML(t))
			return
		}
		_, _ = fmt.Fprint(w, `{"name":"dish.dmn","path":"models/dish.dmn","sha":"blobABC","type":"file"}`)
	}
}

func TestGitToolsAdvertised(t *testing.T) {
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
	for _, want := range []string{"git_list_models", "git_load_model", "git_propose"} {
		if !got[want] {
			t.Errorf("tools/list missing %q", want)
		}
	}
}

func TestGitListModelsTool(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/pblumer/temis/contents/models", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer ghp_x" {
			t.Errorf("Authorization = %q, want bearer ghp_x", got)
		}
		_, _ = fmt.Fprint(w, `[{"name":"dish.dmn","path":"models/dish.dmn","sha":"b1","type":"file"},{"name":"x.md","path":"models/x.md","type":"file"}]`)
	})
	s := gitServer(t, mux)

	resp := callTool(t, s, 1, "git_list_models", map[string]any{
		"owner": "pblumer", "repo": "temis", "dir": "models", "gitToken": "ghp_x",
	})
	p := resp.payload(t)
	if p["count"].(float64) != 1 {
		t.Errorf("count = %v, want 1 (.dmn only): %v", p["count"], p)
	}
}

func TestGitLoadModelTool_thenEvaluate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/pblumer/temis/contents/models/dish.dmn", dishContents(t))
	s := gitServer(t, mux)

	load := callTool(t, s, 1, "git_load_model", map[string]any{
		"owner": "pblumer", "repo": "temis", "ref": "main", "path": "models/dish.dmn",
	})
	p := load.payload(t)
	id, _ := p["modelId"].(string)
	if !strings.HasPrefix(id, "sha256:") {
		t.Fatalf("modelId = %v", p["modelId"])
	}
	if p["repo"] != "pblumer/temis" || p["path"] != "models/dish.dmn" {
		t.Errorf("provenance = %v", p)
	}

	// The model is now cached and evaluable by id via the normal evaluate tool.
	ev := callTool(t, s, 2, "evaluate", map[string]any{
		"modelId": id, "decision": "Dish", "input": map[string]any{"Season": "Winter", "Guest Count": 8},
	})
	out := ev.payload(t)
	outputs, _ := out["outputs"].(map[string]any)
	if outputs["Dish"] != "Roastbeef" {
		t.Errorf("Dish = %v, want Roastbeef", outputs["Dish"])
	}
}

func TestGitLoadModelTool_notFoundIsToolError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/pblumer/temis/contents/models/missing.dmn", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"message":"Not Found"}`)
	})
	s := gitServer(t, mux)

	resp := callTool(t, s, 1, "git_load_model", map[string]any{
		"owner": "pblumer", "repo": "temis", "path": "models/missing.dmn",
	})
	if !resp.call(t).IsError {
		t.Errorf("expected isError for a missing file")
	}
}

func TestGitProposeTool(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/pblumer/temis/contents/models/dish.dmn", dishContents(t))
	mux.HandleFunc("/repos/pblumer/temis/commits/main", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"sha":"basesha"}`)
	})
	mux.HandleFunc("POST /repos/pblumer/temis/git/refs", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprint(w, `{}`)
	})
	mux.HandleFunc("PUT /repos/pblumer/temis/contents/models/dish.dmn", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"content":{"sha":"b2"},"commit":{"sha":"c2"}}`)
	})
	mux.HandleFunc("POST /repos/pblumer/temis/pulls", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprint(w, `{"number":7,"html_url":"https://github.com/pblumer/temis/pull/7","state":"open"}`)
	})
	s := gitServer(t, mux)

	resp := callTool(t, s, 1, "git_propose", map[string]any{
		"owner": "pblumer", "repo": "temis", "base": "main", "branch": "edit-dish",
		"path": "models/dish.dmn", "xml": dishXML(t), "title": "Update dish", "message": "tweak",
	})
	p := resp.payload(t)
	if p["number"].(float64) != 7 || p["base"] != "main" {
		t.Errorf("propose payload = %v", p)
	}
}

func TestGitProposeTool_malformedRejected(t *testing.T) {
	s := gitServer(t, http.NewServeMux())
	resp := callTool(t, s, 1, "git_propose", map[string]any{
		"owner": "pblumer", "repo": "temis", "base": "main", "branch": "b",
		"path": "x.dmn", "xml": "not xml", "title": "t",
	})
	if !resp.call(t).IsError {
		t.Errorf("expected isError for a malformed model")
	}
}
