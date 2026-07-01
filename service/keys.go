package service

// Key lifecycle API (ADR-0028 Phase 2, WP-103): an admin-scoped HTTP surface to
// create, list, rotate and revoke scoped API keys, backed by the optional
// persistent keystore (-keys-dir). A created/rotated secret is returned exactly
// once and never again; only its SHA-256 is stored. The endpoints are dormant
// (404) unless a key store is configured, mirroring the disabled-listing pattern.

import (
	"errors"
	"net/http"
	"strings"
	"time"
)

// createKeyRequest is the POST /v1/keys body: the scopes to grant plus an
// optional owner label and expiry.
type createKeyRequest struct {
	Scopes    []string `json:"scopes"`
	Owner     string   `json:"owner,omitempty"`
	ExpiresAt string   `json:"expiresAt,omitempty"` // RFC3339; empty = never
}

// createdKeyResponse carries the one-time secret. bearer is the ready-to-use
// "kid.secret" credential; after this response the secret is unrecoverable.
type createdKeyResponse struct {
	Kid       string  `json:"kid"`
	Secret    string  `json:"secret"`
	Bearer    string  `json:"bearer"`
	Scopes    []Scope `json:"scopes"`
	Owner     string  `json:"owner,omitempty"`
	ExpiresAt string  `json:"expiresAt,omitempty"`
}

type listKeysResponse struct {
	Keys  []KeyView `json:"keys"`
	Count int       `json:"count"`
}

// keyMgmtEnabled reports whether the lifecycle API is active (a key store was
// configured). When false the endpoints answer 404 so they look absent.
func (s *Server) keyMgmtEnabled() bool { return s.keyStore != nil }

// handleCreateKey mints a new managed key and returns its one-time secret (201).
func (s *Server) handleCreateKey(w http.ResponseWriter, r *http.Request) {
	if !s.keyMgmtEnabled() {
		writeProblem(w, http.StatusNotFound, "NOT_FOUND", "key management is disabled")
		return
	}
	var req createKeyRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	scopes, err := parseScopes(req.Scopes)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	var expires time.Time
	if strings.TrimSpace(req.ExpiresAt) != "" {
		expires, err = time.Parse(time.RFC3339, req.ExpiresAt)
		if err != nil {
			writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", "expiresAt not RFC3339: "+err.Error())
			return
		}
	}
	kid, secret, err := s.keyStore.createKey(scopes, strings.TrimSpace(req.Owner), expires)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	resp := createdKeyResponse{
		Kid:    kid,
		Secret: secret,
		Bearer: kid + "." + secret,
		Scopes: scopes,
		Owner:  strings.TrimSpace(req.Owner),
	}
	if !expires.IsZero() {
		resp.ExpiresAt = expires.UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusCreated, resp)
}

// handleListKeys returns every key without any secret material (200).
func (s *Server) handleListKeys(w http.ResponseWriter, _ *http.Request) {
	if !s.keyMgmtEnabled() {
		writeProblem(w, http.StatusNotFound, "NOT_FOUND", "key management is disabled")
		return
	}
	views := s.keyStore.listKeys()
	writeJSON(w, http.StatusOK, listKeysResponse{Keys: views, Count: len(views)})
}

// handleRotateKey issues a fresh secret for a managed key, invalidating the old
// one, and returns the new one-time secret (200).
func (s *Server) handleRotateKey(w http.ResponseWriter, r *http.Request) {
	if !s.keyMgmtEnabled() {
		writeProblem(w, http.StatusNotFound, "NOT_FOUND", "key management is disabled")
		return
	}
	kid := r.PathValue("kid")
	secret, err := s.keyStore.rotateKey(kid)
	if err != nil {
		writeKeyError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, createdKeyResponse{Kid: kid, Secret: secret, Bearer: kid + "." + secret})
}

// handleRevokeKey marks a managed key revoked (200). Revocation marks rather than
// deletes, so the kid remains a stable audit handle.
func (s *Server) handleRevokeKey(w http.ResponseWriter, r *http.Request) {
	if !s.keyMgmtEnabled() {
		writeProblem(w, http.StatusNotFound, "NOT_FOUND", "key management is disabled")
		return
	}
	kid := r.PathValue("kid")
	if err := s.keyStore.revokeKey(kid); err != nil {
		writeKeyError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"kid": kid, "revoked": true})
}

// writeKeyError maps a lifecycle sentinel to its HTTP problem: an unknown kid is
// 404, an existing-but-unmanaged kid is 409 (can't rotate/revoke an operator key).
func writeKeyError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errKeyNotFound):
		writeProblem(w, http.StatusNotFound, "KEY_NOT_FOUND", err.Error())
	case errors.Is(err, errKeyNotManaged):
		writeProblem(w, http.StatusConflict, "KEY_NOT_MANAGED", err.Error())
	default:
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
	}
}
