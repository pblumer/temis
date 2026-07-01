package service

// Scoped API-key authentication (ADR-0028, WP-100/101/102). This is the adapter
// layer's own gate; the engine core (package dmn) never imports it (ADR-0011).
// It replaces the flat all-or-nothing bearer token with clio's kid.secret model:
// a bearer is "<kid>.<secret>", the keystore holds only sha256(secret) (never the
// plaintext), the secret is verified in constant time, and every route carries a
// required Scope. Missing/invalid/expired/revoked → 401; valid but lacking the
// scope → 403. With no keys and no legacy token configured the API stays open,
// exactly as before. Pure stdlib: crypto/sha256, crypto/subtle, encoding/json.

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Scope is a coarse permission label attached to each key and required by each
// route (ADR-0028 §2). admin is a super-scope: a key holding it satisfies every
// requirement, which is how the legacy -token (a synthetic admin key) keeps
// covering all routes byte-identically.
type Scope string

// The scope vocabulary (ADR-0028 §2). Each route requires one of these.
const (
	ScopeEvaluate    Scope = "evaluate"     // run evaluations
	ScopeModelsRead  Scope = "models:read"  // read models, schemas, decisions
	ScopeModelsWrite Scope = "models:write" // create/edit models, modeler saves
	ScopeGit         Scope = "git"          // git-backed model/flow endpoints
	ScopeAssist      Scope = "assist"       // the LLM modeling assistant (costs money)
	ScopeFlow        Scope = "flow"         // decision-flow endpoints
	ScopeAdmin       Scope = "admin"        // key management, model DELETE, ops routes
	ScopeAudit       Scope = "audit"        // read-only audit log (Phase 3)
)

// knownScopes is the closed set accepted from configuration; an unknown scope in
// a keys file is a load error rather than a silently ineffective grant.
var knownScopes = map[Scope]bool{
	ScopeEvaluate: true, ScopeModelsRead: true, ScopeModelsWrite: true,
	ScopeGit: true, ScopeAssist: true, ScopeFlow: true, ScopeAdmin: true, ScopeAudit: true,
}

// Key is one API key. Only the SHA-256 of the secret is held — the plaintext is
// never stored (ADR-0028 §1).
type Key struct {
	Kid       string    // public key id (loggable)
	hash      [32]byte  // sha256(secret)
	Scopes    []Scope   // granted scopes
	Owner     string    // free-form owner label, for audit/identity
	ExpiresAt time.Time // zero = never expires
	Revoked   bool      // a revoked key never authenticates
	// managed marks a key created through the lifecycle API (WP-103); only managed
	// keys are persisted and may be rotated/revoked. Static-file keys, the
	// bootstrap key and the legacy token are unmanaged (operator-owned, immutable
	// at runtime).
	managed bool
}

// HasScope reports whether the key grants scope. A key with admin grants every
// scope (super-scope), so a legacy admin key covers the whole surface.
func (k *Key) HasScope(scope Scope) bool {
	for _, s := range k.Scopes {
		if s == ScopeAdmin || s == scope {
			return true
		}
	}
	return false
}

// Authenticator verifies a kid.secret bearer against a set of scoped keys and
// reports whether authentication is configured at all. The in-memory keystore is
// the Phase-1 implementation (ADR-0028); Phase 2 (WP-103) adds a persistent one
// behind the same seam. The methods are unexported on purpose: every
// implementation lives in this package, because auth never leaves the adapter
// layer (ADR-0011). requireScope, the gRPC interceptor and the MCP gate all sit
// on top of this seam.
type Authenticator interface {
	// authenticate verifies bearer, returning the matched key on success. It fails
	// (ok=false) for a missing, malformed, unknown, wrong, expired or revoked
	// credential — all of which map to 401 at the transports.
	authenticate(bearer string) (*Key, bool)
	// enabled reports whether any authentication is configured; when false the API
	// stays open (the historical default) and the gates are transparent.
	enabled() bool
}

var _ Authenticator = (*keystore)(nil)

