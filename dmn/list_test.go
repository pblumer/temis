package dmn_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// TestBoxedListView exposes a list's ordered FEEL items and reports absent for a
// non-list decision.
func TestBoxedListView(t *testing.T) {
	defs := compileModel(t, "boxed_collections_15.dmn")
	v, ok := defs.BoxedList("Numbers")
	if !ok {
		t.Fatal("Numbers should be a boxed list")
	}
	if !v.Simple {
		t.Error("Numbers list should be simple (all literal items)")
	}
	if fmt.Sprint(v.Items) != "[1 2 3]" {
		t.Errorf("list items = %v, want [1 2 3]", v.Items)
	}

	if _, ok := defs.BoxedList("Grade"); ok {
		t.Error("Grade is a conditional; BoxedList should report absent")
	}
}

// TestSetBoxedList edits the items and checks the recompiled model evaluates with
// the new list, and the view round-trips.
func TestSetBoxedList(t *testing.T) {
	src := readModel(t, "boxed_collections_15.dmn")
	out, err := dmn.SetBoxedList(src, "id_numbers", dmn.ListEdit{Items: []string{"10", "20", "  ", "30"}})
	if err != nil {
		t.Fatalf("SetBoxedList: %v", err)
	}
	defs, diags, err := dmn.New().Compile(context.Background(), out)
	if err != nil || diags.HasErrors() {
		t.Fatalf("compile: err=%v diags=%+v", err, diags)
	}
	// Blank items are dropped: [10, 20, 30].
	v, _ := defs.BoxedList("id_numbers")
	if fmt.Sprint(v.Items) != "[10 20 30]" {
		t.Errorf("items not updated / blanks not dropped: %v", v.Items)
	}
	dec, _ := defs.Decision("Numbers")
	res, err := dec.Evaluate(context.Background(), dmn.Input{})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got := fmt.Sprint(res.Outputs["Numbers"]); got != "[10 20 30]" {
		t.Errorf("Numbers = %v, want [10 20 30]", got)
	}
}

// TestSetBoxedListRefuses rejects an empty list and a decision that already
// carries non-list logic.
func TestSetBoxedListRefuses(t *testing.T) {
	src := readModel(t, "boxed_collections_15.dmn")
	if _, err := dmn.SetBoxedList(src, "id_numbers", dmn.ListEdit{Items: []string{"  ", ""}}); err == nil {
		t.Error("SetBoxedList should reject an all-blank (empty) list")
	}
	if _, err := dmn.SetBoxedList(readModel(t, "dish_15.dmn"), "Dish", dmn.ListEdit{Items: []string{"1"}}); err == nil {
		t.Error("SetBoxedList should refuse a decision-table decision")
	}
}

// TestCreateBoxedList gives an undecided decision a fresh list and refuses one
// that already has logic.
func TestCreateBoxedList(t *testing.T) {
	if _, err := dmn.CreateBoxedList(readModel(t, "dish_15.dmn"), "Dish"); err == nil {
		t.Error("CreateBoxedList should refuse a decision that already has logic")
	}
	const undecided = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" id="d" name="d" namespace="urn:x">
  <decision id="id_d" name="D"><variable name="D"/></decision>
</definitions>`
	out, err := dmn.CreateBoxedList([]byte(undecided), "id_d")
	if err != nil {
		t.Fatalf("CreateBoxedList: %v", err)
	}
	v, ok := mustCompile(t, string(out)).BoxedList("id_d")
	if !ok || len(v.Items) != 1 {
		t.Errorf("fresh list not created: ok=%v view=%+v", ok, v)
	}
}

// TestBoxedListNestedReadOnly reports a list with a nested non-literal item as not
// simple, so the editor opens read-only.
func TestBoxedListNestedReadOnly(t *testing.T) {
	const nested = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" id="d" name="d" namespace="urn:x">
  <decision id="id_d" name="D">
    <variable name="D"/>
    <list>
      <literalExpression><text>1</text></literalExpression>
      <context>
        <contextEntry><variable name="a"/><literalExpression><text>2</text></literalExpression></contextEntry>
      </context>
    </list>
  </decision>
</definitions>`
	v, ok := mustCompile(t, nested).BoxedList("D")
	if !ok {
		t.Fatal("D should be a boxed list")
	}
	if v.Simple {
		t.Error("a list with a nested context item should not be simple")
	}
	if len(v.Items) != 2 || v.Items[0] != "1" {
		t.Errorf("items = %v, want [1, <placeholder>]", v.Items)
	}
}
