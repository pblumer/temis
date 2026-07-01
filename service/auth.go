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
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
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
// raw token that authenticates as a synthetic admin key. It is read-only after
// construction (Phase 1 is static config), so it needs no locking.
type keystore struct {
	keys        map[string]*Key
	legacyToken string           // raw -token / TEMIS_API_TOKEN; empty = none
	legacyKey   *Key             // the synthetic admin key the legacy token maps to
	now         func() time.Time // injectable clock for expiry (tests)
}

func newKeystore() *keystore {
	return &keystore{keys: map[string]*Key{}, now: time.Now}
}

// enabled reports whether any authentication is configured. When false the API
// is open (the historical default) and the gates are transparent.
func (ks *keystore) enabled() bool {
	return ks != nil && (len(ks.keys) > 0 || ks.legacyToken != "")
}

// add registers a key, rejecting a duplicate or empty kid.
func (ks *keystore) add(k *Key) error {
	if k.Kid == "" {
		return errors.New("key has empty kid")
	}
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
