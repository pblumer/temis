package service

import (
	"net/http"
	"testing"

	"github.com/pblumer/temis/dmn"
)

const badID = "sha256:deadbeef"

// TestHandlerUnknownModel404 sweeps every cached-model handler with an id that is
// not in the cache, asserting each returns 404 MODEL_NOT_FOUND. This exercises
// the lookup-miss branch shared by the read, edit and evaluate handlers.
func TestHandlerUnknownModel404(t *testing.T) {
	h := newTestServer(t)

	cases := []struct {
		name, method, url string
	}{
		{"get model", "GET", "/v1/models/" + badID},
		{"get xml", "GET", "/v1/models/" + badID + "/xml"},
		{"get graph", "GET", "/v1/models/" + badID + "/graph"},
		{"save graph", "POST", "/v1/models/" + badID + "/graph"},
		{"get types", "GET", "/v1/models/" + badID + "/types"},
		{"save type", "POST", "/v1/models/" + badID + "/types"},
		{"delete type", "DELETE", "/v1/models/" + badID + "/types/Color"},
		{"get table", "GET", "/v1/models/" + badID + "/decisions/Dish/table"},
		{"save table", "POST", "/v1/models/" + badID + "/decisions/Dish/table"},
		{"create table", "POST", "/v1/models/" + badID + "/decisions/Dish/create-table"},
		{"get literal", "GET", "/v1/models/" + badID + "/decisions/Dish/literal"},
		{"save literal", "POST", "/v1/models/" + badID + "/decisions/Dish/literal"},
		{"get context", "GET", "/v1/models/" + badID + "/decisions/Dish/context"},
		{"save context", "POST", "/v1/models/" + badID + "/decisions/Dish/context"},
		{"create context", "POST", "/v1/models/" + badID + "/decisions/Dish/create-context"},
		{"get bkm", "GET", "/v1/models/" + badID + "/bkm/id_rate"},
		{"save bkm", "POST", "/v1/models/" + badID + "/bkm/id_rate"},
		{"save model", "POST", "/v1/models/" + badID + "/save"},
		{"evaluate", "POST", "/v1/models/" + badID + "/evaluate"},
		{"evaluate graph", "POST", "/v1/models/" + badID + "/evaluate-graph"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := do(t, h, c.method, c.url, "application/json", []byte("{}"))
			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404 (body %s)", rec.Code, rec.Body)
			}
			if p := decode[problem](t, rec); p.Code != "MODEL_NOT_FOUND" {
				t.Errorf("code = %q, want MODEL_NOT_FOUND", p.Code)
			}
		})
	}
}

// TestHandlerBadJSON400 sweeps every handler that decodes a JSON body with a
// malformed body, asserting each returns 400 INVALID_REQUEST. The model must
// exist so the lookup passes and the decode is reached.
func TestHandlerBadJSON400(t *testing.T) {
	h := newTestServer(t)
	id := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", dishXML(t))).ModelID

	bad := []byte("{not json")
	cases := []struct {
		name, method, url string
	}{
		{"save graph", "POST", "/v1/models/" + id + "/graph"},
		{"save type", "POST", "/v1/models/" + id + "/types"},
		{"save table", "POST", "/v1/models/" + id + "/decisions/Dish/table"},
		{"save literal", "POST", "/v1/models/" + id + "/decisions/Dish/literal"},
		{"save context", "POST", "/v1/models/" + id + "/decisions/Dish/context"},
		{"save bkm", "POST", "/v1/models/" + id + "/bkm/x"},
		{"save model", "POST", "/v1/models/" + id + "/save"},
		{"evaluate", "POST", "/v1/models/" + id + "/evaluate"},
		{"evaluate graph", "POST", "/v1/models/" + id + "/evaluate-graph"},
		{"evaluate stateless", "POST", "/v1/evaluate"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := do(t, h, c.method, c.url, "application/json", bad)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body)
			}
			if p := decode[problem](t, rec); p.Code != "INVALID_REQUEST" {
				t.Errorf("code = %q, want INVALID_REQUEST", p.Code)
			}
		})
	}
}

// TestSaveTypeFails400 makes SetItemDefinition reject the edit (a structured
// type is not a simple item definition), covering the TYPE_SAVE_FAILED branch.
func TestSaveTypeFails400(t *testing.T) {
	h := newTestServer(t)
	id := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", dishXML(t))).ModelID

	// An empty type name is rejected by SetItemDefinition.
	body := mustJSON(t, dmn.ItemType{Name: "", TypeRef: "string"})
	rec := do(t, h, "POST", "/v1/models/"+id+"/types", "application/json", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body)
	}
	if p := decode[problem](t, rec); p.Code != "TYPE_SAVE_FAILED" {
		t.Errorf("code = %q, want TYPE_SAVE_FAILED", p.Code)
	}
}

