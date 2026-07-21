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

// structuredCollectionModel declares a structured element type (Driver) and a
// collection type (DriverList) that reuses it — the shape a self-describing form
// needs to unfold into „list of objects with these fields".
const structuredCollectionModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/drv" name="Drv" id="def_drv">
  <itemDefinition id="t_driver" name="Driver">
    <itemComponent id="c_age" name="age"><typeRef>number</typeRef></itemComponent>
    <itemComponent id="c_name" name="dname"><typeRef>string</typeRef></itemComponent>
  </itemDefinition>
  <itemDefinition id="t_drivers" name="DriverList" isCollection="true">
    <typeRef>Driver</typeRef>
  </itemDefinition>
  <inputData id="i_x" name="X"><variable name="X" typeRef="number"/></inputData>
  <decision id="d_x" name="UseX">
    <informationRequirement><requiredInput href="#i_x"/></informationRequirement>
    <literalExpression><text>X + 1</text></literalExpression>
  </decision>
</definitions>`

// TestItemDefinitionComponents checks a structured type reports its fields (name
// + type), so a consumer can describe the shape a caller must supply — the whole
// point of exposing Components on ItemType.
func TestItemDefinitionComponents(t *testing.T) {
	types := itemTypes(t, []byte(structuredCollectionModel))

	driver := types["Driver"]
	if !driver.Structured {
		t.Fatalf("Driver should be structured, got %+v", driver)
	}
	if len(driver.Components) != 2 {
		t.Fatalf("Driver components = %d, want 2 (%+v)", len(driver.Components), driver.Components)
	}
	byName := map[string]dmn.ItemType{}
	for _, c := range driver.Components {
		byName[c.Name] = c
	}
	if got := byName["age"]; got.TypeRef != "number" {
		t.Errorf("age component typeRef = %q, want number", got.TypeRef)
	}
	if got := byName["dname"]; got.TypeRef != "string" {
		t.Errorf("dname component typeRef = %q, want string", got.TypeRef)
	}

	// The collection type carries its element type and no components of its own.
	list := types["DriverList"]
	if !list.IsCollection || list.TypeRef != "Driver" {
		t.Errorf("DriverList = %+v, want collection of Driver", list)
	}
	if len(list.Components) != 0 {
		t.Errorf("DriverList should have no direct components, got %+v", list.Components)
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

// TestSetStructuredType covers creating and editing a structured type through
// SetItemDefinition's Components path (the struct editor's backend): a new struct
// round-trips its fields, and re-saving replaces them.
func TestSetStructuredType(t *testing.T) {
	src := readModel(t, "dish_15.dmn")
	out, err := dmn.SetItemDefinition(src, dmn.ItemType{
		Name: "Person",
		Components: []dmn.ItemType{
			{Name: "name", TypeRef: "string"},
			{Name: "alter", TypeRef: "number"},
		},
	})
	if err != nil {
		t.Fatalf("SetItemDefinition (struct): %v", err)
	}
	types := itemTypes(t, out)
	p := types["Person"]
	if !p.Structured || len(p.Components) != 2 {
		t.Fatalf("Person should be structured with 2 fields, got %+v", p)
	}

	// Re-saving with a different field set replaces the components.
	out, err = dmn.SetItemDefinition(out, dmn.ItemType{
		Name:       "Person",
		Components: []dmn.ItemType{{Name: "email", TypeRef: "string", IsCollection: true}},
	})
	if err != nil {
		t.Fatalf("SetItemDefinition (struct re-save): %v", err)
	}
	p = itemTypes(t, out)["Person"]
	if len(p.Components) != 1 || p.Components[0].Name != "email" || !p.Components[0].IsCollection {
		t.Fatalf("re-saved Person components = %+v, want one collection field 'email'", p.Components)
	}

	// A struct field with an empty name is rejected.
	if _, err := dmn.SetItemDefinition(src, dmn.ItemType{Name: "Bad", Components: []dmn.ItemType{{Name: " "}}}); err == nil {
		t.Error("expected error for a struct field with an empty name")
	}
}
