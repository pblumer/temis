package service

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// createManagedKey POSTs /v1/keys as admin and returns the created key response.
func createManagedKey(t *testing.T, h http.Handler, admin string, scopes ...string) createdKeyResponse {
	t.Helper()
	body := `{"scopes":[` + quoteJoin(scopes) + `]}`
	rec := doAuth(t, h, "POST", "/v1/keys", "application/json", []byte(body), admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create key = %d, want 201 (%s)", rec.Code, rec.Body)
	}
	return decode[createdKeyResponse](t, rec)
}

func quoteJoin(ss []string) string {
	q := make([]string, len(ss))
	for i, s := range ss {
		q[i] = `"` + s + `"`
	}
	return strings.Join(q, ",")
}

// TestKeyLifecycleE2E is the WP-103 acceptance path: create → auth ok → rotate →
// old secret 401 / new secret ok → revoke → 401.
func TestKeyLifecycleE2E(t *testing.T) {
	const admin = "admintok"
	dir := t.TempDir()
	h := NewServer(nil, WithToken(admin), WithKeyStore(dir)).Handler()

	// Create a models:read key.
	created := createManagedKey(t, h, admin, "models:read")
	if created.Secret == "" || created.Bearer != created.Kid+"."+created.Secret {
		t.Fatalf("bad create response: %+v", created)
	}

	// The new key authenticates and reaches a models:read route.
	if rec := doAuth(t, h, "GET", "/v1/models", "", nil, created.Bearer); rec.Code != http.StatusOK {
		t.Fatalf("new key GET /v1/models = %d, want 200 (%s)", rec.Code, rec.Body)
	}
	// It does NOT reach a models:write route (scope enforced).
	if rec := doAuth(t, h, "POST", "/v1/models", "application/xml", dishXML(t), created.Bearer); rec.Code != http.StatusForbidden {
		t.Fatalf("read key POST /v1/models = %d, want 403", rec.Code)
	}

	// Rotate: old secret stops working, new secret works.
	rec := doAuth(t, h, "POST", "/v1/keys/"+created.Kid+"/rotate", "", nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("rotate = %d, want 200 (%s)", rec.Code, rec.Body)
	}
	rotated := decode[createdKeyResponse](t, rec)
	if rotated.Secret == created.Secret {
		t.Fatal("rotate returned the same secret")
	}
	if rec := doAuth(t, h, "GET", "/v1/models", "", nil, created.Bearer); rec.Code != http.StatusUnauthorized {
		t.Fatalf("old secret after rotate = %d, want 401", rec.Code)
	}
	if rec := doAuth(t, h, "GET", "/v1/models", "", nil, rotated.Bearer); rec.Code != http.StatusOK {
		t.Fatalf("new secret after rotate = %d, want 200 (%s)", rec.Code, rec.Body)
	}

	// Revoke: the key no longer authenticates.
	if rec := doAuth(t, h, "POST", "/v1/keys/"+created.Kid+"/revoke", "", nil, admin); rec.Code != http.StatusOK {
		t.Fatalf("revoke = %d, want 200 (%s)", rec.Code, rec.Body)
	}
	if rec := doAuth(t, h, "GET", "/v1/models", "", nil, rotated.Bearer); rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked key = %d, want 401", rec.Code)
	}
}

// TestKeyListHasNoSecrets asserts the listing never leaks secret material and
// reflects state (scopes, revoked).
func TestKeyListHasNoSecrets(t *testing.T) {
	const admin = "admintok"
	h := NewServer(nil, WithToken(admin), WithKeyStore(t.TempDir())).Handler()
	created := createManagedKey(t, h, admin, "evaluate")

	rec := doAuth(t, h, "GET", "/v1/keys", "", nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d, want 200", rec.Code)
	}
	// The raw body must not contain the plaintext secret or a hash field.
	body := rec.Body.String()
	if strings.Contains(body, created.Secret) {
		t.Fatal("listing leaked the plaintext secret")
	}
	if strings.Contains(body, "secret") || strings.Contains(body, "hash") {
		t.Fatalf("listing exposes a secret/hash field: %s", body)
	}
	list := decode[listKeysResponse](t, rec)
	var found *KeyView
	for i := range list.Keys {
		if list.Keys[i].Kid == created.Kid {
			found = &list.Keys[i]
		}
	}
	if found == nil {
		t.Fatalf("created kid %q not in listing", created.Kid)
	}
	if !found.Managed || found.Revoked {
		t.Errorf("view = %+v, want managed && !revoked", *found)
	}
}

// TestKeyMgmtRequiresAdmin asserts a non-admin key cannot use the lifecycle API.
func TestKeyMgmtRequiresAdmin(t *testing.T) {
	const admin = "admintok"
	h := NewServer(nil, WithToken(admin), WithKeyStore(t.TempDir())).Handler()
	reader := createManagedKey(t, h, admin, "models:read")

	rec := doAuth(t, h, "POST", "/v1/keys", "application/json", []byte(`{"scopes":["evaluate"]}`), reader.Bearer)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin create = %d, want 403 (%s)", rec.Code, rec.Body)
	}
	if p := decode[problem](t, rec); p.Code != "FORBIDDEN" {
		t.Errorf("code = %q, want FORBIDDEN", p.Code)
	}
}

