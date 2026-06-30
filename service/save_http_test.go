package service

import (
	"net/http"
	"os"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// TestSaveModelPersistsEdits drives the Edit→Save endpoint: it renames a leaf
// decision and repositions it, then checks the saved revision is a new model
// whose graph reflects both edits — the round-trip the modeler relies on.
func TestSaveModelPersistsEdits(t *testing.T) {
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
	if dish.X == 0 && dish.Y == 0 {
		t.Fatal("Dish has no DMNDI bounds to move")
	}

	body := mustJSON(t, saveModelRequest{Nodes: []dmn.NodeEdit{
		{ID: dish.ID, Name: strPtr("Dish Choice"), X: floatPtr(321), Y: floatPtr(123)},
	}})
	rec := do(t, h, "POST", "/v1/models/"+id+"/save", "application/json", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST save = %d, want 201 (body %s)", rec.Code, rec.Body)
	}
	saved := decode[modelResponse](t, rec)
	if saved.ModelID == id {
		t.Error("saved model id unchanged; an edit should change the content hash")
	}
	if !contains(saved.Decisions, "Dish Choice") {
		t.Errorf("saved decisions = %v, want to contain 'Dish Choice'", saved.Decisions)
	}

	g2 := decode[dmn.Graph](t, do(t, h, "GET", "/v1/models/"+saved.ModelID+"/graph", "", nil))
	var moved dmn.GraphNode
	for _, n := range g2.Nodes {
		if n.ID == dish.ID {
			moved = n
		}
	}
	if moved.Name != "Dish Choice" {
		t.Errorf("saved node name = %q, want 'Dish Choice'", moved.Name)
	}
	if moved.X != 321 || moved.Y != 123 {
		t.Errorf("saved node bounds = (%v,%v), want (321,123)", moved.X, moved.Y)
	}
}

// TestSaveDecisionTableColumns edits a table's columns + hit policy through the
// endpoint and checks the saved revision reflects the new structure.
func TestSaveDecisionTableColumns(t *testing.T) {
	h := newTestServer(t)
	id := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", dishXML(t))).ModelID
	tv := decode[dmn.TableView](t, do(t, h, "GET", "/v1/models/"+id+"/decisions/Dish/table", "", nil))

	edit := dmn.TableEdit{
		ReplaceColumns: true,
		HitPolicy:      "F",
		Inputs:         []dmn.TableInput{{Expression: "Season", TypeRef: "string"}},
		Outputs:        []dmn.TableOutput{{Name: "Dish", TypeRef: "string"}, {Name: "Price", TypeRef: "number"}},
		Rules:          []dmn.TableRule{{InputEntries: []string{`"Fall"`}, OutputEntries: []string{`"Ribs"`, `9`}}},
	}
	rec := do(t, h, "POST", "/v1/models/"+id+"/decisions/"+tv.DecisionID+"/table", "application/json", mustJSON(t, edit))
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST table columns = %d, want 201 (body %s)", rec.Code, rec.Body)
	}
	saved := decode[modelResponse](t, rec)
	got := decode[dmn.TableView](t, do(t, h, "GET", "/v1/models/"+saved.ModelID+"/decisions/Dish/table", "", nil))
	if got.HitPolicy != "F" || len(got.Inputs) != 1 || len(got.Outputs) != 2 || got.Outputs[1].Name != "Price" {
		t.Errorf("saved table = %+v, want First / 1 input / Dish+Price outputs", got)
	}
}

