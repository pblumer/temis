package dmn_test

import (
	"context"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// itemTypes recompiles xml and indexes its item definitions by name.
func itemTypes(t *testing.T, xml []byte) map[string]dmn.ItemType {
	t.Helper()
	defs, diags, err := dmn.New().Compile(context.Background(), xml)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("diagnostics: %+v", diags)
	}
	m := map[string]dmn.ItemType{}
	for _, it := range defs.ItemDefinitions() {
		m[it.Name] = it
	}
	return m
}

// TestItemDefinitionCRUD creates, updates and removes a simple custom type and
// checks each step survives the recompile round-trip.
func TestItemDefinitionCRUD(t *testing.T) {
	src := readModel(t, "dish_15.dmn")

	out, err := dmn.SetItemDefinition(src, dmn.ItemType{Name: "Color", TypeRef: "string", AllowedValues: `"red","green"`})
	if err != nil {
		t.Fatalf("SetItemDefinition: %v", err)
	}
	got := itemTypes(t, out)["Color"]
	if got.TypeRef != "string" || got.AllowedValues != `"red","green"` {
		t.Errorf("Color = %+v, want string with allowed values", got)
	}

	// Update it (collection of strings, new allowed values).
	out, err = dmn.SetItemDefinition(out, dmn.ItemType{Name: "Color", TypeRef: "string", IsCollection: true, AllowedValues: `"blue"`})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if upd := itemTypes(t, out)["Color"]; !upd.IsCollection || upd.AllowedValues != `"blue"` {
		t.Errorf("updated Color = %+v, want collection with 'blue'", upd)
	}

	// Remove it.
	out, err = dmn.RemoveItemDefinition(out, "Color")
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, still := itemTypes(t, out)["Color"]; still {
		t.Error("Color still present after remove")
	}
}

// TestItemDefinitionAssignable checks a custom type can be assigned to an input
// and the model still compiles (the engine resolves the named type).
func TestItemDefinitionAssignable(t *testing.T) {
	src := readModel(t, "dish_15.dmn")
	withType, err := dmn.SetItemDefinition(src, dmn.ItemType{Name: "Temperature", TypeRef: "number"})
	if err != nil {
		t.Fatalf("SetItemDefinition: %v", err)
	}
	// Assign it to an inputData via the structural graph (type carried as the
	// node's dataType).
	g := graphEdit(t, withType)
	for i := range g.Nodes {
		if g.Nodes[i].Name == "Guest Count" {
			g.Nodes[i].DataType = "Temperature"
		}
	}
	out, err := dmn.ApplyGraph(withType, g)
	if err != nil {
		t.Fatalf("ApplyGraph: %v", err)
	}
	// Compiles cleanly with the custom type assigned.
	defs, diags, err := dmn.New().Compile(context.Background(), out)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("diagnostics with custom type: %+v", diags)
	}
	if _, ok := defs.LiteralExpression("Dish"); ok {
		t.Fatal("sanity: Dish should be a table, not literal")
	}
}

// TestItemDefinitionErrors checks empty name and removing an unknown type error.
func TestItemDefinitionErrors(t *testing.T) {
	src := readModel(t, "dish_15.dmn")
	if _, err := dmn.SetItemDefinition(src, dmn.ItemType{Name: "  "}); err == nil {
		t.Error("expected error for empty type name")
	}
	if _, err := dmn.RemoveItemDefinition(src, "Nope"); err == nil {
		t.Error("expected error removing an unknown type")
	}
}
