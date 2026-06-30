package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// fakeAnthropic is a minimal Anthropic Messages API stand-in driven by a script
// of raw response bodies, one per call. It records the X-Api-Key it saw so tests
// can assert on the bring-your-own-key path.
type fakeAnthropic struct {
	mu      sync.Mutex
	calls   int
	lastKey string
	script  []string
	status  int // non-zero overrides 200 for every call
}

func (f *fakeAnthropic) server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.lastKey = r.Header.Get("X-Api-Key")
		_, _ = io.ReadAll(r.Body)
		if f.status != 0 {
			w.WriteHeader(f.status)
			_, _ = io.WriteString(w, `{"error":{"type":"x","message":"bad key"}}`)
			return
		}
		i := f.calls
		f.calls++
		if i >= len(f.script) {
			_, _ = io.WriteString(w, `{"content":[{"type":"text","text":"(no script)"}],"stop_reason":"end_turn"}`)
			return
		}
		_, _ = io.WriteString(w, f.script[i])
	}))
	t.Cleanup(srv.Close)
	return srv
}

func toolUse(id, name, input string) string {
	return `{"content":[{"type":"tool_use","id":"` + id + `","name":"` + name + `","input":` + input + `}],"stop_reason":"tool_use"}`
}

func finalText(text string) string {
	return `{"content":[{"type":"text","text":"` + text + `"}],"stop_reason":"end_turn"}`
}

func TestChatDisabled(t *testing.T) {
	h := NewServer(nil).Handler()
	rec := do(t, h, "POST", "/v1/chat", "application/json", []byte(`{"messages":[{"role":"user","text":"hi"}]}`))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 (body %s)", rec.Code, rec.Body)
	}
}

func TestChatLoopRunsTools(t *testing.T) {
	fake := &fakeAnthropic{script: []string{
		toolUse("t1", "list_models", `{}`),
		finalText("Es gibt Modelle."),
	}}
	srv := fake.server(t)

	h := NewServer(nil,
		WithExamples(),
		WithAssist(AssistConfig{Provider: "anthropic", Token: "sk-server", BaseURL: srv.URL}),
	).Handler()

	rec := do(t, h, "POST", "/v1/chat", "application/json",
		[]byte(`{"messages":[{"role":"user","text":"Welche Modelle gibt es?"}]}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
	resp := decode[chatResponse](t, rec)
	if resp.Reply != "Es gibt Modelle." {
		t.Errorf("reply = %q", resp.Reply)
	}
	if resp.Provider != "anthropic" {
		t.Errorf("provider = %q", resp.Provider)
	}
	if len(resp.Steps) != 1 || resp.Steps[0].Tool != "list_models" || resp.Steps[0].Error {
		t.Fatalf("steps = %+v, want one successful list_models", resp.Steps)
	}
	// The tool result must be real JSON listing the example models.
	var listed struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal([]byte(resp.Steps[0].Result), &listed); err != nil {
		t.Fatalf("tool result not JSON: %v (%s)", err, resp.Steps[0].Result)
	}
	if listed.Count == 0 {
		t.Errorf("expected example models to be listed, got count 0")
	}
	if fake.calls != 2 {
		t.Errorf("provider calls = %d, want 2", fake.calls)
	}
}

func TestChatNoToken(t *testing.T) {
	srv := (&fakeAnthropic{}).server(t)
	h := NewServer(nil, WithAssist(AssistConfig{Provider: "anthropic", AllowBYOK: true, BaseURL: srv.URL})).Handler()
	rec := do(t, h, "POST", "/v1/chat", "application/json", []byte(`{"messages":[{"role":"user","text":"hi"}]}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body)
	}
	if got := decode[problem](t, rec); got.Code != "ASSIST_NO_TOKEN" {
		t.Errorf("code = %q, want ASSIST_NO_TOKEN", got.Code)
	}
}

func TestChatBYOKHeaderWins(t *testing.T) {
	fake := &fakeAnthropic{script: []string{finalText("hi")}}
	srv := fake.server(t)
	h := NewServer(nil, WithAssist(AssistConfig{Provider: "anthropic", Token: "sk-server", AllowBYOK: true, BaseURL: srv.URL})).Handler()

	req := newJSONRequest("POST", "/v1/chat", []byte(`{"messages":[{"role":"user","text":"hi"}]}`))
	req.Header.Set("X-LLM-Token", "sk-byok")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
	if fake.lastKey != "sk-byok" {
		t.Errorf("provider saw key %q, want sk-byok (BYOK must override server token)", fake.lastKey)
	}
}

