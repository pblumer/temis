package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// --- toolListModels / List / WithStore ---

// TestToolListModels exercises list_models against the default in-process store,
// covering toolListModels and memStore.List with two cached models so the sort
// runs over a non-trivial slice.
func TestToolListModels(t *testing.T) {
	s := newServer()

	// Empty cache first: count is zero.
	empty := run(t, s, call(1, "list_models", `{}`))[0].payload(t)
	if empty["count"].(float64) != 0 {
		t.Errorf("empty cache count = %v, want 0", empty["count"])
	}

	xml, _ := json.Marshal(dishXML(t))
	run(t, s, call(2, "load_model", `{"xml":`+string(xml)+`}`))

	listed := run(t, s, call(3, "list_models", `{}`))[0].payload(t)
	if listed["count"].(float64) != 1 {
		t.Fatalf("count = %v, want 1", listed["count"])
	}
	models, _ := listed["models"].([]any)
	if len(models) != 1 {
		t.Fatalf("models = %v, want one entry", listed["models"])
	}
	first, _ := models[0].(map[string]any)
	if !contains(toStrings(first["decisions"]), "Dish") {
		t.Errorf("listed model decisions = %v, want Dish", first["decisions"])
	}
	// list_models surfaces the model's display name (the DMN definitions name).
	if first["name"] != "Dish" {
		t.Errorf("listed model name = %v, want Dish", first["name"])
	}
}

// TestToolGetModelXML covers get_model_xml: it reads a cached model's raw XML back
// (byte-identical to what was loaded, with its name), and errors for a missing or
// unknown modelId.
func TestToolGetModelXML(t *testing.T) {
	s := newServer()
	src := dishXML(t)
	xml, _ := json.Marshal(src)
	id, _ := run(t, s, call(1, "load_model", `{"xml":`+string(xml)+`}`))[0].payload(t)["modelId"].(string)
	if id == "" {
		t.Fatal("load_model returned no modelId")
	}

	got := run(t, s, call(2, "get_model_xml", `{"modelId":"`+id+`"}`))[0].payload(t)
	if got["xml"] != src {
		t.Errorf("get_model_xml returned XML that does not match the loaded source")
	}
	if got["name"] != "Dish" {
		t.Errorf("get_model_xml name = %v, want Dish", got["name"])
	}
	if got["modelId"] != id {
		t.Errorf("get_model_xml modelId = %v, want %v", got["modelId"], id)
	}

	// Missing modelId → error.
	if cr := run(t, s, call(3, "get_model_xml", `{}`))[0].call(t); !cr.IsError ||
		!strings.Contains(cr.Content[0].Text, "missing required argument") {
		t.Errorf("get_model_xml without modelId should error, got %+v", cr)
	}

	// Unknown modelId → error.
	if cr := run(t, s, call(4, "get_model_xml", `{"modelId":"sha256:deadbeef"}`))[0].call(t); !cr.IsError ||
		!strings.Contains(cr.Content[0].Text, "no model with id") {
		t.Errorf("get_model_xml with unknown id should error, got %+v", cr)
	}
}

// fakeStore is a minimal Store used to prove WithStore swaps the cache and that
// list_models reads through whatever Store the server holds.
type fakeStore struct {
	infos     []ModelInfo
	xml       map[string][]byte
	compileFn func() (string, *dmn.Definitions, dmn.ModelIndex, dmn.Diagnostics, error)
	lookupFn  func(id string) (*dmn.Definitions, dmn.ModelIndex, bool)
}

func (f *fakeStore) Compile(context.Context, []byte) (string, *dmn.Definitions, dmn.ModelIndex, dmn.Diagnostics, error) {
	if f.compileFn != nil {
		return f.compileFn()
	}
	return "", nil, dmn.ModelIndex{}, nil, errors.New("not implemented")
}

func (f *fakeStore) Lookup(id string) (*dmn.Definitions, dmn.ModelIndex, bool) {
	if f.lookupFn != nil {
		return f.lookupFn(id)
	}
	return nil, dmn.ModelIndex{}, false
}

func (f *fakeStore) List() []ModelInfo { return f.infos }

func (f *fakeStore) ModelXML(id string) ([]byte, bool) {
	xml, ok := f.xml[id]
	return xml, ok
}

