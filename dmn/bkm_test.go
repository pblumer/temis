package dmn_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// TestBKMFunctionView checks the BKM view exposes the function's parameters and
// literal body.
func TestBKMFunctionView(t *testing.T) {
	defs := compileModel(t, "bkm_invocation_15.dmn")
	v, ok := defs.BKMFunction("Discount Rate")
	if !ok {
		t.Fatal("Discount Rate BKM not found")
	}
	if !v.Simple {
		t.Error("literal-body BKM should be simple (editable)")
	}
	if len(v.Params) != 1 || v.Params[0].Name != "total" || v.Params[0].TypeRef != "number" {
		t.Errorf("params = %+v, want one 'total' : number", v.Params)
	}
	if v.BodyText != "if total > 1000 then 0.2 else 0.1" {
		t.Errorf("body = %q, want the discount expression", v.BodyText)
	}
}

// TestFunctions lists the model's user-defined functions (its BKMs) with their
// parameter names — the signatures the modeler hands to its FEEL editors so a
// call to a BKM (a BKM's own recursion included) completes and validates as a
// known function.
func TestFunctions(t *testing.T) {
	defs := compileModel(t, "bkm_invocation_15.dmn")
	fns := defs.Functions()
	if len(fns) != 1 {
		t.Fatalf("Functions() = %+v, want one BKM", fns)
	}
	if fns[0].Name != "Discount Rate" {
		t.Errorf("name = %q, want %q", fns[0].Name, "Discount Rate")
	}
	if len(fns[0].Params) != 1 || fns[0].Params[0] != "total" {
		t.Errorf("params = %+v, want [total]", fns[0].Params)
	}
}

// TestSetBKMFunction edits a BKM's body and parameters and checks the recompiled
// model invokes the new logic.
func TestSetBKMFunction(t *testing.T) {
	src := readModel(t, "bkm_invocation_15.dmn")

	out, err := dmn.SetBKMFunction(src, "id_rate", dmn.BKMFunctionEdit{
		Params:      []dmn.BKMParam{{Name: "total", TypeRef: "number"}},
		BodyText:    "if total > 500 then 0.5 else 0.0",
		BodyTypeRef: "number",
	})
	if err != nil {
		t.Fatalf("SetBKMFunction: %v", err)
	}

	defs, diags, err := dmn.New().Compile(context.Background(), out)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("diagnostics: %+v", diags)
	}
	if v, _ := defs.BKMFunction("Discount Rate"); v.BodyText != "if total > 500 then 0.5 else 0.0" {
		t.Errorf("body not updated: %q", v.BodyText)
	}
	// The decision that invokes the BKM now reflects the new threshold.
	dec, err := defs.Decision("Order Discount")
	if err != nil {
		t.Fatalf("decision: %v", err)
	}
	res, err := dec.Evaluate(context.Background(), dmn.Input{"Order Total": 600})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got := fmt.Sprint(res.Outputs["Order Discount"]); got != "0.5" {
		t.Errorf("Order Discount = %v, want 0.5 (new BKM logic)", res.Outputs["Order Discount"])
	}
}

// TestSetBKMFunctionRejects checks empty body and unknown BKM are rejected.
func TestSetBKMFunctionRejects(t *testing.T) {
	src := readModel(t, "bkm_invocation_15.dmn")
	if _, err := dmn.SetBKMFunction(src, "id_rate", dmn.BKMFunctionEdit{BodyText: "  "}); err == nil {
		t.Error("expected error for empty BKM body")
	}
	if _, err := dmn.SetBKMFunction(src, "nope", dmn.BKMFunctionEdit{BodyText: "1"}); err == nil {
		t.Error("expected error for unknown BKM")
	}
}