// keystore is the in-memory Authenticator: keys by kid plus an optional legacy
// raw token that authenticates as a synthetic admin key. Static config is loaded
// at construction; the lifecycle API (WP-103) can then create/rotate/revoke
// managed keys at runtime, so a mutex guards the map and mutations flush to the
// optional persistent store.
type keystore struct {
	mu          sync.RWMutex
	keys        map[string]*Key
	legacyToken string           // raw -token / TEMIS_API_TOKEN; empty = none
	legacyKey   *Key             // the synthetic admin key the legacy token maps to
	now         func() time.Time // injectable clock for expiry (tests)
	persist     *keyPersist      // nil = keys are in-memory only (no -keys-dir)
}

func newKeystore() *keystore {
	return &keystore{keys: map[string]*Key{}, now: time.Now}
}

// enabled reports whether any authentication is configured. When false the API
// is open (the historical default) and the gates are transparent.
func (ks *keystore) enabled() bool {
	if ks == nil {
		return false
	}
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	return len(ks.keys) > 0 || ks.legacyToken != ""
}

// add registers a key, rejecting a duplicate or empty kid. It is used at
// construction (single-threaded) and under the write lock by the lifecycle API.
func (ks *keystore) add(k *Key) error {
	if k.Kid == "" {
		return errors.New("key has empty kid")
	}
	ks.mu.Lock()
	defer ks.mu.Unlock()
	if _, dup := ks.keys[k.Kid]; dup {
		return fmt.Errorf("duplicate kid %q", k.Kid)
	}
	ks.keys[k.Kid] = k
	return nil
}

// setLegacyToken records the deprecated single token as a synthetic admin key.
// It is matched as a whole-string bearer (not kid.secret) so existing clients
// that send "Authorization: Bearer <token>" keep working unchanged.
func (ks *keystore) setLegacyToken(token string) {
	if token == "" {
		return
	}
	ks.legacyToken = token
	ks.legacyKey = &Key{Kid: "legacy", Scopes: []Scope{ScopeAdmin}, Owner: "legacy -token (deprecated)"}
}

// authenticate verifies a bearer credential and returns the authenticated key.
// ok is false for any authentication failure — missing, malformed, unknown kid,
// wrong secret, expired or revoked — all of which map to 401 at the transports.
func (ks *keystore) authenticate(bearer string) (*Key, bool) {
	if bearer == "" {
		return nil, false
	}
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	// Legacy path: a whole-string constant-time match grants the synthetic admin
	// key. Checked first so a byte-identical legacy token keeps working.
	if ks.legacyToken != "" && subtle.ConstantTimeCompare([]byte(bearer), []byte(ks.legacyToken)) == 1 {
		return ks.legacyKey, true
	}
	kid, secret, ok := splitKidSecret(bearer)
	if !ok {
		return nil, false
	}
	key, ok := ks.keys[kid]
	if !ok {
		return nil, false
	}
	sum := sha256.Sum256([]byte(secret))
	if subtle.ConstantTimeCompare(sum[:], key.hash[:]) != 1 {
		return nil, false
	}
	if key.Revoked {
		return nil, false
	}
	if !key.ExpiresAt.IsZero() && ks.now().After(key.ExpiresAt) {
		return nil, false
	}
	return key, true
}

// splitKidSecret splits a "kid.secret" bearer on its first dot. Both parts must
// be non-empty; anything else is not a kid.secret credential.
func splitKidSecret(bearer string) (kid, secret string, ok bool) {
	i := strings.IndexByte(bearer, '.')
	if i <= 0 || i >= len(bearer)-1 {
		return "", "", false
	}
	return bearer[:i], bearer[i+1:], true
}

// --- static configuration (WP-102) ---

// keyFile is the JSON schema of a -keys-file / $TEMIS_KEYS_FILE document. Each
// entry supplies the SHA-256 hash of its secret (secretHash, hex) so the
// plaintext never touches disk; secret (plaintext) is accepted as a convenience
// and hashed on load, then discarded.
type keyFile struct {
	Keys []keyFileEntry `json:"keys"`
}

type keyFileEntry struct {
	Kid        string   `json:"kid"`
	Secret     string   `json:"secret,omitempty"`     // plaintext, hashed on load then dropped (convenience)
	SecretHash string   `json:"secretHash,omitempty"` // hex sha256(secret) — preferred, keeps plaintext off disk
	Scopes     []string `json:"scopes"`
	Owner      string   `json:"owner,omitempty"`
	ExpiresAt  string   `json:"expiresAt,omitempty"` // RFC3339; empty = never
	Revoked    bool     `json:"revoked,omitempty"`
}