// TestDeleteTypeNotFound404 removes a type that does not exist, covering the
// TYPE_NOT_FOUND branch of handleDeleteType.
func TestDeleteTypeNotFound404(t *testing.T) {
	h := newTestServer(t)
	id := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", dishXML(t))).ModelID

	rec := do(t, h, "DELETE", "/v1/models/"+id+"/types/Nonexistent", "", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body %s)", rec.Code, rec.Body)
	}
	if p := decode[problem](t, rec); p.Code != "TYPE_NOT_FOUND" {
		t.Errorf("code = %q, want TYPE_NOT_FOUND", p.Code)
	}
}

// TestSaveContextFails400 edits a context on a decision whose logic is not a
// boxed context, covering the CONTEXT_SAVE_FAILED branch.
func TestSaveContextFails400(t *testing.T) {
	h := newTestServer(t)
	id := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", dishXML(t))).ModelID

	body := mustJSON(t, dmn.ContextEdit{
		Entries: []dmn.ContextEntryView{{Name: "X", Text: "1", TypeRef: "number"}},
		Result:  "X",
	})
	rec := do(t, h, "POST", "/v1/models/"+id+"/decisions/Nope/context", "application/json", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body)
	}
	if p := decode[problem](t, rec); p.Code != "CONTEXT_SAVE_FAILED" {
		t.Errorf("code = %q, want CONTEXT_SAVE_FAILED", p.Code)
	}
}

// TestCreateContextFails400 creates a context for an unknown decision, covering
// the CONTEXT_CREATE_FAILED branch.
func TestCreateContextFails400(t *testing.T) {
	h := newTestServer(t)
	id := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", dishXML(t))).ModelID

	rec := do(t, h, "POST", "/v1/models/"+id+"/decisions/Nope/create-context", "application/json", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body)
	}
	if p := decode[problem](t, rec); p.Code != "CONTEXT_CREATE_FAILED" {
		t.Errorf("code = %q, want CONTEXT_CREATE_FAILED", p.Code)
	}
}

// TestCreateContextSucceeds gives an undecided decision a fresh boxed context,
// covering the success path of handleCreateContext.
func TestCreateContextSucceeds(t *testing.T) {
	h := newTestServer(t)
	id := deployFile(t, h, contextModelPath)

	// Add an undecided decision so create-context has somewhere to land.
	g := decode[dmn.Graph](t, do(t, h, "GET", "/v1/models/"+id+"/graph", "", nil))
	edit := dmn.GraphEdit{}
	for _, n := range g.Nodes {
		edit.Nodes = append(edit.Nodes, dmn.GraphNodeEdit{ID: n.ID, Type: n.Type, Name: n.Name, DataType: n.DataType, X: n.X, Y: n.Y, Width: n.Width, Height: n.Height})
	}
	for _, e := range g.Edges {
		edit.Edges = append(edit.Edges, dmn.GraphEdgeEdit(e))
	}
	edit.Nodes = append(edit.Nodes, dmn.GraphNodeEdit{ID: "id_fresh", Type: "decision", Name: "Fresh", X: 400, Y: 400, Width: 150, Height: 70})
	withDec := decode[modelResponse](t, do(t, h, "POST", "/v1/models/"+id+"/graph", "application/json", mustJSON(t, edit))).ModelID

	rec := do(t, h, "POST", "/v1/models/"+withDec+"/decisions/id_fresh/create-context", "application/json", nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create-context = %d, want 201 (body %s)", rec.Code, rec.Body)
	}
}

// TestSaveBKMFails400 edits a BKM function on a non-existent BKM id, covering
// the BKM_SAVE_FAILED branch.
func TestSaveBKMFails400(t *testing.T) {
	h := newTestServer(t)
	xml := readFile(t, "../dmn/testdata/models/bkm_invocation_15.dmn")
	id := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", xml)).ModelID

	body := mustJSON(t, dmn.BKMFunctionEdit{Params: []dmn.BKMParam{{Name: "x", TypeRef: "number"}}, BodyText: "x", BodyTypeRef: "number"})
	rec := do(t, h, "POST", "/v1/models/"+id+"/bkm/nonexistent", "application/json", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body)
	}
	if p := decode[problem](t, rec); p.Code != "BKM_SAVE_FAILED" {
		t.Errorf("code = %q, want BKM_SAVE_FAILED", p.Code)
	}
}

// TestSaveLiteralFails400 sets a literal on an unknown decision, covering the
// LITERAL_SAVE_FAILED branch.
func TestSaveLiteralFails400(t *testing.T) {
	h := newTestServer(t)
	id := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", dishXML(t))).ModelID

	body := mustJSON(t, saveLiteralRequest{Text: "1 + 1", TypeRef: "number"})
	rec := do(t, h, "POST", "/v1/models/"+id+"/decisions/Nope/literal", "application/json", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body)
	}
	if p := decode[problem](t, rec); p.Code != "LITERAL_SAVE_FAILED" {
		t.Errorf("code = %q, want LITERAL_SAVE_FAILED", p.Code)
	}
}