func TestChatProviderError(t *testing.T) {
	fake := &fakeAnthropic{status: http.StatusUnauthorized}
	srv := fake.server(t)
	h := NewServer(nil, WithAssist(AssistConfig{Provider: "anthropic", Token: "sk", BaseURL: srv.URL})).Handler()
	rec := do(t, h, "POST", "/v1/chat", "application/json", []byte(`{"messages":[{"role":"user","text":"hi"}]}`))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 (body %s)", rec.Code, rec.Body)
	}
	if got := decode[problem](t, rec); got.Code != "ASSIST_PROVIDER_ERROR" {
		t.Errorf("code = %q, want ASSIST_PROVIDER_ERROR", got.Code)
	}
}

// TestAssistExecutorTools drives the tool surface directly (no LLM) over a server
// with the dish example loaded, covering both the read and the build tools.
func TestAssistExecutorTools(t *testing.T) {
	s := NewServer(nil)
	sm, err := s.compileAndStore(context.Background(), dishXML(t))
	if err != nil {
		t.Fatalf("load dish: %v", err)
	}
	e := newAssistExecutor(s)
	ctx := context.Background()
	mustJSON := func(name, args string) map[string]any {
		t.Helper()
		out, err := e.Execute(ctx, name, json.RawMessage(args))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(out), &m); err != nil {
			t.Fatalf("%s result not JSON: %v (%s)", name, err, out)
		}
		return m
	}

	// Tool catalog is non-empty and well-formed.
	if len(e.Tools()) < 5 {
		t.Errorf("expected several tools, got %d", len(e.Tools()))
	}

	mid := sm.id
	mustJSON("list_models", `{}`)
	desc := mustJSON("describe_decision", `{"modelId":"`+mid+`","decision":"Dish"}`)
	if desc["inputs"] == nil {
		t.Errorf("describe_decision missing inputs: %v", desc)
	}
	mustJSON("get_decision_table", `{"modelId":"`+mid+`","decision":"Dish"}`)

	ev := mustJSON("evaluate", `{"modelId":"`+mid+`","decision":"Dish","input":{"Season":"Winter","Guest Count":8}}`)
	outputs, _ := ev["outputs"].(map[string]any)
	if outputs["Dish"] != "Roastbeef" {
		t.Errorf("evaluate Dish = %v, want Roastbeef", outputs["Dish"])
	}

	// Build tool: load a model from XML; lastModel must be set for the UI.
	load := mustJSON("load_model", mustQuoteXML(t, dishXML(t)))
	if load["modelId"] == "" || e.lastModel == "" {
		t.Errorf("load_model did not report/record a modelId: %v", load)
	}

	// Build tool: rewrite the Dish table's rules (rules-only edit keeps columns),
	// then verify the new model still evaluates.
	save := mustJSON("save_decision_table", `{"modelId":"`+mid+`","decision":"Dish","rules":[{"inputEntries":["-","-"],"outputEntries":["\"Pizza\""]}]}`)
	newID, _ := save["modelId"].(string)
	if newID == "" || newID == mid {
		t.Fatalf("save_decision_table should return a new modelId, got %q", newID)
	}
	ev2 := mustJSON("evaluate", `{"modelId":"`+newID+`","decision":"Dish","input":{"Season":"Winter","Guest Count":8}}`)
	out2, _ := ev2["outputs"].(map[string]any)
	if out2["Dish"] != "Pizza" {
		t.Errorf("after save, Dish = %v, want Pizza", out2["Dish"])
	}

	// A bad model id is reported as a tool error (fed back to the model), not a panic.
	if _, err := e.Execute(ctx, "describe_decision", json.RawMessage(`{"modelId":"sha256:nope","decision":"Dish"}`)); err == nil {
		t.Errorf("expected error for unknown model")
	}
	if _, err := e.Execute(ctx, "nope", json.RawMessage(`{}`)); err == nil {
		t.Errorf("expected error for unknown tool")
	}
}

// mustQuoteXML renders {"xml": "<escaped document>"} for the load_model tool.
func mustQuoteXML(t *testing.T, xml []byte) string {
	t.Helper()
	q, err := json.Marshal(string(xml))
	if err != nil {
		t.Fatal(err)
	}
	return `{"xml":` + string(q) + `}`
}

func newJSONRequest(method, path string, body []byte) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}