// TestWithStore checks WithStore replaces the default store (and that a nil
// store is ignored, leaving the default in place).
func TestWithStore(t *testing.T) {
	fs := &fakeStore{infos: []ModelInfo{
		{ID: "sha256:bbb", Decisions: []string{"B"}, Inputs: []string{"y"}},
		{ID: "sha256:aaa", Decisions: []string{"A"}, Inputs: []string{"x"}},
	}}
	s := NewServer(nil, WithStore(fs))
	if s.store != fs {
		t.Fatalf("WithStore did not install the custom store")
	}

	listed := run(t, s, call(1, "list_models", `{}`))[0].payload(t)
	if listed["count"].(float64) != 2 {
		t.Fatalf("count = %v, want 2", listed["count"])
	}
	// Summaries are sorted by modelId ascending.
	models, _ := listed["models"].([]any)
	if id := models[0].(map[string]any)["modelId"]; id != "sha256:aaa" {
		t.Errorf("first model id = %v, want sha256:aaa (sorted)", id)
	}

	// A nil store is ignored: the default memStore stays.
	def := NewServer(dmn.New(), WithStore(nil))
	if _, ok := def.store.(*memStore); !ok {
		t.Errorf("WithStore(nil) replaced the default store: %T", def.store)
	}
}

// TestNewServerNilEngine covers the nil-engine branch of NewServer.
func TestNewServerNilEngine(t *testing.T) {
	s := NewServer(nil)
	if s.store == nil {
		t.Fatal("NewServer(nil) produced a server with no store")
	}
	// It is usable: evaluate by xml compiles on the fly.
	xml, _ := json.Marshal(dishXML(t))
	out := run(t, s, call(1, "evaluate",
		`{"xml":`+string(xml)+`,"decision":"Dish","input":{"Season":"Winter","Guest Count":8}}`))[0].payload(t)
	if o, _ := out["outputs"].(map[string]any); o["Dish"] != "Roastbeef" {
		t.Errorf("Dish = %v, want Roastbeef", out["outputs"])
	}
}

// --- diagnostics DTO conversion ---

// routingXML returns a model that compiles successfully but carries a warning
// diagnostic (an unknown element is ignored), so toDiagnosticDTOs maps a
// non-empty slice.
func routingXML(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../dmn/testdata/models/routing_13.dmn")
	if err != nil {
		t.Fatalf("read routing model: %v", err)
	}
	return string(b)
}

// TestLoadModelEmitsDiagnostics drives load_model with a model that compiles
// with a warning, exercising toDiagnosticDTOs over a populated slice.
func TestLoadModelEmitsDiagnostics(t *testing.T) {
	xml, _ := json.Marshal(routingXML(t))
	p := run(t, newServer(), call(1, "load_model", `{"xml":`+string(xml)+`}`))[0].payload(t)
	diags, ok := p["diagnostics"].([]any)
	if !ok || len(diags) == 0 {
		t.Fatalf("expected non-empty diagnostics, got %v", p["diagnostics"])
	}
	first, _ := diags[0].(map[string]any)
	if first["severity"] != "warning" {
		t.Errorf("diagnostic severity = %v, want warning", first["severity"])
	}
	if first["message"] == "" {
		t.Errorf("diagnostic message is empty: %v", first)
	}
}

// --- toolText marshal-error path ---

// TestToolTextMarshalError feeds toolText a value that cannot be JSON-encoded,
// covering its error branch (a channel is unmarshalable).
func TestToolTextMarshalError(t *testing.T) {
	_, rerr := toolText(map[string]any{"bad": make(chan int)})
	if rerr == nil || rerr.Code != codeInternalError {
		t.Fatalf("want internal error from toolText, got %+v", rerr)
	}
}

// --- handleToolsCall invalid params ---

// TestToolsCallInvalidParams sends a tools/call whose params are not an object,
// covering the unmarshal-error branch of handleToolsCall.
func TestToolsCallInvalidParams(t *testing.T) {
	resps := run(t, newServer(), req(1, "tools/call", `"not-an-object"`))
	if resps[0].Error == nil || resps[0].Error.Code != codeInvalidParams {
		t.Fatalf("want invalid-params error, got %+v", resps[0])
	}
}

// --- per-tool invalid-arguments and missing-argument branches ---

// TestToolInvalidArguments covers the "invalid arguments" JSON-unmarshal branch
// of every tool by passing arguments of the wrong JSON shape.
func TestToolInvalidArguments(t *testing.T) {
	tools := []string{
		"load_model", "get_model_xml", "describe_decision", "evaluate",
		"git_list_models", "git_load_model", "git_propose",
	}
	for _, name := range tools {
		t.Run(name, func(t *testing.T) {
			// arguments is a string, not the object each tool expects → unmarshal fails.
			cr := run(t, newServer(), call(1, name, `"oops"`))[0].call(t)
			if !cr.IsError {
				t.Errorf("%s with bad arguments should be isError, got %+v", name, cr)
			}
			if !strings.Contains(cr.Content[0].Text, "invalid arguments") {
				t.Errorf("%s error text = %q, want 'invalid arguments'", name, cr.Content[0].Text)
			}
		})
	}
}

