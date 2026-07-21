package service

import (
	"net/http"
	"testing"

	"github.com/pblumer/temis/mcp"
)

// evalDishBody is the JSON body for evaluating the dish model's "Dish" decision.
func evalDishBody(t *testing.T) []byte {
	t.Helper()
	return mustJSON(t, evaluateModelRequest{Decision: "Dish", Input: map[string]any{"Season": "Winter", "Guest Count": 4}})
}

// TestPublicModelEvaluateByID: with keys configured (auth enabled) a model on the
// per-model public allowlist (by modelId) is evaluable anonymously, while every
// other route — and evaluation of a non-listed model — still needs a key.
func TestPublicModelEvaluateByID(t *testing.T) {
	keys := writeKeysFile(t, []scopedKey{{"boss", "a", []Scope{ScopeAdmin}}})
	// The modelId is content-addressed, so it is known up front without seeding.
	publicID := modelID(dishXML(t))

	h := NewServer(nil, WithKeysFile(keys), WithPublicModels(publicID)).Handler()
	// Seed the model with the admin key so the public route has a target to evaluate.
	doAuth(t, h, "POST", "/v1/models", "application/xml", dishXML(t), "boss.a")

	// Anonymous evaluation of the public model → served (not 401/403).
	rec := do(t, h, "POST", "/v1/models/"+publicID+"/evaluate", "application/json", evalDishBody(t))
	if rec.Code != http.StatusOK {
		t.Fatalf("anonymous evaluate of public model = %d, want 200 (%s)", rec.Code, rec.Body)
	}
	// Anonymous evaluation of a non-listed model → 401 at the gate, before the
	// handler (no route/model existence leak, like TestHTTPNoAuthLeak).
	rec = do(t, h, "POST", "/v1/models/sha256:deadbeef/evaluate", "application/json",
		mustJSON(t, evaluateModelRequest{Decision: "Dish", Input: map[string]any{}}))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous evaluate of non-listed model = %d, want 401 (%s)", rec.Code, rec.Body)
	}
	// Public opens only evaluation: reading the public model anonymously is still 401.
	rec = do(t, h, "GET", "/v1/models/"+publicID, "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous read of public model = %d, want 401 (only evaluate is public) (%s)", rec.Code, rec.Body)
	}
	// A caller with an evaluate key is of course still served.
	// (admin is a super-scope, so the admin key evaluates too.)
	rec = doAuth(t, h, "POST", "/v1/models/"+publicID+"/evaluate", "application/json", evalDishBody(t), "boss.a")
	if rec.Code != http.StatusOK {
		t.Fatalf("admin evaluate of public model = %d, want 200 (%s)", rec.Code, rec.Body)
	}
}

// TestPublicModelEvaluateByName: the allowlist matches a model by its display
// name too, so a re-saved model (new modelId) stays public when listed by name.
func TestPublicModelEvaluateByName(t *testing.T) {
	keys := writeKeysFile(t, []scopedKey{{"boss", "a", []Scope{ScopeAdmin}}})
	h := NewServer(nil, WithKeysFile(keys), WithPublicModels("Dish")).Handler()
	rec := doAuth(t, h, "POST", "/v1/models", "application/xml", dishXML(t), "boss.a")
	if got := decode[modelResponse](t, rec).Name; got != "Dish" {
		t.Fatalf("seeded model name = %q, want %q (by-name public assumption)", got, "Dish")
	}
	id := decode[modelResponse](t, rec).ModelID

	rec = do(t, h, "POST", "/v1/models/"+id+"/evaluate", "application/json", evalDishBody(t))
	if rec.Code != http.StatusOK {
		t.Fatalf("anonymous evaluate of by-name public model = %d, want 200 (%s)", rec.Code, rec.Body)
	}
}

