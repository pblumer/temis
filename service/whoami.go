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
	"net/http"
	"sort"
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
// (ADR-0035), read-only for the admin access UI. The toggles are startup config
// today (env/flags); the panel surfaces their state so an admin can see which
// decisions are open to anonymous callers.
type AccessPublicConfig struct {
	Evaluate bool     `json:"evaluate"` // WithPublicEvaluate: the whole evaluate scope is anonymous
	Models   []string `json:"models"`   // WithPublicModels: modelIds/names open to anonymous evaluation
}

// handleAccessPublic returns the effective public-decision configuration. It is
// admin-scoped (registered under requireScope in Handler).
func (s *Server) handleAccessPublic(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, AccessPublicConfig{Evaluate: s.publicEvaluate, Models: s.publicModelsList()})
}

// publicModelsList returns the configured public model ids/names, sorted for a
// stable UI order.
func (s *Server) publicModelsList() []string {
	out := make([]string, 0, len(s.publicModels))
	for id := range s.publicModels {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
