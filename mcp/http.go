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
// yields 202 with no body, per the transport.
func (s *Server) handleHTTPMessage(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "missing or invalid bearer token", http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxMessageBytes))
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
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

// authorized reports whether the request may use the endpoint: always true when
// no token is configured, otherwise it must present the matching bearer token
// (compared in constant time).
func (s *Server) authorized(r *http.Request) bool {
	if s.token == "" {
		return true
	}
	got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) == 1
}