// TestDescribeDecisionMissingArgs covers describe_decision's missing-modelId and
// missing-decision branches.
func TestDescribeDecisionMissingArgs(t *testing.T) {
	// Missing modelId.
	cr := run(t, newServer(), call(1, "describe_decision", `{"decision":"Dish"}`))[0].call(t)
	if !cr.IsError || !strings.Contains(cr.Content[0].Text, "modelId") {
		t.Errorf("missing modelId: %+v", cr)
	}

	// Present modelId but missing decision: load first, then omit decision.
	s := newServer()
	xml, _ := json.Marshal(dishXML(t))
	id, _ := run(t, s, call(1, "load_model", `{"xml":`+string(xml)+`}`))[0].payload(t)["modelId"].(string)
	cr2 := run(t, s, call(2, "describe_decision", `{"modelId":"`+id+`"}`))[0].call(t)
	if !cr2.IsError || !strings.Contains(cr2.Content[0].Text, "decision") {
		t.Errorf("missing decision: %+v", cr2)
	}
}

// TestDescribeDecisionUnknownDecision covers the defs.Decision error branch:
// a real, cached model but a decision name it does not declare.
func TestDescribeDecisionUnknownDecision(t *testing.T) {
	s := newServer()
	xml, _ := json.Marshal(dishXML(t))
	id, _ := run(t, s, call(1, "load_model", `{"xml":`+string(xml)+`}`))[0].payload(t)["modelId"].(string)
	cr := run(t, s, call(2, "describe_decision", `{"modelId":"`+id+`","decision":"Nope"}`))[0].call(t)
	if !cr.IsError {
		t.Errorf("unknown decision should be isError, got %+v", cr)
	}
}

// TestDescribeDecisionReachableInputs covers the additive reachableInputs field:
// Routing needs "Applicant Age" only transitively (through Eligibility), so it is
// absent from the direct "inputs" schema but present in "reachableInputs" — what
// a flow step targeting Routing may wire (ADR-0026).
func TestDescribeDecisionReachableInputs(t *testing.T) {
	s := newServer()
	xml, _ := json.Marshal(routingXML(t))
	id, _ := run(t, s, call(1, "load_model", `{"xml":`+string(xml)+`}`))[0].payload(t)["modelId"].(string)

	desc := run(t, s, call(2, "describe_decision", `{"modelId":"`+id+`","decision":"Routing"}`))[0].payload(t)

	names := func(key string) map[string]bool {
		out := map[string]bool{}
		raw, _ := desc[key].([]any)
		for _, r := range raw {
			f, _ := r.(map[string]any)
			if n, ok := f["name"].(string); ok {
				out[n] = true
			}
		}
		return out
	}
	// Direct inputs: Routing declares none (it names only Eligibility).
	if got := names("inputs"); got["Applicant Age"] {
		t.Errorf("direct inputs should not include the transitive Applicant Age: %v", got)
	}
	// Reachable inputs: the leaf input reached through Eligibility.
	if got := names("reachableInputs"); !got["Applicant Age"] {
		t.Errorf("reachableInputs should include Applicant Age, got %v", got)
	}
}

// TestEvaluateUnknownDecision covers toolEvaluate's defs.Decision error branch
// (model resolves, decision does not).
func TestEvaluateUnknownDecision(t *testing.T) {
	xml, _ := json.Marshal(dishXML(t))
	cr := run(t, newServer(), call(1, "evaluate",
		`{"xml":`+string(xml)+`,"decision":"Nope"}`))[0].call(t)
	if !cr.IsError {
		t.Errorf("unknown decision should be isError, got %+v", cr)
	}
}

// TestEvaluateBadXML covers toolEvaluate's compile-error branch (xml source that
// does not compile).
func TestEvaluateBadXML(t *testing.T) {
	cr := run(t, newServer(), call(1, "evaluate",
		`{"xml":"<not-dmn/>","decision":"Dish"}`))[0].call(t)
	if !cr.IsError {
		t.Errorf("malformed xml should be isError, got %+v", cr)
	}
}

// TestEvaluateRuntimeError covers toolEvaluate's generic (non-InputError)
// evaluation-failure branch: a required input is missing, so Evaluate returns an
// *EvalError (not an *InputError) which is reported as a plain tool error.
func TestEvaluateRuntimeError(t *testing.T) {
	xml, _ := json.Marshal(dishXML(t))
	cr := run(t, newServer(), call(1, "evaluate",
		`{"xml":`+string(xml)+`,"decision":"Dish","input":{}}`))[0].call(t)
	if !cr.IsError || !strings.Contains(cr.Content[0].Text, "evaluation failed") {
		t.Errorf("missing required input should yield 'evaluation failed', got %+v", cr)
	}
}