// TestKeyMgmtDisabled asserts the endpoints are absent (404) without a key store.
func TestKeyMgmtDisabled(t *testing.T) {
	const admin = "admintok"
	h := NewServer(nil, WithToken(admin)).Handler() // no WithKeyStore
	rec := doAuth(t, h, "POST", "/v1/keys", "application/json", []byte(`{"scopes":["evaluate"]}`), admin)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("create with no key store = %d, want 404 (%s)", rec.Code, rec.Body)
	}
}

// TestKeysSurviveRestart asserts managed keys (and revocation) persist across a
// restart: a fresh server on the same directory loads them.
func TestKeysSurviveRestart(t *testing.T) {
	const admin = "admintok"
	dir := t.TempDir()

	h1 := NewServer(nil, WithToken(admin), WithKeyStore(dir)).Handler()
	live := createManagedKey(t, h1, admin, "models:read")
	doomed := createManagedKey(t, h1, admin, "models:read")
	if rec := doAuth(t, h1, "POST", "/v1/keys/"+doomed.Kid+"/revoke", "", nil, admin); rec.Code != http.StatusOK {
		t.Fatalf("revoke = %d", rec.Code)
	}

	// Restart: a new server on the same dir.
	h2 := NewServer(nil, WithToken(admin), WithKeyStore(dir)).Handler()
	if rec := doAuth(t, h2, "GET", "/v1/models", "", nil, live.Bearer); rec.Code != http.StatusOK {
		t.Fatalf("live key after restart = %d, want 200 (%s)", rec.Code, rec.Body)
	}
	if rec := doAuth(t, h2, "GET", "/v1/models", "", nil, doomed.Bearer); rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked key after restart = %d, want 401", rec.Code)
	}
}

// TestRotateUnmanagedKeyConflicts asserts a static/operator key cannot be rotated
// or revoked through the runtime API (409), and an unknown kid is 404.
func TestRotateUnmanagedKeyConflicts(t *testing.T) {
	path := writeKeysFile(t, []scopedKey{{kid: "adm", secret: "s", scopes: []Scope{ScopeAdmin}}})
	h := NewServer(nil, WithKeysFile(path), WithKeyStore(t.TempDir())).Handler()
	const admin = "adm.s"

	if rec := doAuth(t, h, "POST", "/v1/keys/adm/rotate", "", nil, admin); rec.Code != http.StatusConflict {
		t.Fatalf("rotate static key = %d, want 409 (%s)", rec.Code, rec.Body)
	}
	if rec := doAuth(t, h, "POST", "/v1/keys/nope/revoke", "", nil, admin); rec.Code != http.StatusNotFound {
		t.Fatalf("revoke unknown key = %d, want 404 (%s)", rec.Code, rec.Body)
	}
}

// TestOfflineKeyAdmin covers WP-104: a key minted offline (server stopped) is
// accepted by the running server; listing carries no secret; rotate invalidates
// the old secret; the running server picks up the offline rotation on restart.
func TestOfflineKeyAdmin(t *testing.T) {
	dir := t.TempDir()

	admin, err := OpenKeyStore(dir)
	if err != nil {
		t.Fatalf("OpenKeyStore: %v", err)
	}
	kid, secret, err := admin.Create([]string{"models:read"}, "recovery", time.Time{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// A server started on the same dir accepts the offline-minted key.
	h := NewServer(nil, WithToken("admintok"), WithKeyStore(dir)).Handler()
	if rec := doAuth(t, h, "GET", "/v1/models", "", nil, kid+"."+secret); rec.Code != http.StatusOK {
		t.Fatalf("offline key on running server = %d, want 200 (%s)", rec.Code, rec.Body)
	}

	// List carries no secret material.
	for _, v := range admin.List() {
		if v.Kid == kid && (v.Revoked || !v.Managed) {
			t.Errorf("offline view = %+v, want managed && !revoked", v)
		}
	}

	// Rotate offline: the old secret stops working, the new one works — on a fresh
	// server (simulating the operator restarting after recovery).
	admin2, err := OpenKeyStore(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	newSecret, err := admin2.Rotate(kid)
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if newSecret == secret {
		t.Fatal("rotate returned the same secret")
	}
	h2 := NewServer(nil, WithToken("admintok"), WithKeyStore(dir)).Handler()
	if rec := doAuth(t, h2, "GET", "/v1/models", "", nil, kid+"."+secret); rec.Code != http.StatusUnauthorized {
		t.Errorf("old secret after offline rotate = %d, want 401", rec.Code)
	}
	if rec := doAuth(t, h2, "GET", "/v1/models", "", nil, kid+"."+newSecret); rec.Code != http.StatusOK {
		t.Errorf("new secret after offline rotate = %d, want 200 (%s)", rec.Code, rec.Body)
	}
}

// TestOpenKeyStoreEmptyDir rejects an empty directory argument.
func TestOpenKeyStoreEmptyDir(t *testing.T) {
	if _, err := OpenKeyStore(""); err == nil {
		t.Error("OpenKeyStore(\"\") = nil error, want failure")
	}
}
