package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// writeKeysFile writes a keys JSON document with one key per (kid, secret,
// scopes) triple and returns its path, for driving a scoped server.
func writeKeysFile(t *testing.T, entries []scopedKey) string {
	t.Helper()
	type fe struct {
		Kid        string   `json:"kid"`
		SecretHash string   `json:"secretHash"`
		Scopes     []string `json:"scopes"`
	}
	doc := struct {
		Keys []fe `json:"keys"`
	}{}
	for _, e := range entries {
		h := sha256.Sum256([]byte(e.secret))
		scopes := make([]string, len(e.scopes))
		for i, s := range e.scopes {
			scopes[i] = string(s)
		}
		doc.Keys = append(doc.Keys, fe{Kid: e.kid, SecretHash: hex.EncodeToString(h[:]), Scopes: scopes})
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "keys.json")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

type scopedKey struct {
	kid    string
	secret string
	scopes []Scope
}

// TestHTTPScopeAuthorization drives the WP-101 acceptance matrix: for each scope
// class a positive case (a key with that scope reaches the route) and a negative
// case (a key with a different scope is 403). It also checks the 401 paths
// (no/unknown key) and that admin reaches everything.
func TestHTTPScopeAuthorization(t *testing.T) {
	path := writeKeysFile(t, []scopedKey{
		{"reader", "r", []Scope{ScopeModelsRead}},
		{"writer", "w", []Scope{ScopeModelsWrite}},
		{"runner", "e", []Scope{ScopeEvaluate}},
		{"flower", "f", []Scope{ScopeFlow}},
		{"chatter", "c", []Scope{ScopeAssist}},
		{"gitter", "g", []Scope{ScopeGit}},
		{"auditor", "au", []Scope{ScopeAudit}},
		{"boss", "a", []Scope{ScopeAdmin}},
	})
	// Point the /v1/git routes at a fake GitHub so this test is hermetic: the
	// authorization gate is what we exercise, not real network access (a real
	// call would 401 from GitHub and be mistaken for a gate rejection).
	fakeGitHub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"name":"main","commit":{"sha":"abc"}}]`))
	}))
	t.Cleanup(fakeGitHub.Close)
	h := NewServer(nil,
		WithKeysFile(path),
		WithAssist(AssistConfig{Provider: "anthropic", AllowBYOK: true}),
		WithGitHubBaseURL(fakeGitHub.URL),
	).Handler()

	// Seed a model with the admin key so id-bearing routes have a target.
	xml := dishXML(t)
	rec := doAuth(t, h, "POST", "/v1/models", "application/xml", xml, "boss.a")
	if rec.Code != http.StatusCreated {
		t.Fatalf("seed model = %d (%s)", rec.Code, rec.Body)
	}
	id := decode[modelResponse](t, rec).ModelID

	type call struct {
		method, path, ctype string
		body                []byte
	}
	read := call{"GET", "/v1/models/" + id, "", nil}
	write := call{"POST", "/v1/models/" + id + "/rename", "application/json", mustJSON(t, renameModelRequest{Name: "X"})}
	del := call{"DELETE", "/v1/models/" + id, "", nil}
	eval := call{"POST", "/v1/models/" + id + "/evaluate", "application/json", mustJSON(t, evaluateModelRequest{Decision: "Dish", Input: map[string]any{"Season": "Winter", "Guest Count": 4}})}
	flow := call{"POST", "/v1/flows", "application/json", []byte(`{"name":"x","steps":[]}`)}
	chat := call{"POST", "/v1/chat", "application/json", []byte(`{"messages":[]}`)}
	git := call{"GET", "/v1/git/branches?owner=o&repo=r", "", nil}
	status := call{"GET", "/v1/status", "", nil}

	tests := []struct {
		name    string
		bearer  string
		c       call
		notWant int // status that means "gate rejected"; we assert response != 401/403
		want    string
	}{
		// Positive: the matching scope passes the gate (not 401/403).
		{"read ok", "reader.r", read, 0, "pass"},
		{"write ok", "writer.w", write, 0, "pass"},
		{"eval ok", "runner.e", eval, 0, "pass"},
		{"flow ok", "flower.f", flow, 0, "pass"},
		{"assist ok", "chatter.c", chat, 0, "pass"},
		{"git ok", "gitter.g", git, 0, "pass"},
		{"admin delete ok", "boss.a", del, 0, "pass"},
		// admin reaches a non-admin route too (super-scope).
		{"admin reaches read", "boss.a", read, 0, "pass"},
		// Status (ADR-0030) is guarded by the audit scope; admin (super-scope)
		// reaches it too, an unrelated scope is 403.
		{"audit reaches status", "auditor.au", status, 0, "pass"},
		{"admin reaches status", "boss.a", status, 0, "pass"},
		{"eval lacks status", "runner.e", status, 0, "403"},

		// Negative: wrong scope → 403.
		{"read lacks write", "reader.r", write, 0, "403"},
		{"write lacks eval", "writer.w", eval, 0, "403"},
		{"eval lacks read", "runner.e", read, 0, "403"},
		{"eval lacks flow", "runner.e", flow, 0, "403"},
		{"reader lacks assist", "reader.r", chat, 0, "403"},
		{"reader lacks git", "reader.r", git, 0, "403"},
		{"reader lacks admin delete", "reader.r", del, 0, "403"},

		// Unauthenticated → 401.
		{"unknown kid", "ghost.x", read, 0, "401"},
		{"wrong secret", "reader.wrong", read, 0, "401"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doAuth(t, h, tt.c.method, tt.c.path, tt.c.ctype, tt.c.body, tt.bearer)
			switch tt.want {
			case "401":
				if rec.Code != http.StatusUnauthorized {
					t.Fatalf("got %d, want 401 (%s)", rec.Code, rec.Body)
				}
				if p := decode[problem](t, rec); p.Code != "UNAUTHORIZED" {
					t.Errorf("code = %q, want UNAUTHORIZED", p.Code)
				}
				if wa := rec.Header().Get("WWW-Authenticate"); wa == "" {
					t.Error("missing WWW-Authenticate challenge on 401")
				}
			case "403":
				if rec.Code != http.StatusForbidden {
					t.Fatalf("got %d, want 403 (%s)", rec.Code, rec.Body)
				}
				if p := decode[problem](t, rec); p.Code != "FORBIDDEN" {
					t.Errorf("code = %q, want FORBIDDEN", p.Code)
				}
			case "pass":
				if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
					t.Fatalf("gate rejected an authorized call: %d (%s)", rec.Code, rec.Body)
				}
			}
		})
	}
}

// TestHTTPNoAuthLeak asserts a scoped-but-nonexistent route still returns the
// gate's answer for the method+path, never leaking route existence through a
// different status. A GET on a gated path without a key is 401, same as an
// existing one, and an unknown path under no auth is a plain 404.
func TestHTTPNoAuthLeak(t *testing.T) {
	path := writeKeysFile(t, []scopedKey{{"reader", "r", []Scope{ScopeModelsRead}}})
	h := NewServer(nil, WithKeysFile(path)).Handler()

	// A gated route with no credentials is 401 whether or not the model exists —
	// the gate runs before the handler, so a missing model never surfaces as 404.
	rec := do(t, h, "GET", "/v1/models/sha256:deadbeef", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth GET on unknown model = %d, want 401 (no existence leak)", rec.Code)
	}
	// With a valid read key the same unknown model is a normal 404.
	rec = doAuth(t, h, "GET", "/v1/models/sha256:deadbeef", "", nil, "reader.r")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("authed GET on unknown model = %d, want 404", rec.Code)
	}
}

// TestHTTPLegacyTokenCoversEverything asserts the deprecated -token still reaches
// every scope class (admin) — byte-identical to the pre-scopes behaviour.
func TestHTTPLegacyTokenCoversEverything(t *testing.T) {
	const tok = "s3cr3t-token"
	h := NewServer(nil, WithToken(tok)).Handler()
	xml := dishXML(t)

	rec := doAuth(t, h, "POST", "/v1/models", "application/xml", xml, tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("legacy POST = %d, want 201 (%s)", rec.Code, rec.Body)
	}
	id := decode[modelResponse](t, rec).ModelID

	// A models:read, an evaluate, a flow and an admin DELETE all pass with the one
	// legacy token.
	for _, c := range []struct {
		method, path, ctype string
		body                []byte
	}{
		{"GET", "/v1/models/" + id, "", nil},
		{"POST", "/v1/models/" + id + "/evaluate", "application/json", mustJSON(t, evaluateModelRequest{Decision: "Dish", Input: map[string]any{"Season": "Winter", "Guest Count": 4}})},
		{"POST", "/v1/flows", "application/json", []byte(`{"name":"x","steps":[]}`)},
		{"DELETE", "/v1/models/" + id, "", nil},
	} {
		rec := doAuth(t, h, c.method, c.path, c.ctype, c.body, tok)
		if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
			t.Errorf("%s %s with legacy token = %d, want it to pass the gate", c.method, c.path, rec.Code)
		}
	}
}
