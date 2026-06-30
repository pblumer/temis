package service

import (
	"context"

	"github.com/pblumer/temis/dmn"
	"github.com/pblumer/temis/mcp"
)

// AttachMCP co-locates an MCP server's endpoint in this HTTP server: Handler then
// mounts POST/GET /mcp alongside the web UI and the /v1 API. To share this
// server's model cache (so examples and API-loaded models appear over MCP and
// vice versa), construct the MCP server with mcp.WithStore(s.ModelStore()). Call
// before Handler; a nil argument leaves /mcp unmounted.
func (s *Server) AttachMCP(m *mcp.Server) { s.mcpServer = m }

// ModelStore exposes this server's model cache as an mcp.Store, so an MCP server
// built with mcp.WithStore(s.ModelStore()) reads and writes the very same cache —
// one process, one address space. The model id (the XML's "sha256:" hash) is
// identical across both surfaces, so a model loaded over either is found over
// both.
func (s *Server) ModelStore() mcp.Store { return mcpStore{s} }

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
