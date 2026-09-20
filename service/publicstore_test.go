package service

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// TestPublicToggleRuntime: an admin opens a model for anonymous evaluation at
// runtime, it takes effect immediately, and closing it removes access — no
// restart, no redeploy.
func TestPublicToggleRuntime(t *testing.T) {
	keys := writeKeysFile(t, []scopedKey{{"boss", "a", []Scope{ScopeAdmin}}})
	h := NewServer(nil, WithKeysFile(keys)).Handler()
	id := modelID(dishXML(t))
	doAuth(t, h, "POST", "/v1/models", "application/xml", dishXML(t), "boss.a")

	// Not public yet → anonymous evaluate is 401.
	if rec := do(t, h, "POST", "/v1/models/"+id+"/evaluate", "application/json", evalDishBody(t)); rec.Code != http.StatusUnauthorized {
		t.Fatalf("before toggle: anonymous evaluate = %d, want 401", rec.Code)
	}
	// Admin opens it by name.
	cfg := decode[AccessPublicConfig](t, doAuth(t, h, "POST", "/v1/access/public/models", "application/json",
		mustJSON(t, setPublicModelRequest{Model: "Dish", Public: true}), "boss.a"))
	if len(cfg.Managed) != 1 || cfg.Managed[0] != "Dish" {
		t.Fatalf("after add: managed = %v, want [Dish]", cfg.Managed)
	}
	// Now anonymous evaluate is served.
	if rec := do(t, h, "POST", "/v1/models/"+id+"/evaluate", "application/json", evalDishBody(t)); rec.Code != http.StatusOK {
		t.Fatalf("after toggle on: anonymous evaluate = %d, want 200 (%s)", rec.Code, rec.Body)
	}
	// Admin closes it again → back to 401.
	doAuth(t, h, "POST", "/v1/access/public/models", "application/json",
		mustJSON(t, setPublicModelRequest{Model: "Dish", Public: false}), "boss.a")
	if rec := do(t, h, "POST", "/v1/models/"+id+"/evaluate", "application/json", evalDishBody(t)); rec.Code != http.StatusUnauthorized {
		t.Fatalf("after toggle off: anonymous evaluate = %d, want 401", rec.Code)
	}
}

// TestPublicToggleStaticImmutable: a static (-public-models) entry cannot be
// removed at runtime — it is deployment config (409).
func TestPublicToggleStaticImmutable(t *testing.T) {
	keys := writeKeysFile(t, []scopedKey{{"boss", "a", []Scope{ScopeAdmin}}})
	h := NewServer(nil, WithKeysFile(keys), WithPublicModels("Dish")).Handler()
	rec := doAuth(t, h, "POST", "/v1/access/public/models", "application/json",
		mustJSON(t, setPublicModelRequest{Model: "Dish", Public: false}), "boss.a")
	if rec.Code != http.StatusConflict {
		t.Fatalf("removing static entry = %d, want 409", rec.Code)
	}
	if p := decode[problem](t, rec); p.Code != "PUBLIC_STATIC" {
		t.Errorf("code = %q, want PUBLIC_STATIC", p.Code)
	}
}

// TestPublicToggleAuthz: only admins may toggle; non-admins are 403, anonymous 401.
func TestPublicToggleAuthz(t *testing.T) {
	keys := writeKeysFile(t, []scopedKey{
		{"boss", "a", []Scope{ScopeAdmin}},
		{"runner", "e", []Scope{ScopeEvaluate}},
	})
	h := NewServer(nil, WithKeysFile(keys)).Handler()
	body := mustJSON(t, setPublicModelRequest{Model: "Dish", Public: true})
	if rec := doAuth(t, h, "POST", "/v1/access/public/models", "application/json", body, "runner.e"); rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin toggle = %d, want 403", rec.Code)
	}
	if rec := do(t, h, "POST", "/v1/access/public/models", "application/json", body); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous toggle = %d, want 401", rec.Code)
	}
}

// TestPublicTogglePersists: with an access-control dir the managed set is written
// to public.json and reloaded on restart, so a toggle survives.
func TestPublicTogglePersists(t *testing.T) {
	keys := writeKeysFile(t, []scopedKey{{"boss", "a", []Scope{ScopeAdmin}}})
	dir := t.TempDir()
	h := NewServer(nil, WithKeysFile(keys), WithKeyStore(dir)).Handler()
	cfg := decode[AccessPublicConfig](t, doAuth(t, h, "POST", "/v1/access/public/models", "application/json",
		mustJSON(t, setPublicModelRequest{Model: "Dish", Public: true}), "boss.a"))
	if !cfg.Persistent {
		t.Fatalf("with -keys-dir the store must report persistent")
	}
	if _, err := os.Stat(filepath.Join(dir, publicFileName)); err != nil {
		t.Fatalf("public.json not written: %v", err)
	}

	// A fresh server on the same dir reloads the managed entry.
	h2 := NewServer(nil, WithKeysFile(keys), WithKeyStore(dir)).Handler()
	id := modelID(dishXML(t))
	doAuth(t, h2, "POST", "/v1/models", "application/xml", dishXML(t), "boss.a")
	if rec := do(t, h2, "POST", "/v1/models/"+id+"/evaluate", "application/json", evalDishBody(t)); rec.Code != http.StatusOK {
		t.Fatalf("after restart: anonymous evaluate of persisted-public model = %d, want 200 (%s)", rec.Code, rec.Body)
	}
	got := decode[AccessPublicConfig](t, doAuth(t, h2, "GET", "/v1/access/public", "", nil, "boss.a"))
	if len(got.Managed) != 1 || got.Managed[0] != "Dish" {
		t.Fatalf("after restart: managed = %v, want [Dish]", got.Managed)
	}
}
