package service

import (
	"net/http"
	"os"
	"testing"
	"time"
)

// readModel reads a DMN model file for a test, failing the test if it cannot.
func readModel(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read model %s: %v", path, err)
	}
	return b
}

// TestSplitGrant covers parsing bare and resource-prefixed scope grants (WP-105).
func TestSplitGrant(t *testing.T) {
	cases := []struct {
		grant      Scope
		wantBase   Scope
		wantPrefix string
	}{
		{"evaluate", ScopeEvaluate, ""},
		{"evaluate:/orders/*", ScopeEvaluate, "/orders/*"},
		{"models:read", ScopeModelsRead, ""},
		{"models:read:sha256:ab", ScopeModelsRead, "sha256:ab"},
		{"models:write", ScopeModelsWrite, ""},
		{"admin", ScopeAdmin, ""},
		{"bogus", "bogus", ""},
	}
	for _, tc := range cases {
		base, prefix := splitGrant(tc.grant)
		if base != tc.wantBase || prefix != tc.wantPrefix {
			t.Errorf("splitGrant(%q) = (%q,%q), want (%q,%q)", tc.grant, base, prefix, tc.wantBase, tc.wantPrefix)
		}
	}
}

// TestGrantSatisfies covers the prefix-scope match rules (WP-105).
func TestGrantSatisfies(t *testing.T) {
	cases := []struct {
		grant    Scope
		want     Scope
		resource string
		ok       bool
	}{
		{"evaluate", ScopeEvaluate, "anything", true}, // unconstrained
		{"evaluate", ScopeEvaluate, "", true},         // unconstrained, no resource
		{"models:read", ScopeModelsWrite, "x", false}, // wrong base
		{"admin", ScopeEvaluate, "", true},            // super-scope
		{"evaluate:/orders/*", ScopeEvaluate, "/orders/42", true},
		{"evaluate:/orders/*", ScopeEvaluate, "/refunds/1", false},
		{"evaluate:/orders/", ScopeEvaluate, "/orders/42", true}, // trailing star optional
		{"evaluate:/orders/*", ScopeEvaluate, "", false},         // constrained needs a resource
		{"models:read:sha256:ab", ScopeModelsRead, "sha256:abcd", true},
		{"models:read:sha256:ab", ScopeModelsRead, "sha256:ffff", false},
	}
	for _, tc := range cases {
		if got := grantSatisfies(tc.grant, tc.want, tc.resource); got != tc.ok {
			t.Errorf("grantSatisfies(%q,%q,%q) = %v, want %v", tc.grant, tc.want, tc.resource, got, tc.ok)
		}
	}
}

// TestHTTPPrefixScopeRestrictsModel is the WP-105 AK: a prefix-scope pins a key to
// a modelId — it reaches the matching model and is 403 on any other.
func TestHTTPPrefixScopeRestrictsModel(t *testing.T) {
	// First, seed two models with an admin key to learn their ids.
	admin := writeKeysFile(t, []scopedKey{{kid: "boss", secret: "a", scopes: []Scope{ScopeAdmin}}})
	h := NewServer(nil, WithKeysFile(admin)).Handler()
	idA := decode[modelResponse](t, doAuth(t, h, "POST", "/v1/models", "application/xml", dishXML(t), "boss.a")).ModelID
	idB := decode[modelResponse](t, doAuth(t, h, "POST", "/v1/models", "application/xml", readModel(t, "../dmn/testdata/models/discount_14.dmn"), "boss.a")).ModelID
	if idA == idB {
		t.Fatal("expected two distinct model ids")
	}

	// A key pinned to model A (by its full id) via a prefix scope.
	pinned := writeKeysFile(t, []scopedKey{
		{kid: "boss", secret: "a", scopes: []Scope{ScopeAdmin}},
		{kid: "pin", secret: "s", scopes: []Scope{Scope("models:read:" + idA)}},
	})
	h = NewServer(nil, WithKeysFile(pinned)).Handler()
	// Re-seed both models on the new server.
	doAuth(t, h, "POST", "/v1/models", "application/xml", dishXML(t), "boss.a")
	doAuth(t, h, "POST", "/v1/models", "application/xml", readModel(t, "../dmn/testdata/models/discount_14.dmn"), "boss.a")

	if rec := doAuth(t, h, "GET", "/v1/models/"+idA, "", nil, "pin.s"); rec.Code != http.StatusOK {
		t.Errorf("pinned key on model A = %d, want 200 (%s)", rec.Code, rec.Body)
	}
	if rec := doAuth(t, h, "GET", "/v1/models/"+idB, "", nil, "pin.s"); rec.Code != http.StatusForbidden {
		t.Errorf("pinned key on model B = %d, want 403", rec.Code)
	}
	// The pinned key cannot use a resource-less models:read route (the listing),
	// since a constrained grant has nothing to match there.
	if rec := doAuth(t, h, "GET", "/v1/models", "", nil, "pin.s"); rec.Code != http.StatusForbidden {
		t.Errorf("pinned key on listing = %d, want 403", rec.Code)
	}
}

// TestHTTPExpiredKeyRejected is the WP-105 expiry AK: an expired key is 401.
func TestHTTPExpiredKeyRejected(t *testing.T) {
	const admin = "admintok"
	h := NewServer(nil, WithToken(admin), WithKeyStore(t.TempDir())).Handler()

	// Create a key that already expired.
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	body := `{"scopes":["models:read"],"expiresAt":"` + past + `"}`
	rec := doAuth(t, h, "POST", "/v1/keys", "application/json", []byte(body), admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d, want 201 (%s)", rec.Code, rec.Body)
	}
	created := decode[createdKeyResponse](t, rec)

	if rec := doAuth(t, h, "GET", "/v1/models", "", nil, created.Bearer); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expired key = %d, want 401", rec.Code)
	}
}
