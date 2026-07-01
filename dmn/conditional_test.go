package dmn_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// TestBoxedConditionalView exposes a conditional's three FEEL branches and reports
// absent for a non-conditional decision.
func TestBoxedConditionalView(t *testing.T) {
	defs := compileModel(t, "boxed_collections_15.dmn")
	v, ok := defs.BoxedConditional("Grade")
	if !ok {
		t.Fatal("Grade should be a boxed conditional")
	}
	if !v.Simple {
		t.Error("Grade conditional should be simple (all literal branches)")
	}
	if v.If != "Threshold > 5" || v.Then != `"high"` || v.Else != `"low"` {
		t.Errorf("conditional view = %+v, want Threshold > 5 / \"high\" / \"low\"", v)
	}

	if _, ok := defs.BoxedConditional("Numbers"); ok {
		t.Error("Numbers is a list; BoxedConditional should report absent")
	}
}

// TestSetBoxedConditional edits the branches and checks the recompiled model
// evaluates with the new logic, and the view round-trips.
func TestSetBoxedConditional(t *testing.T) {
	src := readModel(t, "boxed_collections_15.dmn")
	out, err := dmn.SetBoxedConditional(src, "id_grade", dmn.ConditionalEdit{
		If: "Threshold > 10", Then: `"top"`, Else: `"bottom"`,
	})
	if err != nil {
		t.Fatalf("SetBoxedConditional: %v", err)
	}
	defs, diags, err := dmn.New().Compile(context.Background(), out)
	if err != nil || diags.HasErrors() {
		t.Fatalf("compile: err=%v diags=%+v", err, diags)
	}
	v, _ := defs.BoxedConditional("id_grade")
	if v.If != "Threshold > 10" || v.Then != `"top"` {
		t.Errorf("branches not updated: %+v", v)
	}
	// Threshold=3 → not > 10 → else "bottom".
	dec, _ := defs.Decision("Grade")
	res, err := dec.Evaluate(context.Background(), dmn.Input{"Threshold": 3})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got := fmt.Sprint(res.Outputs["Grade"]); got != "bottom" {
		t.Errorf("Grade = %v, want bottom", got)
	}
}

// TestSetBoxedConditionalRefuses rejects an empty branch and a decision that
// already carries non-conditional logic.
func TestSetBoxedConditionalRefuses(t *testing.T) {
	src := readModel(t, "boxed_collections_15.dmn")
	if _, err := dmn.SetBoxedConditional(src, "id_grade", dmn.ConditionalEdit{If: "true", Then: "", Else: "0"}); err == nil {
		t.Error("SetBoxedConditional should reject an empty branch")
	}
	// Dish is a decision table, not a conditional.
	if _, err := dmn.SetBoxedConditional(readModel(t, "dish_15.dmn"), "Dish", dmn.ConditionalEdit{If: "true", Then: "1", Else: "0"}); err == nil {
		t.Error("SetBoxedConditional should refuse a decision-table decision")
	}
}

// TestCreateBoxedConditional gives an undecided decision a fresh conditional and
// refuses one that already has logic.
func TestCreateBoxedConditional(t *testing.T) {
	// A decision-table decision already has logic → refuse.
	if _, err := dmn.CreateBoxedConditional(readModel(t, "dish_15.dmn"), "Dish"); err == nil {
		t.Error("CreateBoxedConditional should refuse a decision that already has logic")
	}
	// Build a tiny model with an undecided decision, then create a conditional.
	const undecided = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" id="d" name="d" namespace="urn:x">
  <decision id="id_d" name="D"><variable name="D" typeRef="number"/></decision>
</definitions>`
	out, err := dmn.CreateBoxedConditional([]byte(undecided), "id_d")
	if err != nil {
		t.Fatalf("CreateBoxedConditional: %v", err)
	}
	defs, diags, err := dmn.New().Compile(context.Background(), out)
	if err != nil || diags.HasErrors() {
		t.Fatalf("compile: err=%v diags=%+v", err, diags)
	}
	v, ok := defs.BoxedConditional("id_d")
	if !ok || v.If == "" || v.Then == "" || v.Else == "" {
		t.Errorf("fresh conditional not created: ok=%v view=%+v", ok, v)
	}
}

// TestBoxedConditionalNestedReadOnly reports a conditional with a nested
// non-literal branch as not simple, so the editor opens read-only.
func TestBoxedConditionalNestedReadOnly(t *testing.T) {
	const nested = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" id="d" name="d" namespace="urn:x">
  <inputData id="id_in" name="In"><variable name="In" typeRef="number"/></inputData>
  <decision id="id_d" name="D">
    <variable name="D" typeRef="number"/>
    <informationRequirement><requiredInput href="#id_in"/></informationRequirement>
    <conditional>
      <if><literalExpression><text>In > 0</text></literalExpression></if>
      <then>
        <context>
          <contextEntry><variable name="a"/><literalExpression><text>1</text></literalExpression></contextEntry>
        </context>
      </then>
      <else><literalExpression><text>0</text></literalExpression></else>
    </conditional>
  </decision>
</definitions>`
	v, ok := mustCompile(t, nested).BoxedConditional("D")
	if !ok {
		t.Fatal("D should be a boxed conditional")
	}
	if v.Simple {
		t.Error("a conditional with a nested context branch should not be simple")
	}
	if !strings.HasPrefix(v.If, "In > 0") {
		t.Errorf("if branch = %q, want In > 0", v.If)
	}
}

// mustCompile compiles inline XML, failing on any error or diagnostic.
func mustCompile(t *testing.T, xml string) *dmn.Definitions {
	t.Helper()
	defs, diags, err := dmn.New().Compile(context.Background(), []byte(xml))
	if err != nil || diags.HasErrors() {
		t.Fatalf("compile: err=%v diags=%+v", err, diags)
	}
	return defs
}
