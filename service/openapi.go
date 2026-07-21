package service

import (
	_ "embed"
	"net/http"
	"strings"
)

// openapiSpec is the OpenAPI 3 document describing the HTTP API. It is embedded
// at build time so the binary serves its own contract with no external files.
//
//go:embed openapi.yaml
var openapiSpec []byte

// swaggerUIPage is the self-contained Swagger UI host page. The Swagger UI
// assets themselves are loaded from a CDN (jsDelivr) so the engine keeps zero
// extra Go or vendored-asset dependencies; the page therefore needs outbound
// internet access to render. It points at the embedded spec served from
// /openapi.yaml and pre-fills the "Authorize" dialog with a bearer scheme so a
// token-protected server can be exercised straight from the browser.
const swaggerUIPage = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Temis DMN API — Swagger UI</title>
  <link rel="icon" href="data:,">
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-bundle.js" crossorigin></script>
  <script>
    window.ui = SwaggerUIBundle({
      url: '/openapi.yaml',
      dom_id: '#swagger-ui',
      deepLinking: true,
      persistAuthorization: true,
      presets: [SwaggerUIBundle.presets.apis],
      layout: 'BaseLayout',
    });
  </script>
</body>
</html>
`

// handleDocs serves the interactive Swagger UI page. It is always public so the
// API contract stays discoverable even when the data endpoints require a token.
func (s *Server) handleDocs(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(swaggerUIPage))
}

// handleOpenAPISpec serves the embedded OpenAPI document that Swagger UI loads.
func (s *Server) handleOpenAPISpec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(openapiSpec)
}

// requireScope wraps a handler so it is only reachable with a valid kid.secret
// API key that grants the given scope (ADR-0028 §1/§2). When no keys and no
// legacy token are configured the wrapper is transparent and the API stays open
// (the historical default). A missing/invalid/expired/revoked key → 401 with a
// WWW-Authenticate challenge; a valid key lacking the scope → 403 FORBIDDEN. The
// secret comparison is constant-time (see keystore.authenticate).
func (s *Server) requireScope(scope Scope, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.auth.enabled() {
			next(w, r)
			return
		}
		// Public decisions (ADR-0035): an evaluation the operator has opened stays
		// reachable without a key while auth guards everything else. Only the evaluate
		// scope can be opened this way. A caller that still presents a valid key keeps
		// its authorship (clioauthkid); a missing or invalid credential is served
		// anonymously rather than rejected — the route is public on purpose.
		if scope == ScopeEvaluate && s.evaluateIsPublic(r.PathValue("id")) {
			if key, ok := s.auth.authenticate(bearerToken(r.Header.Get("Authorization"))); ok {
				next(w, r.WithContext(withAuthKid(r.Context(), key.Kid)))
				return
			}
			next(w, r)
			return
		}
		key, ok := s.auth.authenticate(bearerToken(r.Header.Get("Authorization")))
		if !ok {
			w.Header().Set("WWW-Authenticate", `Bearer realm="temis"`)
			writeProblem(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid bearer token")
			return
		}
		// The resource a prefix-scope (WP-105) constrains against is the request's
		// model/flow id where present ("/v1/models/{id}/…"); resource-less routes
		// pass "" and only an unconstrained grant satisfies them.
		if !key.HasScopeFor(scope, r.PathValue("id")) {
			writeProblem(w, http.StatusForbidden, "FORBIDDEN", "the key lacks the required scope: "+string(scope))
			return
		}
		// Stash the authenticated kid so the audit sink can stamp authorship
		// (clioauthkid) on the decision/flow event (ADR-0023, WP-105).
		next(w, r.WithContext(withAuthKid(r.Context(), key.Kid)))
	}
}

// bearerToken extracts the credential from an "Authorization: Bearer <token>"
// header value, returning "" when the header is absent or not a bearer header.
func bearerToken(header string) string {
	const prefix = "Bearer "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}
