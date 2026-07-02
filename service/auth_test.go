package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// makeKey builds a Key from a plaintext secret, holding only its hash (mirroring
// what the keystore stores).
func makeKey(kid, secret string, scopes ...Scope) *Key {
	return &Key{Kid: kid, hash: sha256.Sum256([]byte(secret)), Scopes: scopes}
}

// TestKeystoreAuthenticate is the WP-100 table: a valid key with the right secret
// authenticates; an unknown kid, wrong secret, expired or revoked key does not.
func TestKeystoreAuthenticate(t *testing.T) {
	past := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	future := time.Date(2999, 1, 1, 0, 0, 0, 0, time.UTC)

	ks := newKeystore()
	mustAdd(t, ks, makeKey("ci01", "s3cret", ScopeModelsWrite))
	expired := makeKey("old", "shh", ScopeEvaluate)
	expired.ExpiresAt = past
	mustAdd(t, ks, expired)
	live := makeKey("live", "shh", ScopeEvaluate)
	live.ExpiresAt = future
	mustAdd(t, ks, live)
	revoked := makeKey("dead", "shh", ScopeEvaluate)
	revoked.Revoked = true
	mustAdd(t, ks, revoked)

	tests := []struct {
		name    string
		bearer  string
		wantKid string // "" = authentication must fail
	}{
		{"valid", "ci01.s3cret", "ci01"},
		{"valid not expired", "live.shh", "live"},
		{"unknown kid", "nope.s3cret", ""},
		{"wrong secret", "ci01.wrong", ""},
		{"expired", "old.shh", ""},
		{"revoked", "dead.shh", ""},
		{"empty", "", ""},
		{"no dot", "ci01s3cret", ""},
		{"empty secret", "ci01.", ""},
		{"empty kid", ".s3cret", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, ok := ks.authenticate(tt.bearer)
			if tt.wantKid == "" {
				if ok {
					t.Fatalf("authenticate(%q) = ok, want failure", tt.bearer)
				}
				return
			}
			if !ok {
				t.Fatalf("authenticate(%q) failed, want key %q", tt.bearer, tt.wantKid)
			}
			if key.Kid != tt.wantKid {
				t.Errorf("kid = %q, want %q", key.Kid, tt.wantKid)
			}
		})
	}
}

// TestKeystoreHoldsNoPlaintext asserts the keystore never retains the secret in
// plaintext — only its SHA-256 hash. It scans every stored Key's fields for the
// secret bytes.
func TestKeystoreHoldsNoPlaintext(t *testing.T) {
	const secret = "the-plaintext-secret-value"
	ks := newKeystore()
	mustAdd(t, ks, makeKey("k", secret, ScopeAdmin))

	key := ks.keys["k"]
	// The hash must equal sha256(secret) and must not be the plaintext.
	want := sha256.Sum256([]byte(secret))
	if key.hash != want {
		t.Fatalf("stored hash != sha256(secret)")
	}
	// A defensive structural scan: no field of the key holds the plaintext.
	if containsSecret(reflect.ValueOf(*key), secret) {
		t.Fatalf("plaintext secret found in stored Key")
	}
}

// containsSecret walks a value and reports whether any string field equals or
// contains the secret.
func containsSecret(v reflect.Value, secret string) bool {
	switch v.Kind() {
	case reflect.String:
		return v.String() != "" && contains([]string{v.String()}, secret)
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			if !f.CanInterface() {
				// Unexported (e.g. hash [32]byte) — inspect as bytes below.
				if f.Kind() == reflect.Array {
					continue // a fixed hash array can never equal the longer plaintext
				}
				continue
			}
			if containsSecret(f, secret) {
				return true
			}
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			if containsSecret(v.Index(i), secret) {
				return true
			}
		}
	}
	return false
}

// TestKeyHasScope verifies scope checks, including admin as a super-scope.
func TestKeyHasScope(t *testing.T) {
	reader := makeKey("r", "x", ScopeModelsRead)
	if !reader.HasScope(ScopeModelsRead) {
		t.Error("reader should have models:read")
	}
	if reader.HasScope(ScopeModelsWrite) {
		t.Error("reader should not have models:write")
	}
	admin := makeKey("a", "x", ScopeAdmin)
	for _, s := range []Scope{ScopeEvaluate, ScopeModelsRead, ScopeModelsWrite, ScopeGit, ScopeAssist, ScopeFlow, ScopeAdmin, ScopeAudit} {
		if !admin.HasScope(s) {
			t.Errorf("admin should satisfy %q (super-scope)", s)
		}
	}
}

// TestKeystoreEnabled covers the open-by-default rule.
func TestKeystoreEnabled(t *testing.T) {
	if newKeystore().enabled() {
		t.Error("empty keystore should not be enabled (API stays open)")
	}
	ks := newKeystore()
	ks.setLegacyToken("tok")
	if !ks.enabled() {
		t.Error("legacy token should enable the keystore")
	}
	ks2 := newKeystore()
	mustAdd(t, ks2, makeKey("k", "s", ScopeEvaluate))
	if !ks2.enabled() {
		t.Error("a configured key should enable the keystore")
	}
}

// TestLegacyTokenIsSyntheticAdmin verifies the deprecated -token authenticates as
// a whole-string bearer and grants admin (every scope), byte-identical to before.
func TestLegacyTokenIsSyntheticAdmin(t *testing.T) {
	ks := newKeystore()
	ks.setLegacyToken("s3cr3t-token")
	key, ok := ks.authenticate("s3cr3t-token")
	if !ok {
		t.Fatal("legacy token should authenticate")
	}
	if !key.HasScope(ScopeAdmin) || !key.HasScope(ScopeEvaluate) {
		t.Error("legacy token should be a synthetic admin key")
	}
	if _, ok := ks.authenticate("wrong"); ok {
		t.Error("a non-matching bearer must not authenticate")
	}
}

