package mcp

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// HTTPHandler returns an http.Handler that serves this MCP server over the MCP
// Streamable HTTP transport, making it reachable remotely (e.g. routed behind a
// reverse proxy) rather than only as a local stdio subprocess.
//
// The transport is a single endpoint: each POST carries exactly one JSON-RPC
// message and the response is returned as application/json. This server has no
// server-initiated messages (no sampling, resources or subscriptions), so it
// offers no SSE stream — a GET on the endpoint is 405, which the spec permits
// for a server that does not stream.
//
// Routes: POST/GET /mcp (the MCP endpoint) and GET /healthz (a liveness probe
// for load balancers). When a bearer token is configured (WithHTTPToken), /mcp
// requires "Authorization: Bearer <token>".
//
// The handler is safe for concurrent requests: each call is independent and the
// shared model cache is mutex-guarded.
func (s *Server) HTTPHandler() http.Handler {
	mux := http.NewServeMux()
	s.RegisterRoutes(mux)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	return mux
}

// RegisterRoutes mounts the MCP endpoint's routes (POST/GET /mcp) on an existing
// mux, so the endpoint can be co-located in another server's process and address
// space (e.g. temisd serving the web UI, the /v1 API and /mcp on one shared model
// cache). Unlike HTTPHandler it adds no /healthz, leaving liveness to the host.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /mcp", s.handleHTTPMessage)
	mux.HandleFunc("GET /mcp", s.handleHTTPGet)
}

// handleHTTPMessage processes one JSON-RPC message from the request body. A
// request yields its JSON-RPC response (200, application/json); a notification
// yields 202 with no body, per the transport. When auth is enabled the message
// is authorized first: for a tools/call the tool's scope is required (403 when
// the key lacks it), other methods need only a valid key (401 otherwise). The
// verdict is decided from the body before dispatch, so an unauthorized call
// never reaches the tool.
func (s *Server) handleHTTPMessage(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxMessageBytes))
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if s.authEnabled() {
		bearer, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		switch s.authorize(bearer, requiredScope(body)) {
		case AuthUnauthenticated:
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "missing or invalid bearer token", http.StatusUnauthorized)
			return
		case AuthForbidden:
			http.Error(w, "the key lacks the required scope for this tool", http.StatusForbidden)
			return
		}
	}
	resp, hasResp := s.handleMessage(r.Context(), body)
	if !hasResp {
		// A notification (no id) carries no response; acknowledge receipt.
		w.WriteHeader(http.StatusAccepted)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// handleHTTPGet rejects the optional server→client SSE stream this server does
// not provide.
func (s *Server) handleHTTPGet(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Allow", "POST")
	http.Error(w, "this MCP endpoint does not offer an SSE stream", http.StatusMethodNotAllowed)
}

// authEnabled reports whether the HTTP transport enforces authentication: either
// a scoped keystore (WithAuth) or the deprecated single token (WithHTTPToken).
func (s *Server) authEnabled() bool { return s.auth != nil || s.token != "" }

// authorize decides a bearer credential against the required scope. The legacy
// token is a whole-string constant-time match that grants every tool (admin);
// otherwise the scoped keystore decides. An empty scope means "any authenticated
// key". It is only called when authEnabled() is true.
func (s *Server) authorize(bearer, scope string) AuthResult {
	if s.token != "" && subtle.ConstantTimeCompare([]byte(bearer), []byte(s.token)) == 1 {
		return AuthAllowed
	}
	if s.auth != nil {
		return s.auth.Authorize(bearer, scope)
	}
	return AuthUnauthenticated
}

// requiredScope returns the scope a JSON-RPC message needs: for a tools/call it
// is the called tool's scope (toolScopes), for every other method the empty
// scope (authentication only — initialize, tools/list, ping, notifications). A
// malformed body yields the empty scope; dispatch then reports the parse error.
func requiredScope(body []byte) string {
	var msg struct {
		Method string `json:"method"`
		Params struct {
			Name string `json:"name"`
		} `json:"params"`
	}
	if err := json.Unmarshal(body, &msg); err != nil {
		return ""
	}
	if msg.Method == "tools/call" {
		return toolScopes[msg.Params.Name]
	}
	return ""
}
