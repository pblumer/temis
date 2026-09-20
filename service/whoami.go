package service

// Access identity + public-config reads for the modeler's admin UI (ADR-0035
// follow-up, WP-107). Two small reads sit beside the discovery endpoints:
//
//   - GET /v1/whoami — public, self-inspecting: the caller learns whether auth is
//     configured and which scopes its own credential grants, so the SPA can show
//     the admin "Zugriff" section only to an admin and drive a login prompt when a
//     credential is missing.
//   - GET /v1/access/public — admin: the effective public-decision configuration
//     (ADR-0035), read-only, for the access section's Public panel.
//
// whoami is deliberately issuer-agnostic (Subject, not "kid") so an OIDC/Keycloak
// authenticator can later report an OIDC subject through the same shape without a
// contract change — the frontend only ever reads Scopes/IsAdmin.

import (
	"errors"
	"net/http"
	"strings"
)

// AccessIdentity describes the calling credential for the access UI. Subject is
// the caller's identity — a key's kid today, an OIDC subject once a Keycloak
// authenticator lands — and Scopes drives what the UI reveals.
type AccessIdentity struct {
	AuthEnabled   bool    `json:"authEnabled"`   // false = open API (dev): everyone is effectively admin
	Authenticated bool    `json:"authenticated"` // true when a valid credential was presented
	Subject       string  `json:"subject,omitempty"`
	Scopes        []Scope `json:"scopes,omitempty"`
	IsAdmin       bool    `json:"isAdmin"`
}

// handleWhoami reports the caller's own identity and scopes. It is public and
// self-inspecting: it never 401s, so the SPA can call it to decide between
// showing the workspace, prompting for a credential, or revealing the admin
// section. When auth is not configured the API is open, so the caller is reported
// as an authenticated admin (the access UI is then fully visible in a dev run).
func (s *Server) handleWhoami(w http.ResponseWriter, r *http.Request) {
	id := AccessIdentity{AuthEnabled: s.auth.enabled()}
	if !id.AuthEnabled {
		id.Authenticated = true
		id.IsAdmin = true
		writeJSON(w, http.StatusOK, id)
		return
	}
	if key, ok := s.auth.authenticate(bearerToken(r.Header.Get("Authorization"))); ok {
		id.Authenticated = true
		id.Subject = key.Kid
		id.Scopes = key.Scopes
		id.IsAdmin = key.HasScope(ScopeAdmin)
	}
	writeJSON(w, http.StatusOK, id)
}

// AccessPublicConfig is the server's effective public-decision configuration
// (ADR-0035) for the admin access UI. Evaluate (the global switch) stays startup
// config; the per-model allowlist is split into immutable Static entries
// (-public-models) and runtime-toggleable Managed ones. Persistent reports whether
// managed toggles survive a restart (an access-control dir is configured).
type AccessPublicConfig struct {
	Evaluate   bool     `json:"evaluate"`   // WithPublicEvaluate: the whole evaluate scope is anonymous
	Static     []string `json:"static"`     // -public-models: immutable at runtime
	Managed    []string `json:"managed"`    // runtime-toggled modelIds/names
	Persistent bool     `json:"persistent"` // managed toggles survive a restart (-keys-dir set)
}

// writeAccessPublic writes the current public-decision configuration (200). It is
// the shared response for the read and the toggle handlers.
func (s *Server) writeAccessPublic(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, AccessPublicConfig{
		Evaluate:   s.publicEvaluate,
		Static:     s.publicModels.staticList(),
		Managed:    s.publicModels.managedList(),
		Persistent: s.publicModels.persistent(),
	})
}

// handleAccessPublic returns the effective public-decision configuration. It is
// admin-scoped (registered under requireScope in Handler).
func (s *Server) handleAccessPublic(w http.ResponseWriter, _ *http.Request) {
	s.writeAccessPublic(w)
}

// setPublicModelRequest toggles one model's public state at runtime.
type setPublicModelRequest struct {
	Model  string `json:"model"`  // a modelId (sha256:…) or a display name
	Public bool   `json:"public"` // true = open to anonymous evaluation, false = close
}

// handleSetPublicModel opens or closes a single model's public evaluation at
// runtime (admin-scoped), persisting the managed set when a store is configured.
// Removing a static (-public-models) entry is a 409 — it is deployment config.
func (s *Server) handleSetPublicModel(w http.ResponseWriter, r *http.Request) {
	var req setPublicModelRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if strings.TrimSpace(req.Model) == "" {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", "model is required")
		return
	}
	var err error
	if req.Public {
		err = s.publicModels.add(req.Model)
	} else {
		err = s.publicModels.remove(req.Model)
	}
	if err != nil {
		if errors.Is(err, errPublicStatic) {
			writeProblem(w, http.StatusConflict, "PUBLIC_STATIC", err.Error())
			return
		}
		writeProblem(w, http.StatusInternalServerError, "PUBLIC_STORE", err.Error())
		return
	}
	s.writeAccessPublic(w)
}
