package dmn_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// literalModel is a minimal DMN 1.5 document: one decision computing a discount
// from two inputs via a literal FEEL expression. It exercises the Compile →
// Decision → Evaluate path without a decision table.
const literalModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" id="d1" name="Discount" namespace="ex">
  <inputData id="id_amount" name="Amount"/>
  <inputData id="id_member" name="Member"/>
  <decision id="id_discount" name="Discount">
    <informationRequirement><requiredInput href="#id_amount"/></informationRequirement>
    <informationRequirement><requiredInput href="#id_member"/></informationRequirement>
    <literalExpression>
      <text>if Member then Amount * 0.1 else 0</text>
    </literalExpression>
  </decision>
</definitions>`

func TestCompileAndEvaluateLiteral(t *testing.T) {
	eng := dmn.New()
	defs, diags, err := eng.Compile(context.Background(), []byte(literalModel))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected compile errors: %+v", diags)
	}

	dec, err := defs.Decision("Discount")
	if err != nil {
		t.Fatalf("lookup decision: %v", err)
	}

	res, err := dec.Evaluate(context.Background(), dmn.Input{"Amount": 200, "Member": true})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got := res.Outputs["Discount"]; got != "20" {
		t.Errorf("Discount = %v (%T), want \"20\"", got, got)
	}

	res, err = dec.Evaluate(context.Background(), dmn.Input{"Amount": 200, "Member": false})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got := res.Outputs["Discount"]; got != "0" {
		t.Errorf("Discount (non-member) = %v, want \"0\"", got)
	}
}

func TestLookupByID(t *testing.T) {
	eng := dmn.New()
	defs, _, err := eng.Compile(context.Background(), []byte(literalModel))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if _, err := defs.Decision("id_discount"); err != nil {
		t.Errorf("lookup by ID failed: %v", err)
	}
	if _, err := defs.Decision("Nope"); err == nil {
		t.Error("expected error for unknown decision")
	}
}

func TestIndex(t *testing.T) {
	eng := dmn.New()
	defs, _, err := eng.Compile(context.Background(), []byte(literalModel))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	idx := defs.Index()
	if !reflect.DeepEqual(idx.Decisions, []string{"Discount"}) {
		t.Errorf("Decisions = %v, want [Discount]", idx.Decisions)
	}
	if !reflect.DeepEqual(idx.Inputs, []string{"Amount", "Member"}) {
		t.Errorf("Inputs = %v, want [Amount Member]", idx.Inputs)
	}
}

func TestMalformedXMLIsError(t *testing.T) {
	eng := dmn.New()
	if _, _, err := eng.Compile(context.Background(), []byte("<not-dmn")); err == nil {
		t.Fatal("expected error for malformed XML")
	}
}

func TestCompileErrorAsDiagnostic(t *testing.T) {
	const bad = `<?xml version="1.0"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" id="d" name="Bad" namespace="ex">
  <decision id="id_bad" name="Bad">
    <literalExpression><text>1 +</text></literalExpression>
  </decision>
</definitions>`
	eng := dmn.New()
	defs, diags, err := eng.Compile(context.Background(), []byte(bad))
	if err != nil {
		t.Fatalf("compile returned hard error: %v", err)
	}
	if !diags.HasErrors() {
		t.Fatal("expected a compile-error diagnostic")
	}
	// The model still loads; the broken decision is simply not executable.
	if _, err := defs.Decision("Bad"); err == nil {
		t.Error("expected Decision to reject a non-compiled decision")
	}
}

// TestDishEndToEnd loads the dish_15.dmn fixture through the public API and
// checks the documented quickstart path end to end.
func TestDishEndToEnd(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "internal", "xml", "testdata", "models", "dish_15.dmn"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	eng := dmn.New()
	defs, diags, err := eng.Compile(context.Background(), data)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected compile errors: %+v", diags)
	}

	dec, err := defs.Decision("Dish")
	if err != nil {
		t.Fatalf("lookup Dish: %v", err)
	}

	cases := []struct {
		season string
		guests int
		want   string
	}{
		{"Fall", 4, "Spareribs"},
		{"Winter", 4, "Roastbeef"},
		{"Spring", 6, "Steak"},
		{"Summer", 10, "Stew"},
	}
	for _, c := range cases {
		res, err := dec.Evaluate(context.Background(), dmn.Input{
			"Season":      c.season,
			"Guest Count": c.guests,
		})
		if err != nil {
			t.Fatalf("evaluate %s/%d: %v", c.season, c.guests, err)
		}
		if got := res.Outputs["Dish"]; got != c.want {
			t.Errorf("Dish(%s, %d) = %v, want %q", c.season, c.guests, got, c.want)
		}
	}
}
