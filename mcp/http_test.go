package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// postMCP sends one JSON-RPC message to the /mcp endpoint and returns the
// recorder.
func postMCP(t *testing.T, h http.Handler, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHTTPInitialize(t *testing.T) {
	h := newServer().HTTPHandler()
	rec := postMCP(t, h, req(1, "initialize", `{"protocolVersion":"2025-06-18"}`), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content-type = %q, want application/json", ct)
	}
	var resp testResp
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var res struct {
		ServerInfo struct{ Name string } `json:"serverInfo"`
	}
	if err := json.Unmarshal(resp.Result, &res); err != nil || res.ServerInfo.Name != serverName {
		t.Errorf("initialize result = %s (err %v)", resp.Result, err)
	}
}

func TestHTTPEvaluate(t *testing.T) {
	h := newServer().HTTPHandler()
	xml, _ := json.Marshal(dishXML(t))
	body := call(2, "evaluate",
		`{"xml":`+string(xml)+`,"decision":"Dish","input":{"Season":"Winter","Guest Count":8}}`)
	rec := postMCP(t, h, body, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
	var resp testResp
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	var cr callResult
	_ = json.Unmarshal(resp.Result, &cr)
	var payload map[string]any
	if err := json.Unmarshal([]byte(cr.Content[0].Text), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if out, _ := payload["outputs"].(map[string]any); out["Dish"] != "Roastbeef" {
		t.Errorf("Dish = %v, want Roastbeef", payload["outputs"])
	}
}

func TestHTTPNotificationIs202(t *testing.T) {
	h := newServer().HTTPHandler()
	rec := postMCP(t, h, `{"jsonrpc":"2.0","method":"notifications/initialized"}`, "")
	if rec.Code != http.StatusAccepted {
		t.Errorf("notification status = %d, want 202", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("notification should have empty body, got %q", rec.Body)
	}
}

func TestHTTPGetIsNotAllowed(t *testing.T) {
	h := newServer().HTTPHandler()
	req := httptest.NewRequest("GET", "/mcp", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /mcp = %d, want 405", rec.Code)
	}
}

func TestHTTPHealth(t *testing.T) {
	h := newServer().HTTPHandler()
	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("healthz = %d, want 200", rec.Code)
	}
}

func TestHTTPTokenAuth(t *testing.T) {
	h := NewServer(dmn.New(), WithHTTPToken("s3cret")).HTTPHandler()
	body := req(1, "initialize", `{}`)

	// No token → 401.
	if rec := postMCP(t, h, body, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("no token = %d, want 401", rec.Code)
	}
	// Wrong token → 401.
	if rec := postMCP(t, h, body, "nope"); rec.Code != http.StatusUnauthorized {
		t.Errorf("wrong token = %d, want 401", rec.Code)
	}
	// Correct token → 200.
	if rec := postMCP(t, h, body, "s3cret"); rec.Code != http.StatusOK {
		t.Errorf("correct token = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
}

// fakeAuth is a test Auth: it maps a bearer credential to its granted scopes and
// applies the same admin-as-super-scope rule the real keystore uses.
type fakeAuth struct{ keys map[string][]string }

func (f fakeAuth) Authorize(bearer, scope string) AuthResult {
	scopes, ok := f.keys[bearer]
	if !ok {
		return AuthUnauthenticated
	}
	if scope == "" {
		return AuthAllowed
	}
	for _, s := range scopes {
		if s == scope || s == "admin" {
			return AuthAllowed
		}
	}
	return AuthForbidden
}

// TestHTTPScopeAuthorization drives the MCP tool→scope gate (ADR-0028, WP-101):
// a key with the tool's scope reaches it (200), a key lacking it is 403, an
// unknown key is 401, and discovery messages need only a valid key.
func TestHTTPScopeAuthorization(t *testing.T) {
	auth := fakeAuth{keys: map[string][]string{
		"reader.r": {"models:read"},
		"runner.e": {"evaluate"},
		"gitter.g": {"git"},
		"boss.a":   {"admin"},
	}}
	h := NewServer(dmn.New(), WithAuth(auth)).HTTPHandler()
	xml, _ := json.Marshal(dishXML(t))
	evalBody := call(1, "evaluate", `{"xml":`+string(xml)+`,"decision":"Dish","input":{"Season":"Winter","Guest Count":8}}`)
	listBody := call(2, "list_models", `{}`)

	tests := []struct {
		name   string
		body   string
		bearer string
		want   int
	}{
		{"evaluate with evaluate key", evalBody, "runner.e", http.StatusOK},
		{"evaluate with read key is forbidden", evalBody, "reader.r", http.StatusForbidden},
		{"list with read key", listBody, "reader.r", http.StatusOK},
		{"list with evaluate key is forbidden", listBody, "runner.e", http.StatusForbidden},
		{"admin reaches evaluate", evalBody, "boss.a", http.StatusOK},
		{"unknown key is unauthorized", evalBody, "ghost.x", http.StatusUnauthorized},
		{"no key is unauthorized", evalBody, "", http.StatusUnauthorized},
		{"initialize needs only a valid key", req(3, "initialize", `{}`), "reader.r", http.StatusOK},
		{"initialize without a key is unauthorized", req(4, "initialize", `{}`), "", http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postMCP(t, h, tt.body, tt.bearer)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.want, rec.Body)
			}
		})
	}
}
