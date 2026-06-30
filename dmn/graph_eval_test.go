package dmn_test

import (
	"context"
	"errors"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// The routing fixture is the canonical transitive chain: Applicant Age (input) →
// Eligibility (decision) → Routing (decision). Routing names only Eligibility, so
// it does not declare Applicant Age directly — the case the per-decision strict
// schema rejects but a graph evaluation must accept.

func TestEvaluateGraphReturnsEveryDecision(t *testing.T) {
	defs := compileModel(t, "routing_13.dmn")

	res, err := defs.EvaluateGraph(context.Background(), dmn.Input{"Applicant Age": 20})
	if err != nil {
		t.Fatalf("EvaluateGraph: %v", err)
	}
	if len(res.Errors) != 0 {
		t.Fatalf("unexpected per-decision errors: %+v", res.Errors)
	}
	if got := res.Values["Eligibility"]; got != "ELIGIBLE" {
		t.Errorf("Eligibility = %v, want ELIGIBLE", got)
	}
	if got := res.Values["Routing"]; got != "ACCEPT" {
		t.Errorf("Routing = %v, want ACCEPT", got)
	}
}

func TestEvaluateGraphStrictAcceptsTransitiveInput(t *testing.T) {
	defs := compileModel(t, "routing_13.dmn")

	// Applicant Age feeds Routing only through Eligibility; strict graph
	// validation must accept it rather than reporting UNKNOWN_INPUT (the bug a
	// per-decision strict evaluation of "Routing" hit).
	res, err := defs.EvaluateGraph(context.Background(), dmn.Input{"Applicant Age": 16}, dmn.WithStrictInput())
	if err != nil {
		t.Fatalf("strict EvaluateGraph: %v", err)
	}
	if got := res.Values["Routing"]; got != "DECLINE" {
		t.Errorf("Routing = %v, want DECLINE", got)
	}
}

func TestEvaluateGraphStrictRejectsUnknownInput(t *testing.T) {
	defs := compileModel(t, "routing_13.dmn")

	_, err := defs.EvaluateGraph(context.Background(), dmn.Input{"Applicant Age": 20, "Nope": 1}, dmn.WithStrictInput())
	var ie *dmn.InputError
	if !errors.As(err, &ie) {
		t.Fatalf("want *InputError, got %v", err)
	}
	found := false
	for _, p := range ie.Problems {
		if p.Code == "UNKNOWN_INPUT" && p.Input == "Nope" {
			found = true
		}
	}
	if !found {
		t.Errorf("want UNKNOWN_INPUT on Nope, got %+v", ie.Problems)
	}
}

func TestEvaluateGraphTracePerDecision(t *testing.T) {
	defs := compileModel(t, "dish_15.dmn")

	res, err := defs.EvaluateGraph(context.Background(), dmn.Input{"Season": "Winter", "Guest Count": 8}, dmn.WithTrace())
	if err != nil {
		t.Fatalf("EvaluateGraph: %v", err)
	}
	if res.Traces["Dish"] == nil {
		t.Errorf("want a trace for Dish, got %+v", res.Traces)
	}
}

// The chain fixture is Age (input) → Category (decision TABLE) → Greeting
// (decision LITERAL). Each decision's trace must show only its OWN logic: the
// literal Greeting must NOT inherit Category's table (the cone-trace bug), and
// Category's trace must be its own single table.
func TestEvaluateGraphTraceIsPerDecisionOwnLogic(t *testing.T) {
	defs := compileModel(t, "chain_15.dmn")

	res, err := defs.EvaluateGraph(context.Background(), dmn.Input{"Age": 20}, dmn.WithTrace())
	if err != nil {
		t.Fatalf("EvaluateGraph: %v", err)
	}
	if res.Values["Category"] != "adult" || res.Values["Greeting"] != "Willkommen" {
		t.Fatalf("values = %+v, want Category=adult, Greeting=Willkommen", res.Values)
	}
	// Category is a decision table: exactly one (its own) table, and it tests Age.
	cat := res.Traces["Category"]
	if cat == nil || len(cat.Tables) != 1 {
		t.Fatalf("Category trace = %+v, want exactly one table", cat)
	}
	if len(cat.Tables[0].Inputs) == 0 || cat.Tables[0].Inputs[0].Expression != "Age" {
		t.Errorf("Category table input = %+v, want it to test Age", cat.Tables[0].Inputs)
	}
	// Greeting is a literal expression: it must have NO table trace (it must not
	// show Category's table).
	if g := res.Traces["Greeting"]; g != nil && len(g.Tables) > 0 {
		t.Errorf("Greeting (literal) should have no table trace, got %+v", g.Tables)
	}
}

func TestModelInputSchemaUnionsLeafInputs(t *testing.T) {
	defs := compileModel(t, "routing_13.dmn")

	got := map[string]dmn.InputField{}
	for _, f := range defs.ModelInputSchema() {
		got[f.Name] = f
	}
	if len(got) != 1 {
		t.Fatalf("want 1 leaf input, got %d (%+v)", len(got), defs.ModelInputSchema())
	}
	if f, ok := got["Applicant Age"]; !ok || !f.Required {
		t.Errorf("Applicant Age = %+v, want present and required", f)
	}
}