// loadKeysFile parses a keys JSON document into Keys, holding only the secret
// hashes. It fails on malformed JSON, a bad hash, an unknown scope or a bad
// timestamp so a misconfigured file is loud rather than silently open.
func loadKeysFile(data []byte) ([]*Key, error) {
	var kf keyFile
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&kf); err != nil {
		return nil, fmt.Errorf("parse keys file: %w", err)
	}
	out := make([]*Key, 0, len(kf.Keys))
	for i, e := range kf.Keys {
		key, err := e.toKey()
		if err != nil {
			return nil, fmt.Errorf("keys[%d] (kid %q): %w", i, e.Kid, err)
		}
		out = append(out, key)
	}
	return out, nil
}

func (e keyFileEntry) toKey() (*Key, error) {
	if e.Kid == "" {
		return nil, errors.New("missing kid")
	}
	var hash [32]byte
	switch {
	case e.SecretHash != "":
		h, err := hex.DecodeString(strings.TrimSpace(e.SecretHash))
		if err != nil {
			return nil, fmt.Errorf("secretHash not hex: %w", err)
		}
		if len(h) != 32 {
			return nil, fmt.Errorf("secretHash must be 32 bytes (64 hex chars), got %d", len(h))
		}
		copy(hash[:], h)
	case e.Secret != "":
		hash = sha256.Sum256([]byte(e.Secret))
	default:
		return nil, errors.New("provide secretHash (preferred) or secret")
	}
	scopes, err := parseScopes(e.Scopes)
	if err != nil {
		return nil, err
	}
	var expires time.Time
	if e.ExpiresAt != "" {
		expires, err = time.Parse(time.RFC3339, e.ExpiresAt)
		if err != nil {
			return nil, fmt.Errorf("expiresAt not RFC3339: %w", err)
		}
	}
	return &Key{Kid: e.Kid, hash: hash, Scopes: scopes, Owner: e.Owner, ExpiresAt: expires, Revoked: e.Revoked}, nil
}

func parseScopes(raw []string) ([]Scope, error) {
	if len(raw) == 0 {
		return nil, errors.New("no scopes")
	}
	out := make([]Scope, 0, len(raw))
	for _, s := range raw {
		sc := Scope(strings.TrimSpace(s))
		if !knownScopes[sc] {
			return nil, fmt.Errorf("unknown scope %q", s)
		}
		out = append(out, sc)
	}
	return out, nil
}

// bootstrapKey builds an admin key from a bootstrap secret. The kid is derived
// deterministically from the secret so it is stable across restarts yet leaks
// nothing (kid is public; sha256 is preimage-resistant). The returned kid is
// logged at startup; the secret never is.
func bootstrapKey(secret string) *Key {
	sum := sha256.Sum256([]byte(secret))
	kid := "boot-" + hex.EncodeToString(sum[:])[:12]
	return &Key{Kid: kid, hash: sum, Scopes: []Scope{ScopeAdmin}, Owner: "bootstrap admin"}
}

// buildKeystore assembles the server's keystore from static config: the keys
// file, the bootstrap admin secret and the legacy token. It returns the store
// and the bootstrap kid (empty when no bootstrap key) so the caller can log it.
// Precedence: an explicit keys-file key wins over the bootstrap key on kid
// collision (the file is the deliberate source of truth); the legacy token is
// independent (matched as a whole string).
func buildKeystore(keysFile, bootstrapSecret, legacyToken string) (ks *keystore, bootKid string, err error) {
	ks = newKeystore()
	if keysFile != "" {
		data, err := os.ReadFile(keysFile)
		if err != nil {
			return nil, "", fmt.Errorf("read keys file: %w", err)
		}
		keys, err := loadKeysFile(data)
		if err != nil {
			return nil, "", err
		}
		for _, k := range keys {
			if err := ks.add(k); err != nil {
				return nil, "", err
			}
		}
	}
	if bootstrapSecret != "" {
		bk := bootstrapKey(bootstrapSecret)
		bootKid = bk.Kid
		// A keys-file entry with the same kid takes precedence; skip rather than fail.
		if _, exists := ks.keys[bk.Kid]; !exists {
			_ = ks.add(bk)
		}
	}
	ks.setLegacyToken(legacyToken)
	return ks, bootKid, nil
}