// TestGetDecisionTable checks the decision-table endpoint returns the table view
// for a table decision, and 404 for a non-table decision.
func TestGetDecisionTable(t *testing.T) {
	h := newTestServer(t)
	id := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", dishXML(t))).ModelID

	rec := do(t, h, "GET", "/v1/models/"+id+"/decisions/Dish/table", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET table = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
	tv := decode[dmn.TableView](t, rec)
	if tv.Name != "Dish" || len(tv.Rules) == 0 || len(tv.Inputs) == 0 {
		t.Errorf("table view = %+v, want Dish with inputs and rules", tv)
	}

	if rec := do(t, h, "GET", "/v1/models/"+id+"/decisions/Nope/table", "", nil); rec.Code != http.StatusNotFound {
		t.Errorf("GET table for unknown decision = %d, want 404", rec.Code)
	}
}

// TestSaveDecisionTable edits a rule's output cell via the endpoint and checks
// the saved revision's table reflects it.
func TestSaveDecisionTable(t *testing.T) {
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
	rec := do(t, h, "POST", "/v1/models/"+id+"/decisions/"+tv.DecisionID+"/table", "application/json", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST table = %d, want 201 (body %s)", rec.Code, rec.Body)
	}
	saved := decode[modelResponse](t, rec)
	if saved.ModelID == id {
		t.Error("saved model id unchanged; a table edit should change the content hash")
	}

	tv2 := decode[dmn.TableView](t, do(t, h, "GET", "/v1/models/"+saved.ModelID+"/decisions/Dish/table", "", nil))
	var sawLobster bool
	for _, r := range tv2.Rules {
		if r.OutputEntries[0] == `"Lobster"` {
			sawLobster = true
		}
	}
	if !sawLobster {
		t.Errorf("edited output not persisted; rules=%+v", tv2.Rules)
	}
}

// TestSaveDecisionTableNoTable is a 404 for a decision without a table.
func TestSaveDecisionTableNoTable(t *testing.T) {
	h := newTestServer(t)
	id := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", dishXML(t))).ModelID
	body := mustJSON(t, dmn.TableEdit{Rules: nil})
	if rec := do(t, h, "POST", "/v1/models/"+id+"/decisions/Nope/table", "application/json", body); rec.Code != http.StatusNotFound {
		t.Errorf("POST table for unknown decision = %d, want 404", rec.Code)
	}
}

// TestSaveGraphAddNode adds a node + edge through the graph endpoint and checks
// the saved revision's graph carries them.
func TestSaveGraphAddNode(t *testing.T) {
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
	edit.Nodes = append(edit.Nodes, dmn.GraphNodeEdit{ID: "id_wine", Type: "inputData", Name: "Wine", DataType: "string", X: 600, Y: 100, Width: 120, Height: 50})
	edit.Edges = append(edit.Edges, dmn.GraphEdgeEdit{Type: "informationRequirement", Source: "id_wine", Target: "id_dish"})

	rec := do(t, h, "POST", "/v1/models/"+id+"/graph", "application/json", mustJSON(t, edit))
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST graph = %d, want 201 (body %s)", rec.Code, rec.Body)
	}
	saved := decode[modelResponse](t, rec)
	if saved.ModelID == id {
		t.Error("saved model id unchanged; a structural edit should change the content hash")
	}

	g2 := decode[dmn.Graph](t, do(t, h, "GET", "/v1/models/"+saved.ModelID+"/graph", "", nil))
	var wine *dmn.GraphNode
	for i := range g2.Nodes {
		if g2.Nodes[i].Name == "Wine" {
			wine = &g2.Nodes[i]
		}
	}
	if wine == nil {
		t.Fatalf("added node 'Wine' missing in saved graph: %+v", g2.Nodes)
	}
	if wine.X != 600 || wine.Width != 120 {
		t.Errorf("Wine shape not persisted: %+v", *wine)
	}
}

// TestCreateDecisionTableEndpoint adds an undecided decision, gives it a table via
// the endpoint, and checks the saved model's table has the requirement-derived
// column.
func TestCreateDecisionTableEndpoint(t *testing.T) {
	h := newTestServer(t)
	id := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", dishXML(t))).ModelID

	// Add an undecided "Pairing" decision requiring Season.
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

	rec := do(t, h, "POST", "/v1/models/"+withDec+"/decisions/id_pairing/create-table", "", nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST create-table = %d, want 201 (body %s)", rec.Code, rec.Body)
	}
	created := decode[modelResponse](t, rec)

	tv := decode[dmn.TableView](t, do(t, h, "GET", "/v1/models/"+created.ModelID+"/decisions/id_pairing/table", "", nil))
	if len(tv.Inputs) != 1 || tv.Inputs[0].Expression != "Season" {
		t.Errorf("created table inputs = %+v, want one 'Season' column", tv.Inputs)
	}

	// Creating a table for a decision that already has one is rejected.
	if rec := do(t, h, "POST", "/v1/models/"+id+"/decisions/id_dish/create-table", "", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("create-table on a decided decision = %d, want 400", rec.Code)
	}
}