// TestPublicEvaluateGlobal: the global switch opens every evaluation surface,
// including the stateless POST /v1/evaluate, while write/admin/assist keep
// requiring a key.
func TestPublicEvaluateGlobal(t *testing.T) {
	keys := writeKeysFile(t, []scopedKey{{"boss", "a", []Scope{ScopeAdmin}}})
	h := NewServer(nil, WithKeysFile(keys), WithPublicEvaluate(true)).Handler()
	id := decode[modelResponse](t, doAuth(t, h, "POST", "/v1/models", "application/xml", dishXML(t), "boss.a")).ModelID

	// Anonymous id-addressed evaluation → served.
	rec := do(t, h, "POST", "/v1/models/"+id+"/evaluate", "application/json", evalDishBody(t))
	if rec.Code != http.StatusOK {
		t.Fatalf("anonymous id-evaluate = %d, want 200 (%s)", rec.Code, rec.Body)
	}
	// Anonymous stateless evaluation (XML + input in one request, no id) → served.
	rec = do(t, h, "POST", "/v1/evaluate", "application/json",
		mustJSON(t, evaluateStatelessRequest{XML: string(dishXML(t)), Decision: "Dish", Input: map[string]any{"Season": "Winter", "Guest Count": 4}}))
	if rec.Code != http.StatusOK {
		t.Fatalf("anonymous stateless evaluate = %d, want 200 (%s)", rec.Code, rec.Body)
	}
	// A mutating route stays gated for anonymous callers.
	rec = do(t, h, "POST", "/v1/models/"+id+"/rename", "application/json", mustJSON(t, renameModelRequest{Name: "X"}))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous rename under public-evaluate = %d, want 401 (%s)", rec.Code, rec.Body)
	}
}

// TestPublicModelUnitMatch is a table-level check of the allowlist matcher.
func TestPublicModelUnitMatch(t *testing.T) {
	s := NewServer(nil, WithKeysFile(writeKeysFile(t, []scopedKey{{"boss", "a", []Scope{ScopeAdmin}}})), WithPublicModels("sha256:abc", "Dish"))
	if !s.isPublicModel("sha256:abc") {
		t.Error("modelId match failed")
	}
	if s.isPublicModel("sha256:other") {
		t.Error("non-listed modelId must not match")
	}
	if s.isPublicModel("") {
		t.Error("empty id must never be public per-model")
	}
	// The global switch subsumes any id.
	sg := NewServer(nil, WithKeysFile(writeKeysFile(t, []scopedKey{{"boss", "a", []Scope{ScopeAdmin}}})), WithPublicEvaluate(true))
	if !sg.evaluateIsPublic("") {
		t.Error("global public-evaluate must open a resource-less evaluation")
	}
}

// TestMCPPublicEvaluate verifies the global switch reaches the MCP gate
// (ADR-0035): the evaluate tool is served without a key, while a models:read tool
// still requires one. Per-model public is deliberately not honoured over MCP (the
// gate sees only the scope, not the tool's model argument).
func TestMCPPublicEvaluate(t *testing.T) {
	keys := writeKeysFile(t, []scopedKey{{"boss", "a", []Scope{ScopeAdmin}}})
	on := NewServer(nil, WithKeysFile(keys), WithPublicEvaluate(true)).MCPAuth().(mcpAuth)
	if got := on.Authorize("", string(ScopeEvaluate)); got != mcp.AuthAllowed {
		t.Errorf("public evaluate: Authorize(evaluate, no key) = %v, want Allowed", got)
	}
	if got := on.Authorize("", string(ScopeModelsRead)); got != mcp.AuthUnauthenticated {
		t.Errorf("public evaluate: Authorize(models:read, no key) = %v, want Unauthenticated", got)
	}
	// With the switch off, evaluate needs a key like every other tool.
	off := NewServer(nil, WithKeysFile(keys)).MCPAuth().(mcpAuth)
	if got := off.Authorize("", string(ScopeEvaluate)); got != mcp.AuthUnauthenticated {
		t.Errorf("no public: Authorize(evaluate, no key) = %v, want Unauthenticated", got)
	}
}
