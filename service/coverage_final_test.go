package service

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// doCtx dispatches a request whose context is the caller's, so a test can drive a
// handler with an already-cancelled context. Path values are still populated by
// the ServeMux from the pattern, independent of the context we inject.
func doCtx(t *testing.T, h http.Handler, ctx context.Context, method, path, contentType string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body)).WithContext(ctx)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// cancelled returns a context that is already done, so the next engine.Compile /
// EvaluateGraph call short-circuits on ctx.Err().
func cancelled() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// wantMalformed asserts a 400 MALFORMED_XML problem — the shape every mutating
// endpoint produces when the (successful) patch cannot be recompiled+stored.
func wantMalformed(t *testing.T, rec *httptest.ResponseRecorder, endpoint string) {
	t.Helper()
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("%s with cancelled ctx = %d, want 400 (body %s)", endpoint, rec.Code, rec.Body)
	}
	if p := decode[problem](t, rec); p.Code != "MALFORMED_XML" {
		t.Errorf("%s code = %q, want MALFORMED_XML", endpoint, p.Code)
	}
}

// TestRespondSavedCompileError drives respondSaved's compile-and-store failure
// tail: a valid type edit patches the XML, but a cancelled context makes the
// recompile fail, so the shared save tail answers 400 MALFORMED_XML.
func TestRespondSavedCompileError(t *testing.T) {
	h := newTestServer(t)
	id := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", dishXML(t))).ModelID

	body := mustJSON(t, dmn.ItemType{Name: "Color", TypeRef: "string", AllowedValues: `"red","green"`})
	rec := doCtx(t, h, cancelled(), "POST", "/v1/models/"+id+"/types", "application/json", body)
	wantMalformed(t, rec, "POST /types")
}

// TestSaveLiteralCompileError covers handleSaveLiteral's compile-error branch.
func TestSaveLiteralCompileError(t *testing.T) {
	h := newTestServer(t)
	xml := readFile(t, "../dmn/testdata/models/pricing_15.dmn")
	id := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", xml)).ModelID

	body := mustJSON(t, saveLiteralRequest{Text: "Unit Price * Quantity * 3", TypeRef: "number"})
	rec := doCtx(t, h, cancelled(), "POST", "/v1/models/"+id+"/decisions/id_net/literal", "application/json", body)
	wantMalformed(t, rec, "POST /literal")
}

// TestSaveDecisionTableCompileError covers handleSaveDecisionTable's compile-error
// branch (patch succeeds, recompile fails under a cancelled context).
func TestSaveDecisionTableCompileError(t *testing.T) {
	h := newTestServer(t)
	id := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", dishXML(t))).ModelID

	tv := decode[dmn.TableView](t, do(t, h, "GET", "/v1/models/"+id+"/decisions/Dish/table", "", nil))
	rules := make([]dmn.TableRule, len(tv.Rules))
	for i, r := range tv.Rules {
		rules[i] = dmn.TableRule{InputEntries: r.InputEntries, OutputEntries: r.OutputEntries, Annotations: r.Annotations}
		if r.OutputEntries[0] == `"Roastbeef"` {
			rules[i].OutputEntries = []string{`"Lobster"`}
		}
	}
	body := mustJSON(t, dmn.TableEdit{Rules: rules})
	rec := doCtx(t, h, cancelled(), "POST", "/v1/models/"+id+"/decisions/"+tv.DecisionID+"/table", "application/json", body)
	wantMalformed(t, rec, "POST /table")
}

// TestCreateDecisionTableCompileError covers handleCreateDecisionTable's
// compile-error branch: an undecided decision gets a table (patch OK), but the
// recompile under a cancelled context fails.
func TestCreateDecisionTableCompileError(t *testing.T) {
	h := newTestServer(t)
	id := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", dishXML(t))).ModelID

	// Add an undecided "Pairing" decision requiring Season, so create-table has a
	// decision to act on.
	g := decode[dmn.Graph](t, do(t, h, "GET", "/v1/models/"+id+"/graph", "", nil))
	edit := dmn.GraphEdit{}
	for _, n := range g.Nodes {
		edit.Nodes = append(edit.Nodes, dmn.GraphNodeEdit{ID: n.ID, Type: n.Type, Name: n.Name, DataType: n.DataType, X: n.X, Y: n.Y, Width: n.Width, Height: n.Height})
	}
	for _, e := range g.Edges {
		edit.Edges = append(edit.Edges, dmn.GraphEdgeEdit(e))
	}
	edit.Nodes = append(edit.Nodes, dmn.GraphNodeEdit{ID: "id_pairing", Type: "decision", Name: "Pairing", X: 600, Y: 260, Width: 150, Height: 70})
	edit.Edges = append(edit.Edges, dmn.GraphEdgeEdit{Type: "informationRequirement", Source: "id_season", Target: "id_pairing"})
	withDec := decode[modelResponse](t, do(t, h, "POST", "/v1/models/"+id+"/graph", "application/json", mustJSON(t, edit))).ModelID

	rec := doCtx(t, h, cancelled(), "POST", "/v1/models/"+withDec+"/decisions/id_pairing/create-table", "", nil)
	wantMalformed(t, rec, "POST /create-table")
}