// TestLiteralEndpoints checks reading and editing a decision's literal expression
// through the endpoints.
func TestLiteralEndpoints(t *testing.T) {
	h := newTestServer(t)
	xml := readFile(t, "../dmn/testdata/models/pricing_15.dmn")
	id := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", xml)).ModelID

	lv := decode[dmn.LiteralView](t, do(t, h, "GET", "/v1/models/"+id+"/decisions/id_net/literal", "", nil))
	if lv.Text != "Unit Price * Quantity" {
		t.Errorf("literal text = %q, want 'Unit Price * Quantity'", lv.Text)
	}

	body := mustJSON(t, saveLiteralRequest{Text: "Unit Price * Quantity * 2", TypeRef: "number"})
	rec := do(t, h, "POST", "/v1/models/"+id+"/decisions/id_net/literal", "application/json", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST literal = %d, want 201 (body %s)", rec.Code, rec.Body)
	}
	saved := decode[modelResponse](t, rec)
	got := decode[dmn.LiteralView](t, do(t, h, "GET", "/v1/models/"+saved.ModelID+"/decisions/id_net/literal", "", nil))
	if got.Text != "Unit Price * Quantity * 2" {
		t.Errorf("saved literal = %q, want the edited expression", got.Text)
	}

	// An unknown decision has no literal — 404.
	if rec := do(t, h, "GET", "/v1/models/"+id+"/decisions/nope/literal", "", nil); rec.Code != http.StatusNotFound {
		t.Errorf("GET literal for unknown decision = %d, want 404", rec.Code)
	}
}

// TestTypeEndpoints checks creating, listing and deleting a custom type through
// the endpoints.
func TestTypeEndpoints(t *testing.T) {
	h := newTestServer(t)
	id := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", dishXML(t))).ModelID

	body := mustJSON(t, dmn.ItemType{Name: "Color", TypeRef: "string", AllowedValues: `"red","green"`})
	rec := do(t, h, "POST", "/v1/models/"+id+"/types", "application/json", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST type = %d, want 201 (body %s)", rec.Code, rec.Body)
	}
	saved := decode[modelResponse](t, rec)

	list := decode[struct {
		Types []dmn.ItemType `json:"types"`
	}](t, do(t, h, "GET", "/v1/models/"+saved.ModelID+"/types", "", nil))
	if len(list.Types) != 1 || list.Types[0].Name != "Color" {
		t.Fatalf("types = %+v, want one named Color", list.Types)
	}

	del := do(t, h, "DELETE", "/v1/models/"+saved.ModelID+"/types/Color", "", nil)
	if del.Code != http.StatusCreated {
		t.Fatalf("DELETE type = %d, want 201 (body %s)", del.Code, del.Body)
	}
	after := decode[modelResponse](t, del)
	empty := decode[struct {
		Types []dmn.ItemType `json:"types"`
	}](t, do(t, h, "GET", "/v1/models/"+after.ModelID+"/types", "", nil))
	if len(empty.Types) != 0 {
		t.Errorf("types after delete = %+v, want none", empty.Types)
	}
}

