package service

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/pblumer/temis/dmn"
	"github.com/pblumer/temis/mcp"
)

// TestMCPStoreLookup covers mcpStore.Lookup on both a hit and a miss through the
// ModelStore adapter.
func TestMCPStoreLookup(t *testing.T) {
	srv := NewServer(nil)
	store := srv.ModelStore()

	// Miss: nothing cached yet.
	if _, _, ok := store.Lookup("sha256:deadbeef"); ok {
		t.Fatal("Lookup on empty store: ok=true, want false")
	}

	// Hit: compile through the store, then look it up.
	id, _, _, _, err := store.Compile(context.Background(), dishXML(t))
	if err != nil {
		t.Fatalf("store.Compile: %v", err)
	}
	defs, _, ok := store.Lookup(id)
	if !ok || defs == nil {
		t.Fatalf("Lookup after Compile: ok=%v defs=%v, want true/non-nil", ok, defs)
	}
}

// TestMCPStoreCompileError covers mcpStore.Compile's error branch (malformed XML).
func TestMCPStoreCompileError(t *testing.T) {
	store := NewServer(nil).ModelStore()
	if _, _, _, _, err := store.Compile(context.Background(), []byte("<not-dmn>")); err == nil {
		t.Fatal("store.Compile with bad xml: want error, got nil")
	}
}

// TestMCPStoreList covers mcpStore.List by listing a compiled model.
func TestMCPStoreList(t *testing.T) {
	store := NewServer(nil).ModelStore()
	if _, _, _, _, err := store.Compile(context.Background(), dishXML(t)); err != nil {
		t.Fatalf("store.Compile: %v", err)
	}
	list := store.List()
	if len(list) != 1 || len(list[0].Decisions) == 0 {
		t.Fatalf("List = %+v, want one model with decisions", list)
	}
}

// TestAttachMCPNil leaves /mcp unmounted when a nil server is attached (the
// guard in AttachMCP / Handler).
func TestAttachMCPNil(t *testing.T) {
	srv := NewServer(nil)
	srv.AttachMCP(nil)
	h := srv.Handler()
	// /mcp is unmounted: the SPA catch-all handles the POST, never a JSON-RPC
	// tool result.
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_models","arguments":{}}}`
	rec := do(t, h, "POST", "/mcp", "application/json", []byte(body))
	if rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), `"result"`) {
		t.Fatal("/mcp answered a JSON-RPC result though no MCP server was attached")
	}
}

// TestWithExamplesPreloadsCache covers loadExamples on the happy path: a server
// built with WithExamples lists the bundled examples.
func TestWithExamplesPreloadsCache(t *testing.T) {
	srv := NewServer(nil, WithExamples())
	if srv.cache.len() == 0 {
		t.Fatal("WithExamples loaded no models into the cache")
	}
	// Each example carries a display name (its file stem), so the no-name branch
	// of loadExamples ran.
	for _, sm := range srv.cache.snapshot() {
		if sm.name == "" {
			t.Errorf("example %s has no display name", sm.id)
		}
	}
}

// TestNewModelCacheNegativeCapacity covers newModelCache's capacity<0 clamp:
// a negative capacity becomes unbounded (0).
func TestNewModelCacheNegativeCapacity(t *testing.T) {
	c := newModelCache(-5)
	for _, id := range []string{"a", "b", "c"} {
		c.add(sm(id))
	}
	if c.len() != 3 {
		t.Errorf("len = %d, want 3 (negative capacity = unbounded)", c.len())
	}
}

// TestModelStoreSharedThroughMCPServer wires a real MCP server onto the shared
// store and exercises Compile + Lookup over it, covering the adapter's success
// paths end to end.
func TestModelStoreSharedThroughMCPServer(t *testing.T) {
	engine := dmn.New()
	srv := NewServer(engine)
	srv.AttachMCP(mcp.NewServer(engine, mcp.WithStore(srv.ModelStore())))
	h := srv.Handler()

	// Load over MCP, then confirm it is in the shared cache via the HTTP API.
	loaded := callTool(t, h, "load_model", map[string]any{"xml": string(dishXML(t))})
	id, _ := loaded["modelId"].(string)
	if id == "" {
		t.Fatal("load_model returned no modelId")
	}
	if rec := do(t, h, "GET", "/v1/models/"+id, "", nil); rec.Code != http.StatusOK {
		t.Errorf("GET shared model = %d, want 200", rec.Code)
	}
}
