package service

import (
	"crypto/subtle"
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

// requireToken wraps a handler so it is only reachable with a valid bearer
// token. When the server has no token configured the wrapper is transparent and
// the API stays open. The comparison is constant-time to avoid leaking the
// token through timing.
func (s *Server) requireToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.token != "" {
			got := bearerToken(r.Header.Get("Authorization"))
			if subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
				w.Header().Set("WWW-Authenticate", `Bearer realm="temis"`)
				writeProblem(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid bearer token")
				return
			}
		}
		next(w, r)
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