// TestBKMEndpoints checks reading and editing a BKM's function via the endpoints.
func TestBKMEndpoints(t *testing.T) {
	h := newTestServer(t)
	xml := readFile(t, "../dmn/testdata/models/bkm_invocation_15.dmn")
	id := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", xml)).ModelID

	v := decode[dmn.BKMView](t, do(t, h, "GET", "/v1/models/"+id+"/bkm/id_rate", "", nil))
	if !v.Simple || len(v.Params) != 1 || v.Params[0].Name != "total" {
		t.Fatalf("BKM view = %+v, want simple with param total", v)
	}

	body := mustJSON(t, dmn.BKMFunctionEdit{Params: []dmn.BKMParam{{Name: "total", TypeRef: "number"}}, BodyText: "if total > 500 then 0.5 else 0.0", BodyTypeRef: "number"})
	rec := do(t, h, "POST", "/v1/models/"+id+"/bkm/id_rate", "application/json", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST bkm = %d, want 201 (body %s)", rec.Code, rec.Body)
	}
	saved := decode[modelResponse](t, rec)
	got := decode[dmn.BKMView](t, do(t, h, "GET", "/v1/models/"+saved.ModelID+"/bkm/id_rate", "", nil))
	if got.BodyText != "if total > 500 then 0.5 else 0.0" {
		t.Errorf("saved BKM body = %q, want the edited expression", got.BodyText)
	}

	if rec := do(t, h, "GET", "/v1/models/"+id+"/bkm/nope", "", nil); rec.Code != http.StatusNotFound {
		t.Errorf("GET unknown BKM = %d, want 404", rec.Code)
	}
}

// TestSaveModelUnknownModel checks saving against a missing model is a 404.
func TestSaveModelUnknownModel(t *testing.T) {
	h := newTestServer(t)
	body := mustJSON(t, saveModelRequest{Nodes: []dmn.NodeEdit{{ID: "x", Name: strPtr("Y")}}})
	if rec := do(t, h, "POST", "/v1/models/sha256:deadbeef/save", "application/json", body); rec.Code != http.StatusNotFound {
		t.Errorf("save unknown model = %d, want 404", rec.Code)
	}
}

// TestEvaluateGraphEndpoint drives the whole-graph evaluation: it fills the one
// leaf input (Applicant Age) and checks every decision in the chain comes back
// with its value, including the transitive one the form does not name directly.
func TestEvaluateGraphEndpoint(t *testing.T) {
	h := newTestServer(t)
	xml, err := os.ReadFile("../dmn/testdata/models/routing_13.dmn")
	if err != nil {
		t.Fatalf("read routing model: %v", err)
	}
	id := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", xml)).ModelID

	body := mustJSON(t, evaluateGraphRequest{Input: map[string]any{"Applicant Age": 20}, Strict: true})
	rec := do(t, h, "POST", "/v1/models/"+id+"/evaluate-graph", "application/json", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST evaluate-graph = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
	res := decode[evaluateGraphResponse](t, rec)
	if res.Values["Eligibility"] != "ELIGIBLE" {
		t.Errorf("Eligibility = %v, want ELIGIBLE", res.Values["Eligibility"])
	}
	if res.Values["Routing"] != "ACCEPT" {
		t.Errorf("Routing = %v, want ACCEPT", res.Values["Routing"])
	}
	// The graph input schema names the single leaf input the form should ask for.
	if len(res.InputSchema) != 1 || res.InputSchema[0].Name != "Applicant Age" {
		t.Errorf("inputSchema = %+v, want [Applicant Age]", res.InputSchema)
	}
}

// TestEvaluateGraphRejectsUnknownInput checks strict graph validation reports an
// input no decision declares as a structured INVALID_INPUT problem.
func TestEvaluateGraphRejectsUnknownInput(t *testing.T) {
	h := newTestServer(t)
	xml, err := os.ReadFile("../dmn/testdata/models/routing_13.dmn")
	if err != nil {
		t.Fatalf("read routing model: %v", err)
	}
	id := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", xml)).ModelID

	body := mustJSON(t, evaluateGraphRequest{Input: map[string]any{"Applicant Age": 20, "Bogus": 1}, Strict: true})
	rec := do(t, h, "POST", "/v1/models/"+id+"/evaluate-graph", "application/json", body)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("evaluate-graph unknown input = %d, want 422 (body %s)", rec.Code, rec.Body)
	}
}

func strPtr(s string) *string     { return &s }
func floatPtr(f float64) *float64 { return &f }