// TestLoadKeysFile covers parsing scoped keys from JSON (WP-102), including the
// preferred secretHash form and the plaintext-secret convenience.
func TestLoadKeysFile(t *testing.T) {
	hash := sha256.Sum256([]byte("topsecret"))
	doc := `{"keys":[
	  {"kid":"ci","secretHash":"` + hex.EncodeToString(hash[:]) + `","scopes":["models:write"],"owner":"CI"},
	  {"kid":"agent","secret":"letmein","scopes":["evaluate"],"expiresAt":"2999-01-01T00:00:00Z"}
	]}`
	keys, err := loadKeysFile([]byte(doc))
	if err != nil {
		t.Fatalf("loadKeysFile: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("got %d keys, want 2", len(keys))
	}
	ks := newKeystore()
	for _, k := range keys {
		mustAdd(t, ks, k)
	}
	if _, ok := ks.authenticate("ci.topsecret"); !ok {
		t.Error("secretHash key should authenticate against its plaintext")
	}
	if _, ok := ks.authenticate("agent.letmein"); !ok {
		t.Error("plaintext-secret key should authenticate")
	}
	if ks.keys["ci"].Owner != "CI" {
		t.Errorf("owner = %q, want CI", ks.keys["ci"].Owner)
	}
}

// TestLoadKeysFileErrors covers the loud-failure cases.
func TestLoadKeysFileErrors(t *testing.T) {
	cases := map[string]string{
		"malformed json": `{`,
		"unknown field":  `{"keys":[{"kid":"k","secret":"s","scopes":["evaluate"],"bogus":1}]}`,
		"missing kid":    `{"keys":[{"secret":"s","scopes":["evaluate"]}]}`,
		"no secret":      `{"keys":[{"kid":"k","scopes":["evaluate"]}]}`,
		"unknown scope":  `{"keys":[{"kid":"k","secret":"s","scopes":["superuser"]}]}`,
		"no scopes":      `{"keys":[{"kid":"k","secret":"s","scopes":[]}]}`,
		"bad hash":       `{"keys":[{"kid":"k","secretHash":"zz","scopes":["evaluate"]}]}`,
		"short hash":     `{"keys":[{"kid":"k","secretHash":"abcd","scopes":["evaluate"]}]}`,
		"bad expiry":     `{"keys":[{"kid":"k","secret":"s","scopes":["evaluate"],"expiresAt":"soon"}]}`,
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := loadKeysFile([]byte(doc)); err == nil {
				t.Errorf("loadKeysFile(%s) = nil error, want failure", name)
			}
		})
	}
}

// TestBuildKeystore covers assembling from file + bootstrap + legacy token and
// the precedence rules (WP-102).
func TestBuildKeystore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.json")
	if err := os.WriteFile(path, []byte(`{"keys":[{"kid":"ci","secret":"filesecret","scopes":["models:write"]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	ks, bootKid, err := buildKeystore(path, "bootsecret", "legacytok")
	if err != nil {
		t.Fatalf("buildKeystore: %v", err)
	}
	if bootKid == "" {
		t.Fatal("expected a bootstrap kid")
	}
	// File key authenticates and is models:write.
	if key, ok := ks.authenticate("ci.filesecret"); !ok || !key.HasScope(ScopeModelsWrite) {
		t.Error("file key should authenticate with models:write")
	}
	// Bootstrap admin authenticates with its derived kid and is admin.
	if key, ok := ks.authenticate(bootKid + ".bootsecret"); !ok || !key.HasScope(ScopeAdmin) {
		t.Error("bootstrap key should authenticate with admin")
	}
	// Legacy token still works as admin.
	if key, ok := ks.authenticate("legacytok"); !ok || !key.HasScope(ScopeAdmin) {
		t.Error("legacy token should authenticate as admin")
	}
}

// TestBootstrapKidStableAcrossRestarts asserts the derived kid is a pure function
// of the secret, so it survives a restart.
func TestBootstrapKidStableAcrossRestarts(t *testing.T) {
	a := bootstrapKey("same-secret").Kid
	b := bootstrapKey("same-secret").Kid
	if a != b {
		t.Errorf("bootstrap kid not stable: %q vs %q", a, b)
	}
	if bootstrapKey("other").Kid == a {
		t.Error("different secrets should derive different kids")
	}
}

// TestBuildKeystoreFilePrecedence asserts a keys-file entry wins over a bootstrap
// key sharing its kid.
func TestBuildKeystoreFilePrecedence(t *testing.T) {
	boot := bootstrapKey("bootsecret")
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.json")
	// Give the file a key with the same kid the bootstrap secret derives, but with a
	// different secret and only evaluate scope.
	doc := map[string]any{"keys": []map[string]any{{
		"kid": boot.Kid, "secret": "filewins", "scopes": []string{"evaluate"},
	}}}
	b, _ := json.Marshal(doc)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	ks, _, err := buildKeystore(path, "bootsecret", "")
	if err != nil {
		t.Fatalf("buildKeystore: %v", err)
	}
	// The file's secret authenticates; the bootstrap secret does not (file wins).
	if _, ok := ks.authenticate(boot.Kid + ".filewins"); !ok {
		t.Error("file key should win on kid collision")
	}
	if _, ok := ks.authenticate(boot.Kid + ".bootsecret"); ok {
		t.Error("bootstrap secret should be shadowed by the file entry")
	}
}

func mustAdd(t *testing.T, ks *keystore, k *Key) {
	t.Helper()
	if err := ks.add(k); err != nil {
		t.Fatalf("add key %q: %v", k.Kid, err)
	}
}
