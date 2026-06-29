package dmn_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// TestLiteralExpressionView checks the literal-expression view exposes the FEEL
// text and type of a literal decision, and reports absent for a table decision.
func TestLiteralExpressionView(t *testing.T) {
	defs := compileModel(t, "pricing_15.dmn")
	lv, ok := defs.LiteralExpression("Net Total")
	if !ok {
		t.Fatal("Net Total should be a literal expression")
	}
	if lv.Text != "Unit Price * Quantity" || lv.TypeRef != "number" {
		t.Errorf("literal view = %+v, want 'Unit Price * Quantity' : number", lv)
	}

	dish := compileModel(t, "dish_15.dmn")
	if _, ok := dish.LiteralExpression("Dish"); ok {
		t.Error("Dish is a decision table; LiteralExpression should report absent")
	}
}

// TestSetLiteralExpression edits a literal decision's text and checks the
// recompiled model evaluates with the new expression.
func TestSetLiteralExpression(t *testing.T) {
	src := readModel(t, "pricing_15.dmn")

	out, err := dmn.SetLiteralExpression(src, "id_net", "Unit Price * Quantity * 2", "number")
	if err != nil {
		t.Fatalf("SetLiteralExpression: %v", err)
	}
	defs, diags, err := dmn.New().Compile(context.Background(), out)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("diagnostics: %+v", diags)
	}
	if lv, _ := defs.LiteralExpression("Net Total"); !strings.Contains(lv.Text, "* 2") {
		t.Errorf("literal text not updated: %q", lv.Text)
	}
	dec, err := defs.Decision("Net Total")
	if err != nil {
		t.Fatalf("decision: %v", err)
	}
	res, err := dec.Evaluate(context.Background(), dmn.Input{"Unit Price": 10, "Quantity": 3})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	// 10 * 3 * 2 = 60
	if got := fmt.Sprint(res.Outputs["Net Total"]); got != "60" {
		t.Errorf("Net Total = %v, want 60 (edited expression)", res.Outputs["Net Total"])
	}
}

// TestSetLiteralExpressionRejects checks empty text and a table decision are
// rejected.
func TestSetLiteralExpressionRejects(t *testing.T) {
	if _, err := dmn.SetLiteralExpression(readModel(t, "pricing_15.dmn"), "id_net", "  ", "number"); err == nil {
		t.Error("expected error for empty literal text")
	}
	if _, err := dmn.SetLiteralExpression(readModel(t, "dish_15.dmn"), "id_dish", "1", ""); err == nil {
		t.Error("expected error setting a literal on a decision-table decision")
	}
}

// TestCreateLiteralOnNewDecision checks a literal can be set on a freshly added
// (undecided) decision, turning it into an evaluable literal decision.
func TestCreateLiteralOnNewDecision(t *testing.T) {
	src := readModel(t, "dish_15.dmn")
	g := graphEdit(t, src)
	g.Nodes = append(g.Nodes, dmn.GraphNodeEdit{ID: "id_const", Type: "decision", Name: "Const", X: 600, Y: 260, Width: 150, Height: 70})
	withDec, err := dmn.ApplyGraph(src, g)
	if err != nil {
		t.Fatalf("ApplyGraph: %v", err)
	}
	out, err := dmn.SetLiteralExpression(withDec, "id_const", "42", "number")
	if err != nil {
		t.Fatalf("SetLiteralExpression: %v", err)
	}
	defs, _, err := dmn.New().Compile(context.Background(), out)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	dec, err := defs.Decision("Const")
	if err != nil {
		t.Fatalf("Const not evaluable: %v", err)
	}
	res, err := dec.Evaluate(context.Background(), dmn.Input{})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got := fmt.Sprint(res.Outputs["Const"]); got != "42" {
		t.Errorf("Const = %v, want 42", res.Outputs["Const"])
	}
}
