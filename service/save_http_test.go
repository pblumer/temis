package service

import (
	"net/http"
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

// TestSaveModelUnknownModel checks saving against a missing model is a 404.
func TestSaveModelUnknownModel(t *testing.T) {
	h := newTestServer(t)
	body := mustJSON(t, saveModelRequest{Nodes: []dmn.NodeEdit{{ID: "x", Name: strPtr("Y")}}})
	if rec := do(t, h, "POST", "/v1/models/sha256:deadbeef/save", "application/json", body); rec.Code != http.StatusNotFound {
		t.Errorf("save unknown model = %d, want 404", rec.Code)
	}
}

func strPtr(s string) *string     { return &s }
func floatPtr(f float64) *float64 { return &f }