// --- git tool missing-argument branches ---

func TestGitToolMissingArgs(t *testing.T) {
	s := gitServer(t, http.NewServeMux())
	cases := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list missing owner/repo", "git_list_models", map[string]any{"owner": "o"}},
		{"load missing owner/repo", "git_load_model", map[string]any{"repo": "r", "path": "p"}},
		{"load missing path", "git_load_model", map[string]any{"owner": "o", "repo": "r"}},
		{"propose missing owner/repo", "git_propose", map[string]any{"base": "main"}},
		{"propose missing base/branch/path/xml", "git_propose", map[string]any{"owner": "o", "repo": "r"}},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := callTool(t, s, i+1, tc.tool, tc.args)
			if !resp.call(t).IsError {
				t.Errorf("%s should be isError", tc.name)
			}
		})
	}
}

// TestGitListModelsError covers toolGitListModels' git-error branch (the remote
// returns 404).
func TestGitListModelsError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/o/r/contents/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	})
	s := gitServer(t, mux)
	resp := callTool(t, s, 1, "git_list_models", map[string]any{"owner": "o", "repo": "r"})
	if !resp.call(t).IsError {
		t.Errorf("git list error should be isError")
	}
}

// TestGitLoadModelCompileError covers toolGitLoadModel's compile-error branch:
// the file is fetched fine but is not a valid DMN document.
func TestGitLoadModelCompileError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/o/r/contents/bad.dmn", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Accept"), "raw") {
			_, _ = w.Write([]byte("<not-dmn/>"))
			return
		}
		_, _ = w.Write([]byte(`{"name":"bad.dmn","path":"bad.dmn","sha":"s","type":"file"}`))
	})
	s := gitServer(t, mux)
	resp := callTool(t, s, 1, "git_load_model", map[string]any{"owner": "o", "repo": "r", "path": "bad.dmn"})
	cr := resp.call(t)
	if !cr.IsError || !strings.Contains(cr.Content[0].Text, "could not compile") {
		t.Errorf("compile error expected, got %+v", cr)
	}
}

// TestGitProposeError covers toolGitPropose's git-error branch: a well-formed
// model but the remote rejects branch creation.
func TestGitProposeError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/o/r/commits/main", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	})
	s := gitServer(t, mux)
	resp := callTool(t, s, 1, "git_propose", map[string]any{
		"owner": "o", "repo": "r", "base": "main", "branch": "b",
		"path": "x.dmn", "xml": dishXML(t), "title": "t",
	})
	cr := resp.call(t)
	if !cr.IsError || !strings.Contains(cr.Content[0].Text, "git:") {
		t.Errorf("git propose error expected, got %+v", cr)
	}
}

// --- Serve transport branches ---

// errWriter fails every write, exercising Serve's encoder-error return.
type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestServeEncoderError(t *testing.T) {
	in := strings.NewReader(req(1, "ping", "") + "\n")
	err := newServer().Serve(context.Background(), in, errWriter{})
	if err == nil {
		t.Fatal("Serve should propagate the encoder write error")
	}
}

// TestServeContextCancelled covers Serve's ctx.Err() early return: a cancelled
// context with pending input stops before dispatching.
func TestServeContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	in := strings.NewReader(req(1, "ping", "") + "\n" + req(2, "ping", "") + "\n")
	var out strings.Builder
	err := newServer().Serve(ctx, in, &out)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("cancelled Serve wrote output: %q", out.String())
	}
}

// TestServeSkipsBlankLines covers the blank-line continue branch in Serve.
func TestServeSkipsBlankLines(t *testing.T) {
	in := strings.NewReader("\n  \n" + req(1, "ping", "") + "\n")
	var out strings.Builder
	if err := newServer().Serve(context.Background(), in, &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if !strings.Contains(out.String(), `"id":1`) {
		t.Errorf("expected ping response after blank lines, got %q", out.String())
	}
}

// --- HTTP body read error ---

// errReadCloser fails on Read, exercising handleHTTPMessage's read-error branch.
type errReadCloser struct{}

func (errReadCloser) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func (errReadCloser) Close() error             { return nil }

func TestHTTPBodyReadError(t *testing.T) {
	h := newServer().HTTPHandler()
	req := httptest.NewRequest("POST", "/mcp", nil)
	req.Body = errReadCloser{}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("body read error status = %d, want 400", rec.Code)
	}
}