// TestSaveGraphUnknownNodeType covers handleSaveGraph's ApplyGraph-error branch:
// a node with an unrecognised type is rejected before any recompile.
func TestSaveGraphUnknownNodeType(t *testing.T) {
	h := newTestServer(t)
	id := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", dishXML(t))).ModelID

	edit := dmn.GraphEdit{Nodes: []dmn.GraphNodeEdit{{ID: "id_bogus", Type: "sasquatch", Name: "Bogus"}}}
	rec := do(t, h, "POST", "/v1/models/"+id+"/graph", "application/json", mustJSON(t, edit))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /graph unknown type = %d, want 400 (body %s)", rec.Code, rec.Body)
	}
	if p := decode[problem](t, rec); p.Code != "MALFORMED_XML" {
		t.Errorf("code = %q, want MALFORMED_XML", p.Code)
	}
}

// TestSaveGraphCompileError covers handleSaveGraph's compile-error branch: a valid
// structural edit patches the XML, but the recompile fails under a cancelled ctx.
func TestSaveGraphCompileError(t *testing.T) {
	h := newTestServer(t)
	id := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", dishXML(t))).ModelID

	g := decode[dmn.Graph](t, do(t, h, "GET", "/v1/models/"+id+"/graph", "", nil))
	edit := dmn.GraphEdit{}
	for _, n := range g.Nodes {
		edit.Nodes = append(edit.Nodes, dmn.GraphNodeEdit{ID: n.ID, Type: n.Type, Name: n.Name, DataType: n.DataType, X: n.X, Y: n.Y, Width: n.Width, Height: n.Height})
	}
	for _, e := range g.Edges {
		edit.Edges = append(edit.Edges, dmn.GraphEdgeEdit(e))
	}
	// A fresh leaf input changes the content hash so this is not a cache hit.
	edit.Nodes = append(edit.Nodes, dmn.GraphNodeEdit{ID: "id_wine", Type: "inputData", Name: "Wine", DataType: "string", X: 600, Y: 100, Width: 120, Height: 50})
	edit.Edges = append(edit.Edges, dmn.GraphEdgeEdit{Type: "informationRequirement", Source: "id_wine", Target: "id_dish"})

	rec := doCtx(t, h, cancelled(), "POST", "/v1/models/"+id+"/graph", "application/json", mustJSON(t, edit))
	wantMalformed(t, rec, "POST /graph")
}

// TestSaveModelCompileError covers handleSaveModel's compile-error branch: a valid
// rename patches the XML, but the recompile fails under a cancelled context.
func TestSaveModelCompileError(t *testing.T) {
	h := newTestServer(t)
	id := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", dishXML(t))).ModelID

	g := decode[dmn.Graph](t, do(t, h, "GET", "/v1/models/"+id+"/graph", "", nil))
	var dish dmn.GraphNode
	for _, n := range g.Nodes {
		if n.Name == "Dish" {
			dish = n
		}
	}
	if dish.ID == "" {
		t.Fatalf("graph lacks 'Dish' decision: %+v", g.Nodes)
	}
	body := mustJSON(t, saveModelRequest{Nodes: []dmn.NodeEdit{{ID: dish.ID, Name: strPtr("Dish Choice")}}})
	rec := doCtx(t, h, cancelled(), "POST", "/v1/models/"+id+"/save", "application/json", body)
	wantMalformed(t, rec, "POST /save")
}

// Note: handleEvaluateGraph's generic EVALUATION_FAILED branch (http.go:815) is
// unreachable — Definitions.EvaluateGraph only ever returns an *InputError (the
// strict-validation path); per-decision runtime failures are recorded in
// GraphResult.Errors and returned with a nil error, so the non-InputError arm is
// defensive code that no input can trigger.

// TestClioSinkWriteEncodeError covers ClioSink.write's json.Marshal failure: an
// output value that cannot be encoded surfaces as an "encode event" error.
func TestClioSinkWriteEncodeError(t *testing.T) {
	sink, err := NewClioSink(ClioConfig{URL: "http://clio.invalid"})
	if err != nil {
		t.Fatalf("NewClioSink: %v", err)
	}
	rec := DecisionRecord{
		ModelID:  "sha256:abc",
		Decision: "Dish",
		Input:    map[string]any{"Season": "Fall"},
		Outputs:  map[string]any{"bad": make(chan int)}, // channels are not JSON-encodable
	}
	if _, err := sink.write(context.Background(), rec); err == nil {
		t.Fatal("write with unencodable output: want error, got nil")
	}
}

// TestClioSinkWriteBuildRequestError covers ClioSink.write's request-build failure:
// a base URL with a control character cannot form a valid *http.Request.
func TestClioSinkWriteBuildRequestError(t *testing.T) {
	sink, err := NewClioSink(ClioConfig{URL: "http://clio.invalid"})
	if err != nil {
		t.Fatalf("NewClioSink: %v", err)
	}
	sink.baseURL = "http://clio.invalid/\x7f" // DEL byte makes url parsing fail
	rec := DecisionRecord{ModelID: "sha256:abc", Decision: "Dish", Input: map[string]any{"Season": "Fall"}}
	if _, err := sink.write(context.Background(), rec); err == nil {
		t.Fatal("write with invalid base URL: want error, got nil")
	}
}