// TestCreateDecisionTableFails400 creates a table for an unknown decision,
// covering the TABLE_CREATE_FAILED branch.
func TestCreateDecisionTableFails400(t *testing.T) {
	h := newTestServer(t)
	id := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", dishXML(t))).ModelID

	rec := do(t, h, "POST", "/v1/models/"+id+"/decisions/Nope/create-table", "", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body)
	}
	if p := decode[problem](t, rec); p.Code != "TABLE_CREATE_FAILED" {
		t.Errorf("code = %q, want TABLE_CREATE_FAILED", p.Code)
	}
}

// TestEvaluateStatelessMissingXML covers the "missing xml" branch of
// handleEvaluateStateless (a body that decodes but has no xml).
func TestEvaluateStatelessMissingXML(t *testing.T) {
	h := newTestServer(t)
	body := mustJSON(t, evaluateStatelessRequest{Decision: "Dish"})
	rec := do(t, h, "POST", "/v1/evaluate", "application/json", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body)
	}
	if p := decode[problem](t, rec); p.Code != "INVALID_REQUEST" {
		t.Errorf("code = %q, want INVALID_REQUEST", p.Code)
	}
}

// TestEvaluateStatelessMalformedXML covers the MALFORMED_XML branch of
// handleEvaluateStateless (a non-empty body that does not compile).
func TestEvaluateStatelessMalformedXML(t *testing.T) {
	h := newTestServer(t)
	body := mustJSON(t, evaluateStatelessRequest{XML: "<not-dmn>", Decision: "Dish"})
	rec := do(t, h, "POST", "/v1/evaluate", "application/json", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body)
	}
	if p := decode[problem](t, rec); p.Code != "MALFORMED_XML" {
		t.Errorf("code = %q, want MALFORMED_XML", p.Code)
	}
}

// TestEvaluateGraphWithExplain covers handleEvaluateGraph's explain branch (trace
// option), exercising the success path with traces and per-decision values.
func TestEvaluateGraphWithExplain(t *testing.T) {
	h := newTestServer(t)
	xml := readFile(t, "../dmn/testdata/models/routing_13.dmn")
	id := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", xml)).ModelID

	body := mustJSON(t, evaluateGraphRequest{Input: map[string]any{"Applicant Age": 20}, Explain: true})
	rec := do(t, h, "POST", "/v1/models/"+id+"/evaluate-graph", "application/json", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
	res := decode[evaluateGraphResponse](t, rec)
	if res.Values["Routing"] != "ACCEPT" {
		t.Errorf("Routing = %v, want ACCEPT", res.Values["Routing"])
	}
}

// TestEvaluateStrictInputErrorByID drives handleEvaluateModel's InvalidInput
// (INVALID_INPUT) branch through the by-id evaluate route.
func TestEvaluateModelStrictInputError(t *testing.T) {
	h := newTestServer(t)
	id := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", dishXML(t))).ModelID

	body := mustJSON(t, evaluateModelRequest{
		Decision: "Dish",
		Input:    map[string]any{"Season": "Winter", "Guest Count": "8"},
		Strict:   true,
	})
	rec := do(t, h, "POST", "/v1/models/"+id+"/evaluate", "application/json", body)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 (body %s)", rec.Code, rec.Body)
	}
	if p := decode[problem](t, rec); p.Code != "INVALID_INPUT" {
		t.Errorf("code = %q, want INVALID_INPUT", p.Code)
	}
}

// TestEvaluateModelWithExplainTrace covers the explain (trace) path of the by-id
// evaluate handler.
func TestEvaluateModelWithExplainTrace(t *testing.T) {
	h := newTestServer(t)
	id := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", dishXML(t))).ModelID

	body := mustJSON(t, evaluateModelRequest{
		Decision: "Dish",
		Input:    map[string]any{"Season": "Winter", "Guest Count": 8},
		Explain:  true,
	})
	rec := do(t, h, "POST", "/v1/models/"+id+"/evaluate", "application/json", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
	if resp := decode[evaluateResponse](t, rec); resp.Trace == nil {
		t.Error("explain set but no trace returned")
	}
}

// TestReadBodyTooLarge covers readBody's MaxBytesReader error branch by posting
// a body larger than maxBodyBytes to POST /v1/models.
func TestReadBodyTooLarge(t *testing.T) {
	h := newTestServer(t)
	big := make([]byte, maxBodyBytes+1)
	for i := range big {
		big[i] = 'a'
	}
	rec := do(t, h, "POST", "/v1/models", "application/xml", big)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body)
	}
	if p := decode[problem](t, rec); p.Code != "INVALID_REQUEST" {
		t.Errorf("code = %q, want INVALID_REQUEST", p.Code)
	}
}
