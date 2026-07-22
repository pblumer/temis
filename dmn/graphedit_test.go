package dmn_test

import (
	"context"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// graphEdit snapshots a model's current graph into an editable GraphEdit.
func graphEdit(t *testing.T, xml []byte) dmn.GraphEdit {
	t.Helper()
	defs, _, err := dmn.New().Compile(context.Background(), xml)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	g := defs.Graph()
	e := dmn.GraphEdit{}
	for _, n := range g.Nodes {
		e.Nodes = append(e.Nodes, dmn.GraphNodeEdit{ID: n.ID, Type: n.Type, Name: n.Name, DataType: n.DataType, X: n.X, Y: n.Y, Width: n.Width, Height: n.Height})
	}
	for _, ed := range g.Edges {
		e.Edges = append(e.Edges, dmn.GraphEdgeEdit(ed))
	}
	return e
}

// TestApplyGraphPersistsLayoutWhenNoDiagram covers the layout-persist path: a
// model authored WITHOUT DMNDI (routing_13) gets the modeler's positions written
// as a synthesised diagram on save, so the arrangement sticks on reload.
func TestApplyGraphPersistsLayoutWhenNoDiagram(t *testing.T) {
	src := readModel(t, "routing_13.dmn") // no DMNDI

	edit := graphEdit(t, src)
	if len(edit.Nodes) == 0 {
		t.Fatal("expected nodes")
	}
	// The source carries no bounds; assign a layout as the modeler would.
	for i := range edit.Nodes {
		edit.Nodes[i].X = float64(i * 200)
		edit.Nodes[i].Y = float64(i * 120)
		edit.Nodes[i].Width = 150
		edit.Nodes[i].Height = 70
	}

	out, err := dmn.ApplyGraph(src, edit)
	if err != nil {
		t.Fatalf("ApplyGraph: %v", err)
	}

	g := graphByName(t, out)
	for i, n := range edit.Nodes {
		got, ok := g[n.Name]
		if !ok {
			t.Fatalf("node %q missing after save", n.Name)
		}
		if got.X != float64(i*200) || got.Y != float64(i*120) || got.Width != 150 || got.Height != 70 {
			t.Errorf("node %q bounds = (%v,%v,%v,%v), want persisted (%v,%v,150,70)", n.Name, got.X, got.Y, got.Width, got.Height, i*200, i*120)
		}
	}
}

// TestApplyGraphAddThenDelete adds a fresh inputData wired into the Dish decision,
// checks it (and its DMNDI shape) round-trips, then removes it again and checks it
// is gone — while the Dish decision keeps evaluating throughout (logic preserved).
func TestApplyGraphAddThenDelete(t *testing.T) {
	src := readModel(t, "dish_15.dmn")

	// Add "Wine" (string) at (600,100) with an information requirement into Dish.
	add := graphEdit(t, src)
	add.Nodes = append(add.Nodes, dmn.GraphNodeEdit{ID: "id_wine", Type: "inputData", Name: "Wine", DataType: "string", X: 600, Y: 100, Width: 120, Height: 50})
	add.Edges = append(add.Edges, dmn.GraphEdgeEdit{Type: "informationRequirement", Source: "id_wine", Target: "id_dish"})

	out1, err := dmn.ApplyGraph(src, add)
	if err != nil {
		t.Fatalf("ApplyGraph add: %v", err)
	}
	g1 := graphByName(t, out1)
	wine, ok := g1["Wine"]
	if !ok {
		t.Fatalf("added node 'Wine' missing; nodes: %v", names(g1))
	}
	if wine.DataType != "string" {
		t.Errorf("Wine type = %q, want string", wine.DataType)
	}
	if wine.X != 600 || wine.Y != 100 || wine.Width != 120 {
		t.Errorf("Wine has no/incorrect DMNDI shape: %+v, want x600 y100 w120", wine)
	}
	if !hasEdge(out1, t, "id_wine", "id_dish") {
		t.Error("missing requirement edge id_wine -> id_dish after add")
	}
	// The decision table survived the structural edit verbatim (its rules are
	// intact); wiring Wine in only adds it as a required input.

	// Now delete "Wine" again (omit it from the desired graph).
	del := graphEdit(t, out1)
	del = without(del, "id_wine")
	out2, err := dmn.ApplyGraph(out1, del)
	if err != nil {
		t.Fatalf("ApplyGraph delete: %v", err)
	}
	g2 := graphByName(t, out2)
	if _, still := g2["Wine"]; still {
		t.Error("'Wine' still present after delete")
	}
	if hasEdge(out2, t, "id_wine", "id_dish") {
		t.Error("requirement edge id_wine -> id_dish still present after delete")
	}
	if got := evalDish(t, out2, "Winter", 4); got != "Roastbeef" {
		t.Errorf("Dish logic broke after delete: Winter/4 = %q, want Roastbeef", got)
	}
}

// TestApplyGraphAddDecision adds a new (undecided) decision wired to an existing
// input; it appears in the graph as a decision with no table.
func TestApplyGraphAddDecision(t *testing.T) {
	src := readModel(t, "dish_15.dmn")
	e := graphEdit(t, src)
	e.Nodes = append(e.Nodes, dmn.GraphNodeEdit{ID: "id_pairing", Type: "decision", Name: "Pairing", X: 600, Y: 260, Width: 150, Height: 70})
	e.Edges = append(e.Edges, dmn.GraphEdgeEdit{Type: "informationRequirement", Source: "id_season", Target: "id_pairing"})

	out, err := dmn.ApplyGraph(src, e)
	if err != nil {
		t.Fatalf("ApplyGraph: %v", err)
	}
	g := graphByName(t, out)
	n, ok := g["Pairing"]
	if !ok {
		t.Fatalf("added decision 'Pairing' missing; nodes: %v", names(g))
	}
	if n.Type != "decision" || n.HasTable {
		t.Errorf("Pairing = {type:%q hasTable:%v}, want a table-less decision", n.Type, n.HasTable)
	}
	if !hasEdge(out, t, "id_season", "id_pairing") {
		t.Error("missing requirement edge id_season -> id_pairing")
	}
}

// TestApplyGraphMoveExisting checks moving an existing node via the graph save
// updates its DMNDI shape bounds.
func TestApplyGraphMoveExisting(t *testing.T) {
	src := readModel(t, "dish_15.dmn")
	e := graphEdit(t, src)
	for i := range e.Nodes {
		if e.Nodes[i].ID == "id_dish" {
			e.Nodes[i].X, e.Nodes[i].Y = 333, 444
		}
	}
	out, err := dmn.ApplyGraph(src, e)
	if err != nil {
		t.Fatalf("ApplyGraph: %v", err)
	}
	if n := graphByName(t, out)["Dish"]; n.X != 333 || n.Y != 444 {
		t.Errorf("Dish bounds after move = (%v,%v), want (333,444)", n.X, n.Y)
	}
}

// TestApplyGraphResizeExisting checks resizing an existing node via the graph
// save writes the new width/height into its DMNDI shape bounds and reads them
// back — the round-trip that persists a manual resize in the modeler.
func TestApplyGraphResizeExisting(t *testing.T) {
	src := readModel(t, "dish_15.dmn")
	e := graphEdit(t, src)
	for i := range e.Nodes {
		if e.Nodes[i].ID == "id_dish" {
			e.Nodes[i].Width, e.Nodes[i].Height = 240, 96
		}
	}
	out, err := dmn.ApplyGraph(src, e)
	if err != nil {
		t.Fatalf("ApplyGraph: %v", err)
	}
	if n := graphByName(t, out)["Dish"]; n.Width != 240 || n.Height != 96 {
		t.Errorf("Dish bounds after resize = (%v x %v), want (240 x 96)", n.Width, n.Height)
	}
}

// TestApplyGraphUnknownType errors on an unrecognised node type.
func TestApplyGraphUnknownType(t *testing.T) {
	src := readModel(t, "dish_15.dmn")
	_, err := dmn.ApplyGraph(src, dmn.GraphEdit{Nodes: []dmn.GraphNodeEdit{{ID: "x", Type: "gizmo", Name: "X"}}})
	if err == nil {
		t.Error("expected error for unknown node type")
	}
}

func without(e dmn.GraphEdit, id string) dmn.GraphEdit {
	out := dmn.GraphEdit{}
	for _, n := range e.Nodes {
		if n.ID != id {
			out.Nodes = append(out.Nodes, n)
		}
	}
	for _, ed := range e.Edges {
		if ed.Source != id && ed.Target != id {
			out.Edges = append(out.Edges, ed)
		}
	}
	return out
}

func hasEdge(xml []byte, t *testing.T, source, target string) bool {
	t.Helper()
	defs, _, err := dmn.New().Compile(context.Background(), xml)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	for _, e := range defs.Graph().Edges {
		if e.Source == source && e.Target == target {
			return true
		}
	}
	return false
}