// attachKeyStore opens the persistent managed-key store at dir, loads its keys
// into ks and wires ks to flush future mutations there (WP-103). A managed key
// whose kid collides with an existing static/bootstrap key is skipped (the
// operator-owned key wins) and reported via the returned skipped count so the
// caller can log it. It is called at construction, before serving.
func (ks *keystore) attachKeyStore(dir string) (loaded, skipped int, err error) {
	persist, err := newKeyPersist(dir)
	if err != nil {
		return 0, 0, err
	}
	keys, err := persist.load()
	if err != nil {
		return 0, 0, err
	}
	for _, k := range keys {
		if err := ks.add(k); err != nil {
			skipped++
			continue
		}
		loaded++
	}
	ks.persist = persist
	return loaded, skipped, nil
}

// --- lifecycle: managed keys (WP-103) ---

// Sentinel errors for the lifecycle API, mapped to HTTP status by the handlers.
var (
	errKeyNotFound   = errors.New("no key with that kid")
	errKeyNotManaged = errors.New("key is not server-managed (created outside the lifecycle API)")
)

// keyView is a secret-free snapshot of a key for the listing endpoint. It never
// carries the hash or any secret material.
type keyView struct {
	Kid       string  `json:"kid"`
	Scopes    []Scope `json:"scopes"`
	Owner     string  `json:"owner,omitempty"`
	ExpiresAt string  `json:"expiresAt,omitempty"` // RFC3339, empty = never
	Revoked   bool    `json:"revoked"`
	Managed   bool    `json:"managed"`
}

func viewOf(k *Key) keyView {
	v := keyView{Kid: k.Kid, Scopes: k.Scopes, Owner: k.Owner, Revoked: k.Revoked, Managed: k.managed}
	if !k.ExpiresAt.IsZero() {
		v.ExpiresAt = k.ExpiresAt.UTC().Format(time.RFC3339)
	}
	return v
}

// createKey mints a new managed key with a random kid and secret, stores only the
// secret's hash and flushes the managed set to disk. It returns the kid and the
// one-time plaintext secret — the caller shows it once and never again. The
// bearer the client uses is "<kid>.<secret>".
func (ks *keystore) createKey(scopes []Scope, owner string, expires time.Time) (kid, secret string, err error) {
	if len(scopes) == 0 {
		return "", "", errors.New("no scopes")
	}
	secret, err = randToken(24)
	if err != nil {
		return "", "", err
	}
	ks.mu.Lock()
	defer ks.mu.Unlock()
	kid, err = ks.freshKidLocked()
	if err != nil {
		return "", "", err
	}
	ks.keys[kid] = &Key{
		Kid:       kid,
		hash:      sha256.Sum256([]byte(secret)),
		Scopes:    scopes,
		Owner:     owner,
		ExpiresAt: expires,
		managed:   true,
	}
	if err := ks.saveManagedLocked(); err != nil {
		delete(ks.keys, kid) // roll back so memory matches disk
		return "", "", err
	}
	return kid, secret, nil
}

// rotateKey replaces a managed key's secret with a fresh one (invalidating the
// old secret) and persists. It returns the new one-time secret. Rotating an
// unknown or unmanaged key is an error.
func (ks *keystore) rotateKey(kid string) (secret string, err error) {
	secret, err = randToken(24)
	if err != nil {
		return "", err
	}
	ks.mu.Lock()
	defer ks.mu.Unlock()
	k, err := ks.managedLocked(kid)
	if err != nil {
		return "", err
	}
	old := k.hash
	k.hash = sha256.Sum256([]byte(secret))
	if err := ks.saveManagedLocked(); err != nil {
		k.hash = old // roll back
		return "", err
	}
	return secret, nil
}

// revokeKey marks a managed key revoked (it never authenticates again) and
// persists. Revocation marks, never deletes, so the kid stays a stable audit
// handle. Revoking an already-revoked key is idempotent.
func (ks *keystore) revokeKey(kid string) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	k, err := ks.managedLocked(kid)
	if err != nil {
		return err
	}
	if k.Revoked {
		return nil
	}
	k.Revoked = true
	if err := ks.saveManagedLocked(); err != nil {
		k.Revoked = false // roll back
		return err
	}
	return nil
}

