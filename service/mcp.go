package service

import (
	"context"

	"github.com/pblumer/temis/dmn"
	"github.com/pblumer/temis/flow"
	"github.com/pblumer/temis/mcp"
)

// AttachMCP co-locates an MCP server's endpoint in this HTTP server: Handler then
// mounts POST/GET /mcp alongside the web UI and the /v1 API. To share this
// server's model cache (so examples and API-loaded models appear over MCP and
// vice versa), construct the MCP server with mcp.WithStore(s.ModelStore()); to
// likewise share the flow catalog (so a flow registered over MCP appears in the
// modeler and vice versa), add mcp.WithFlowStore(s.FlowStore()). Call before
// Handler; a nil argument leaves /mcp unmounted.
func (s *Server) AttachMCP(m *mcp.Server) { s.mcpServer = m }

// ModelStore exposes this server's model cache as an mcp.Store, so an MCP server
// built with mcp.WithStore(s.ModelStore()) reads and writes the very same cache —
// one process, one address space. The model id (the XML's "sha256:" hash) is
// identical across both surfaces, so a model loaded over either is found over
// both.
func (s *Server) ModelStore() mcp.Store { return mcpStore{s} }

// FlowStore exposes this server's flow catalog as an mcp.FlowStore, so an MCP
// server built with mcp.WithFlowStore(s.FlowStore()) registers flows into the very
// same catalog the /v1 API and the modeler read — one process, one catalog. The
// flowId (the descriptor's "sha256:" hash) is identical across both surfaces, so a
// flow registered over either is found over both. Unlike the model store, the flow
// catalog is not written to disk here (ADR-0032: the flows directory stays the
// durable source of truth); MCP-registered flows live in memory like POST /v1/flows.
func (s *Server) FlowStore() mcp.FlowStore { return mcpFlowStore{s} }

// mcpFlowStore adapts the service's flow catalog to the mcp.FlowStore interface,
// writing into the same flowStore the HTTP handlers use. Put mirrors
// handleCreateFlow: it validates the flow against the currently loaded models and
// stores it with those diagnostics, so the modeler lists it via GET /v1/flows.
type mcpFlowStore struct{ s *Server }

func (a mcpFlowStore) Put(ctx context.Context, id string, f *flow.Flow, desc []byte) {
	a.s.flows.put(&storedFlow{id: id, flow: f, desc: desc, diags: f.Validate(ctx, cacheResolver{a.s})})
}

func (a mcpFlowStore) Get(id string) (*flow.Flow, []byte, bool) {
	sf, ok := a.s.flows.get(id)
	if !ok {
		return nil, nil, false
	}
	return sf.flow, sf.desc, true
}

// MCPAuth exposes this server's keystore as an mcp.Auth gate, so a co-located MCP
// endpoint enforces the same scoped keys (ADR-0028) as the /v1 surface — one
// keystore, one process. It returns nil when no authentication is configured, so
// mcp.WithAuth leaves the endpoint open (the historical default).
func (s *Server) MCPAuth() mcp.Auth {
	if !s.auth.enabled() {
		return nil
	}
	return mcpAuth{s.auth}
}

// mcpAuth adapts the service keystore to mcp.Auth: it authenticates the bearer
// and checks the tool's scope, mapping the outcome to mcp's verdict enum.
type mcpAuth struct{ ks Authenticator }

func (a mcpAuth) Authorize(bearer, scope string) mcp.AuthResult {
	key, ok := a.ks.authenticate(bearer)
	if !ok {
		return mcp.AuthUnauthenticated
	}
	// An empty scope (discovery messages) needs only a valid key.
	if scope != "" && !key.HasScope(Scope(scope)) {
		return mcp.AuthForbidden
	}
	return mcp.AuthAllowed
}

// mcpStore adapts the service's content-addressed cache to the mcp.Store
// interface, delegating to the same compileAndStore/lookup/snapshot the HTTP
// handlers use.
type mcpStore struct{ s *Server }

func (a mcpStore) Compile(ctx context.Context, xml []byte) (string, *dmn.Definitions, dmn.ModelIndex, dmn.Diagnostics, error) {
	sm, err := a.s.compileAndStore(ctx, xml)
	if err != nil {
		return "", nil, dmn.ModelIndex{}, nil, err
	}
	return sm.id, sm.defs, sm.index, sm.diags, nil
}

func (a mcpStore) Lookup(id string) (*dmn.Definitions, dmn.ModelIndex, bool) {
	sm, ok := a.s.lookup(id)
	if !ok {
		return nil, dmn.ModelIndex{}, false
	}
	return sm.defs, sm.index, true
}

func (a mcpStore) List() []mcp.ModelInfo {
	models := a.s.cache.snapshot()
	out := make([]mcp.ModelInfo, 0, len(models))
	for _, sm := range models {
		out = append(out, mcp.ModelInfo{ID: sm.id, Decisions: sm.index.Decisions, Inputs: sm.index.Inputs})
	}
	return out
}
