package service

import (
	"net/http"
	"testing"
)

// TestWhoamiOpenAPI: with no auth configured the API is open, so whoami reports an
// authenticated admin — the access UI is fully visible in a dev run.
func TestWhoamiOpenAPI(t *testing.T) {
	h := NewServer(nil).Handler()
	id := decode[AccessIdentity](t, do(t, h, "GET", "/v1/whoami", "", nil))
	if id.AuthEnabled || !id.Authenticated || !id.IsAdmin {
		t.Fatalf("open API whoami = %+v, want authEnabled=false authenticated=true isAdmin=true", id)
	}
}

// TestWhoamiScoped: with keys configured whoami never 401s; it reports the caller's
// own scopes and admin-ness, and an anonymous caller as unauthenticated.
func TestWhoamiScoped(t *testing.T) {
	keys := writeKeysFile(t, []scopedKey{
		{"boss", "a", []Scope{ScopeAdmin}},
		{"runner", "e", []Scope{ScopeEvaluate}},
	})
	h := NewServer(nil, WithKeysFile(keys)).Handler()

	// Admin key → isAdmin.
	admin := decode[AccessIdentity](t, doAuth(t, h, "GET", "/v1/whoami", "", nil, "boss.a"))
	if !admin.AuthEnabled || !admin.Authenticated || !admin.IsAdmin || admin.Subject != "boss" {
		t.Fatalf("admin whoami = %+v, want authenticated admin subject=boss", admin)
	}
	// Evaluate key → authenticated but not admin.
	runner := decode[AccessIdentity](t, doAuth(t, h, "GET", "/v1/whoami", "", nil, "runner.e"))
	if !runner.Authenticated || runner.IsAdmin {
		t.Fatalf("runner whoami = %+v, want authenticated non-admin", runner)
	}
	// Anonymous → 200 but not authenticated (drives the login prompt).
	anon := decode[AccessIdentity](t, do(t, h, "GET", "/v1/whoami", "", nil))
	if !anon.AuthEnabled || anon.Authenticated || anon.IsAdmin {
		t.Fatalf("anonymous whoami = %+v, want authEnabled=true authenticated=false", anon)
	}
}

// TestAccessPublicConfig: the admin-scoped read returns the effective public
// configuration; a non-admin is 403 and an anonymous caller 401.
func TestAccessPublicConfig(t *testing.T) {
	keys := writeKeysFile(t, []scopedKey{
		{"boss", "a", []Scope{ScopeAdmin}},
		{"runner", "e", []Scope{ScopeEvaluate}},
	})
	h := NewServer(nil, WithKeysFile(keys), WithPublicEvaluate(true), WithPublicModels("Dish", "sha256:abc")).Handler()

	cfg := decode[AccessPublicConfig](t, doAuth(t, h, "GET", "/v1/access/public", "", nil, "boss.a"))
	if !cfg.Evaluate || len(cfg.Models) != 2 || cfg.Models[0] != "Dish" {
		t.Fatalf("public config = %+v, want evaluate=true models=[Dish sha256:abc]", cfg)
	}
	if rec := doAuth(t, h, "GET", "/v1/access/public", "", nil, "runner.e"); rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin public config = %d, want 403", rec.Code)
	}
	if rec := do(t, h, "GET", "/v1/access/public", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous public config = %d, want 401", rec.Code)
	}
}