// listKeys returns a secret-free snapshot of every key (managed and unmanaged),
// sorted by kid for a stable order.
func (ks *keystore) listKeys() []keyView {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	out := make([]keyView, 0, len(ks.keys))
	for _, k := range ks.keys {
		out = append(out, viewOf(k))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kid < out[j].Kid })
	return out
}

// managedLocked resolves kid to a managed key, distinguishing "unknown" from
// "exists but not server-managed" so the API can answer 404 vs 409. Caller holds
// the lock.
func (ks *keystore) managedLocked(kid string) (*Key, error) {
	k, ok := ks.keys[kid]
	if !ok {
		return nil, errKeyNotFound
	}
	if !k.managed {
		return nil, errKeyNotManaged
	}
	return k, nil
}

// freshKidLocked returns a random, currently-unused kid. Caller holds the lock.
func (ks *keystore) freshKidLocked() (string, error) {
	for i := 0; i < 8; i++ {
		suffix, err := randToken(6)
		if err != nil {
			return "", err
		}
		kid := "k_" + suffix
		if _, exists := ks.keys[kid]; !exists {
			return kid, nil
		}
	}
	return "", errors.New("could not allocate a unique kid")
}

// saveManagedLocked flushes every managed key to the persistent store. It is a
// no-op when no store is configured (keys then live only in memory). Caller holds
// the lock.
func (ks *keystore) saveManagedLocked() error {
	if ks.persist == nil {
		return nil
	}
	managed := make([]*Key, 0, len(ks.keys))
	for _, k := range ks.keys {
		if k.managed {
			managed = append(managed, k)
		}
	}
	sort.Slice(managed, func(i, j int) bool { return managed[i].Kid < managed[j].Kid })
	return ks.persist.save(managed)
}

// randToken returns a URL-safe random string carrying n bytes of entropy, hex
// encoded (2n chars). Used for key secrets and kid suffixes.
func randToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// --- persistent managed-key store (WP-103, ADR-0027) ---

// keyPersist is the optional filesystem home of the managed keys, a single
// atomically-written JSON file. It reuses the keys-file schema (secretHash only,
// never plaintext) so a persisted keystore is human-readable and identical in
// shape to a static -keys-file. Pure stdlib; no new dependency (no bbolt).
type keyPersist struct {
	path string
}

// keysFileName is the managed keystore's file within the -keys-dir.
const keysFileName = "keys.json"

// newKeyPersist opens (creating if needed) the key store rooted at dir. The
// directory is created 0700 because it holds credential material (secret hashes).
func newKeyPersist(dir string) (*keyPersist, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("key store: %w", err)
	}
	return &keyPersist{path: filepath.Join(dir, keysFileName)}, nil
}

// load reads the persisted managed keys. A missing file is an empty store (first
// run), not an error. Every loaded key is marked managed.
func (p *keyPersist) load() ([]*Key, error) {
	data, err := os.ReadFile(p.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read key store: %w", err)
	}
	keys, err := loadKeysFile(data)
	if err != nil {
		return nil, fmt.Errorf("key store %s: %w", p.path, err)
	}
	for _, k := range keys {
		k.managed = true
	}
	return keys, nil
}

// save writes keys to disk atomically (temp file + rename in the same directory),
// so a crash mid-write never leaves a half-written keystore. Only secret hashes
// are written, never plaintext. The file is 0600.
func (p *keyPersist) save(keys []*Key) error {
	doc := keyFile{Keys: make([]keyFileEntry, 0, len(keys))}
	for _, k := range keys {
		e := keyFileEntry{
			Kid:        k.Kid,
			SecretHash: hex.EncodeToString(k.hash[:]),
			Scopes:     scopeStrings(k.Scopes),
			Owner:      k.Owner,
			Revoked:    k.Revoked,
		}
		if !k.ExpiresAt.IsZero() {
			e.ExpiresAt = k.ExpiresAt.UTC().Format(time.RFC3339)
		}
		doc.Keys = append(doc.Keys, e)
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(p.path)
	tmp, err := os.CreateTemp(dir, ".tmp-keys-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, p.path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

func scopeStrings(scopes []Scope) []string {
	out := make([]string, len(scopes))
	for i, s := range scopes {
		out[i] = string(s)
	}
	return out
}
