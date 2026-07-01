package service

// Offline key administration (WP-104): a handle that operates directly on the
// persistent key store while the server is stopped, so an operator can recover
// from a lockout — no usable admin key left to reach the /v1/keys API. It reuses
// the very same keystore mutation + persistence path as the online lifecycle API,
// so a key minted offline is byte-identical to one minted over HTTP and is
// accepted by the running server on its next start. Backs the `temisd keys …` CLI.

import (
	"errors"
	"time"
)

// KeyAdmin is an offline handle to a persistent key store (a -keys-dir). It is
// not safe for concurrent use with a running server on the same directory; it is
// meant to be used while the server is stopped (lockout recovery).
type KeyAdmin struct {
	ks *keystore
}

// OpenKeyStore opens the persistent key store rooted at dir for offline
// administration, loading its managed keys. It creates the directory if needed.
func OpenKeyStore(dir string) (*KeyAdmin, error) {
	if dir == "" {
		return nil, errors.New("no key store directory (set -keys-dir or $TEMIS_KEYS_DIR)")
	}
	ks := newKeystore()
	if _, _, err := ks.attachKeyStore(dir); err != nil {
		return nil, err
	}
	return &KeyAdmin{ks: ks}, nil
}

// Create mints a new managed key with the given scopes, owner and optional expiry
// (zero = never). It returns the kid and the one-time secret; only the secret's
// hash is persisted. Scopes are validated (unknown scope → error), including
// resource-prefixed forms like "evaluate:/orders/*".
func (a *KeyAdmin) Create(scopes []string, owner string, expires time.Time) (kid, secret string, err error) {
	sc, err := parseScopes(scopes)
	if err != nil {
		return "", "", err
	}
	return a.ks.createKey(sc, owner, expires)
}

// List returns a secret-free snapshot of every key in the store.
func (a *KeyAdmin) List() []KeyView { return a.ks.listKeys() }

// Rotate issues a fresh secret for a managed key, invalidating the old one, and
// returns the new one-time secret. Rotating a non-managed or unknown key errors.
func (a *KeyAdmin) Rotate(kid string) (secret string, err error) { return a.ks.rotateKey(kid) }

// Revoke marks a managed key revoked so it never authenticates again.
func (a *KeyAdmin) Revoke(kid string) error { return a.ks.revokeKey(kid) }
