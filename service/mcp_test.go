package service

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/pblumer/temis/dmn"
	"github.com/pblumer/temis/mcp"
)

// sharedServer builds a service.Server with the MCP endpoint co-located on the
// same model cache, returning the server (for cache assertions) and its handler.
func sharedServer(t *testing.T, opts ...Option) (*Server, http.Handler) {
	t.Helper()
	engine := dmn.New()
	srv := NewServer(engine, opts...)
	srv.AttachMCP(mcp.NewServer(engine, mcp.WithStore(srv.ModelStore()), mcp.WithFlowStore(srv.FlowStore())))
	return srv, srv.Handler()
}

// callTool invokes an MCP tool over POST /mcp and returns the JSON payload
// carried in the result's text content block.
func callTool(t *testing.T, h http.Handler, name string, args map[string]any) map[string]any {
	t.Helper()
	argsJSON, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` +
		name + `","arguments":` + string(argsJSON) + `}}`
	rec := do(t, h, "POST", "/mcp", "application/json", []byte(body))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /mcp %s: status %d (body %s)", name, rec.Code, rec.Body)
	}
	var resp struct {
		Result struct {
			Content []struct{ Text string } `json:"content"`
			IsError bool                    `json:"isError"`
		} `json:"result"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode JSON-RPC response %q: %v", rec.Body, err)
	}
	if resp.Error != nil {
		t.Fatalf("JSON-RPC error from %s: %s", name, resp.Error)
	}
	if len(resp.Result.Content) == 0 {
		t.Fatalf("%s: empty result content (body %s)", name, rec.Body)
	}
	if resp.Result.IsError {
		t.Fatalf("%s tool error: %s", name, resp.Result.Content[0].Text)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(resp.Result.Content[0].Text), &payload); err != nil {
		t.Fatalf("decode %s payload %q: %v", name, resp.Result.Content[0].Text, err)
	}
	return payload
}

// mcpModelIDs returns the set of model ids that MCP list_models reports.
func mcpModelIDs(t *testing.T, h http.Handler) map[string]bool {
	t.Helper()
	payload := callTool(t, h, "list_models", map[string]any{})
	ids := map[string]bool{}
	for _, m := range payload["models"].([]any) {
		ids[m.(map[string]any)["modelId"].(string)] = true
	}
	return ids
}

// TestMCPSeesPreloadedExamples proves the co-located MCP endpoint reads the same
// cache the modeler does: the bundled examples loaded at construction are listed
// over MCP, with the same count as GET /v1/models.
func TestMCPSeesPreloadedExamples(t *testing.T) {
	_, h := sharedServer(t, WithExamples())

	apiList := decode[modelListResponse](t, do(t, h, "GET", "/v1/models", "", nil))
	if apiList.Count == 0 {
		t.Fatal("expected preloaded examples in the API listing, got none")
	}
	mcpIDs := mcpModelIDs(t, h)
	if len(mcpIDs) != apiList.Count {
		t.Fatalf("MCP lists %d models, API lists %d — caches not shared", len(mcpIDs), apiList.Count)
	}
	for _, m := range apiList.Models {
		if !mcpIDs[m.ModelID] {
			t.Errorf("example %q (%s) missing from MCP list_models", m.Name, m.ModelID)
		}
	}
}

// TestSharedCacheCrossVisibility proves the shared cache works both ways: a model
// uploaded over the /v1 API is visible over MCP, and a model loaded over MCP is
// visible over the /v1 API — all in one address space.
func TestSharedCacheCrossVisibility(t *testing.T) {
	_, h := sharedServer(t) // no examples: start from an empty cache

	// API → MCP: upload over /v1/models, then find it via MCP list_models.
	apiResp := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", dishXML(t)))
	if apiResp.ModelID == "" {
		t.Fatal("upload returned no model id")
	}
	if !mcpModelIDs(t, h)[apiResp.ModelID] {
		t.Errorf("API-uploaded model %s not visible over MCP", apiResp.ModelID)
	}

	// MCP → API: load a different model over MCP, then find it via GET /v1/models.
	routingXML, err := os.ReadFile("../dmn/testdata/models/routing_13.dmn")
	if err != nil {
		t.Fatalf("read routing model: %v", err)
	}
	loaded := callTool(t, h, "load_model", map[string]any{"xml": string(routingXML)})
	mcpID, _ := loaded["modelId"].(string)
	if mcpID == "" {
		t.Fatal("load_model returned no modelId")
	}
	if mcpID == apiResp.ModelID {
		t.Fatal("test models are not distinct; cross-visibility check is meaningless")
	}
	apiList := decode[modelListResponse](t, do(t, h, "GET", "/v1/models", "", nil))
	found := false
	for _, m := range apiList.Models {
		if m.ModelID == mcpID {
			found = true
		}
	}
	if !found {
		t.Errorf("MCP-loaded model %s not visible over GET /v1/models", mcpID)
	}
}

// TestSharedFlowCatalogCrossVisibility proves the flow catalog is shared like the
// model cache: a flow registered over MCP load_flow appears in GET /v1/flows (so
// the modeler lists it), and a flow registered over POST /v1/flows is described by
// MCP describe_flow — one catalog, both surfaces.
func TestSharedFlowCatalogCrossVisibility(t *testing.T) {
	_, h := sharedServer(t)
	// Load the referenced models into the shared cache first, so the flow validates
	// without diagnostics regardless of which surface registers it.
	riskID := uploadModel(t, h, "../flow/testdata/risk.dmn")
	loanID := uploadModel(t, h, "../flow/testdata/loan.dmn")
	desc := loanFlowDescriptor(riskID, loanID)

	// MCP → API: register over load_flow, then find it via GET /v1/flows.
	var flowArg map[string]any
	if err := json.Unmarshal(desc, &flowArg); err != nil {
		t.Fatalf("unmarshal descriptor: %v", err)
	}
	loaded := callTool(t, h, "load_flow", map[string]any{"flow": flowArg})
	flowID, _ := loaded["flowId"].(string)
	if flowID == "" {
		t.Fatal("load_flow returned no flowId")
	}
	list := decode[flowListResponse](t, do(t, h, "GET", "/v1/flows", "", nil))
	found := false
	for _, f := range list.Flows {
		if f.FlowID == flowID {
			found = true
			if f.Name != "loan-decisioning" || f.Steps != 2 {
				t.Errorf("catalog summary = %+v", f)
			}
		}
	}
	if !found {
		t.Fatalf("MCP-registered flow %s not visible over GET /v1/flows — catalog not shared", flowID)
	}

	// The same id resolves over both surfaces (content-addressed): GET /v1/flows/{id}
	// and MCP describe_flow both describe it.
	if d := decode[flowDetail](t, do(t, h, "GET", "/v1/flows/"+flowID, "", nil)); len(d.Steps) != 2 {
		t.Errorf("GET /v1/flows/%s = %+v", flowID, d)
	}
	desc2 := callTool(t, h, "describe_flow", map[string]any{"flowId": flowID})
	if desc2["name"] != "loan-decisioning" {
		t.Errorf("describe_flow = %+v", desc2)
	}
}

// TestMCPEndpointAbsentWhenNotAttached confirms /mcp is only mounted when an MCP
// server is attached, so the default service keeps its surface unchanged.
func TestMCPEndpointAbsentWhenNotAttached(t *testing.T) {
	h := NewServer(nil).Handler()
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_models","arguments":{}}}`
	rec := do(t, h, "POST", "/mcp", "application/json", []byte(body))
	// Unmounted: the SPA catch-all handles it, so it is anything but a 200 MCP
	// JSON-RPC result. Assert we did not get a tool result back.
	if rec.Code == http.StatusOK && len(rec.Body.Bytes()) > 0 {
		var resp struct {
			Result json.RawMessage `json:"result"`
		}
		if json.Unmarshal(rec.Body.Bytes(), &resp) == nil && resp.Result != nil {
			t.Fatalf("/mcp answered a JSON-RPC result though no MCP server was attached")
		}
	}
}
