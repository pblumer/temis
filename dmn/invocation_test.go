package dmn_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// TestBoxedInvocationView exposes the called function and bindings, and reports
// absent for a non-invocation decision.
func TestBoxedInvocationView(t *testing.T) {
	defs := compileModel(t, "bkm_invocation_15.dmn")
	v, ok := defs.BoxedInvocation("Order Discount")
	if !ok {
		t.Fatal("Order Discount should be a boxed invocation")
	}
	if !v.Simple {
		t.Error("Order Discount invocation should be simple")
	}
	if v.Called != "Discount Rate" {
		t.Errorf("called = %q, want Discount Rate", v.Called)
	}
	if len(v.Bindings) != 1 || v.Bindings[0].Parameter != "total" || v.Bindings[0].Value != "Order Total" {
		t.Errorf("bindings = %+v, want [{total Order Total}]", v.Bindings)
	}

	if _, ok := defs.BoxedInvocation("Quick Discount"); ok {
		t.Error("Quick Discount is a literal expression; BoxedInvocation should report absent")
	}
}

// TestSetBoxedInvocation edits the bindings and checks the recompiled model
// evaluates with the new call, and the view round-trips.
func TestSetBoxedInvocation(t *testing.T) {
	src := readModel(t, "bkm_invocation_15.dmn")
	out, err := dmn.SetBoxedInvocation(src, "id_discount", dmn.InvocationEdit{
		Called:   "Discount Rate",
		Bindings: []dmn.InvocationBindingView{{Parameter: "total", Value: "Order Total * 2"}, {Parameter: "", Value: ""}},
	})
	if err != nil {
		t.Fatalf("SetBoxedInvocation: %v", err)
	}
	defs, diags, err := dmn.New().Compile(context.Background(), out)
	if err != nil || diags.HasErrors() {
		t.Fatalf("compile: err=%v diags=%+v", err, diags)
	}
	// The blank binding row is dropped; the argument is doubled.
	v, _ := defs.BoxedInvocation("id_discount")
	if len(v.Bindings) != 1 || v.Bindings[0].Value != "Order Total * 2" {
		t.Errorf("bindings not updated / blank not dropped: %+v", v.Bindings)
	}
	// Discount Rate = if total > 1000 then 0.2 else 0.1; doubled 60 = 120 → 0.1.
	dec, _ := defs.Decision("Order Discount")
	res, err := dec.Evaluate(context.Background(), dmn.Input{"Order Total": 60})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got := fmt.Sprint(res.Outputs["Order Discount"]); got != "0.1" {
		t.Errorf("Order Discount = %v, want 0.1", got)
	}
}

// TestSetBoxedInvocationRefuses rejects a missing called function, a duplicate or
// dangling binding, and a decision that already carries non-invocation logic.
func TestSetBoxedInvocationRefuses(t *testing.T) {
	src := readModel(t, "bkm_invocation_15.dmn")
	if _, err := dmn.SetBoxedInvocation(src, "id_discount", dmn.InvocationEdit{Called: "  ", Bindings: []dmn.InvocationBindingView{{Parameter: "total", Value: "1"}}}); err == nil {
		t.Error("SetBoxedInvocation should reject a missing called function")
	}
	if _, err := dmn.SetBoxedInvocation(src, "id_discount", dmn.InvocationEdit{Called: "Discount Rate", Bindings: []dmn.InvocationBindingView{{Parameter: "total", Value: "1"}, {Parameter: "total", Value: "2"}}}); err == nil {
		t.Error("SetBoxedInvocation should reject a duplicate parameter")
	}
	if _, err := dmn.SetBoxedInvocation(src, "id_discount", dmn.InvocationEdit{Called: "Discount Rate", Bindings: []dmn.InvocationBindingView{{Parameter: "", Value: "1"}}}); err == nil {
		t.Error("SetBoxedInvocation should reject an argument without a parameter")
	}
	if _, err := dmn.SetBoxedInvocation(readModel(t, "dish_15.dmn"), "Dish", dmn.InvocationEdit{Called: "f", Bindings: nil}); err == nil {
		t.Error("SetBoxedInvocation should refuse a decision-table decision")
	}
}

// TestCreateBoxedInvocation gives an undecided decision a fresh invocation and
// refuses one that already has logic.
func TestCreateBoxedInvocation(t *testing.T) {
	if _, err := dmn.CreateBoxedInvocation(readModel(t, "dish_15.dmn"), "Dish"); err == nil {
		t.Error("CreateBoxedInvocation should refuse a decision that already has logic")
	}
	const undecided = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" id="d" name="d" namespace="urn:x">
  <decision id="id_d" name="D"><variable name="D"/></decision>
</definitions>`
	out, err := dmn.CreateBoxedInvocation([]byte(undecided), "id_d")
	if err != nil {
		t.Fatalf("CreateBoxedInvocation: %v", err)
	}
	// The fresh invocation names a placeholder function and one binding; the view
	// round-trips even though the model has an "unknown function" diagnostic.
	defs, _, err := dmn.New().Compile(context.Background(), out)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	v, ok := defs.BoxedInvocation("id_d")
	if !ok || v.Called == "" || len(v.Bindings) != 1 {
		t.Errorf("fresh invocation not created: ok=%v view=%+v", ok, v)
	}
}

// TestBoxedInvocationNestedReadOnly reports an invocation with a nested
// non-literal binding as not simple, so the editor opens read-only.
func TestBoxedInvocationNestedReadOnly(t *testing.T) {
	const nested = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" id="d" name="d" namespace="urn:x">
  <businessKnowledgeModel id="id_bkm" name="F">
    <encapsulatedLogic><formalParameter name="p"/><literalExpression><text>p</text></literalExpression></encapsulatedLogic>
  </businessKnowledgeModel>
  <decision id="id_d" name="D">
    <variable name="D"/>
    <knowledgeRequirement><requiredKnowledge href="#id_bkm"/></knowledgeRequirement>
    <invocation>
      <literalExpression><text>F</text></literalExpression>
      <binding>
        <parameter name="p"/>
        <list><literalExpression><text>1</text></literalExpression></list>
      </binding>
    </invocation>
  </decision>
</definitions>`
	v, ok := mustCompile(t, nested).BoxedInvocation("D")
	if !ok {
		t.Fatal("D should be a boxed invocation")
	}
	if v.Simple {
		t.Error("an invocation with a nested list binding should not be simple")
	}
	if v.Called != "F" || len(v.Bindings) != 1 || v.Bindings[0].Parameter != "p" {
		t.Errorf("view = %+v", v)
	}
}
