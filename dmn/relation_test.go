package dmn_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// TestBoxedRelationView exposes a relation's columns and rows, and reports absent
// for a non-relation decision.
func TestBoxedRelationView(t *testing.T) {
	defs := compileModel(t, "boxed_collections_15.dmn")
	v, ok := defs.BoxedRelation("People")
	if !ok {
		t.Fatal("People should be a boxed relation")
	}
	if !v.Simple {
		t.Error("People relation should be simple (all literal cells)")
	}
	if fmt.Sprint(v.Columns) != "[name age]" {
		t.Errorf("columns = %v, want [name age]", v.Columns)
	}
	if len(v.Rows) != 2 || v.Rows[0][0] != `"Ann"` || v.Rows[1][1] != "15" {
		t.Errorf("rows = %v, want [[\"Ann\" 30] [\"Bob\" 15]]", v.Rows)
	}

	if _, ok := defs.BoxedRelation("Numbers"); ok {
		t.Error("Numbers is a list; BoxedRelation should report absent")
	}
}

// TestSetBoxedRelation edits the grid and checks the recompiled model evaluates
// with the new data, and the view round-trips.
func TestSetBoxedRelation(t *testing.T) {
	src := readModel(t, "boxed_collections_15.dmn")
	out, err := dmn.SetBoxedRelation(src, "id_people", dmn.RelationEdit{
		Columns: []string{"name", "age"},
		Rows:    [][]string{{`"Cara"`, "40"}, {"  ", "  "}, {`"Dan"`, "22"}},
	})
	if err != nil {
		t.Fatalf("SetBoxedRelation: %v", err)
	}
	defs, diags, err := dmn.New().Compile(context.Background(), out)
	if err != nil || diags.HasErrors() {
		t.Fatalf("compile: err=%v diags=%+v", err, diags)
	}
	// The blank row is dropped: 2 rows remain.
	v, _ := defs.BoxedRelation("id_people")
	if len(v.Rows) != 2 || v.Rows[0][0] != `"Cara"` || v.Rows[1][1] != "22" {
		t.Errorf("rows not updated / blank not dropped: %v", v.Rows)
	}
	// The relation evaluates to a list of {name, age} contexts.
	dec, _ := defs.Decision("People")
	res, err := dec.Evaluate(context.Background(), dmn.Input{})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	got := fmt.Sprint(res.Outputs["People"])
	if got != "[map[age:40 name:Cara] map[age:22 name:Dan]]" {
		t.Errorf("People = %v", got)
	}
}

// TestSetBoxedRelationRefuses rejects an empty column set, a duplicate column, a
// ragged row and a decision that already carries non-relation logic.
func TestSetBoxedRelationRefuses(t *testing.T) {
	src := readModel(t, "boxed_collections_15.dmn")
	if _, err := dmn.SetBoxedRelation(src, "id_people", dmn.RelationEdit{Columns: nil, Rows: nil}); err == nil {
		t.Error("SetBoxedRelation should reject a relation with no columns")
	}
	if _, err := dmn.SetBoxedRelation(src, "id_people", dmn.RelationEdit{Columns: []string{"a", "a"}, Rows: [][]string{{"1", "2"}}}); err == nil {
		t.Error("SetBoxedRelation should reject duplicate columns")
	}
	if _, err := dmn.SetBoxedRelation(src, "id_people", dmn.RelationEdit{Columns: []string{"a", "b"}, Rows: [][]string{{"1"}}}); err == nil {
		t.Error("SetBoxedRelation should reject a ragged row")
	}
	if _, err := dmn.SetBoxedRelation(readModel(t, "dish_15.dmn"), "Dish", dmn.RelationEdit{Columns: []string{"a"}, Rows: [][]string{{"1"}}}); err == nil {
		t.Error("SetBoxedRelation should refuse a decision-table decision")
	}
}

// TestCreateBoxedRelation gives an undecided decision a fresh relation and refuses
// one that already has logic.
func TestCreateBoxedRelation(t *testing.T) {
	if _, err := dmn.CreateBoxedRelation(readModel(t, "dish_15.dmn"), "Dish"); err == nil {
		t.Error("CreateBoxedRelation should refuse a decision that already has logic")
	}
	const undecided = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" id="d" name="d" namespace="urn:x">
  <decision id="id_d" name="D"><variable name="D"/></decision>
</definitions>`
	out, err := dmn.CreateBoxedRelation([]byte(undecided), "id_d")
	if err != nil {
		t.Fatalf("CreateBoxedRelation: %v", err)
	}
	v, ok := mustCompile(t, string(out)).BoxedRelation("id_d")
	if !ok || len(v.Columns) != 1 || len(v.Rows) != 1 {
		t.Errorf("fresh relation not created: ok=%v view=%+v", ok, v)
	}
}

// TestBoxedRelationNestedReadOnly reports a relation with a nested non-literal
// cell as not simple, so the editor opens read-only.
func TestBoxedRelationNestedReadOnly(t *testing.T) {
	const nested = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" id="d" name="d" namespace="urn:x">
  <decision id="id_d" name="D">
    <variable name="D"/>
    <relation>
      <column name="a"/>
      <row>
        <list><literalExpression><text>1</text></literalExpression></list>
      </row>
    </relation>
  </decision>
</definitions>`
	v, ok := mustCompile(t, nested).BoxedRelation("D")
	if !ok {
		t.Fatal("D should be a boxed relation")
	}
	if v.Simple {
		t.Error("a relation with a nested list cell should not be simple")
	}
}
