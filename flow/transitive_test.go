package flow_test

import (
	"context"
	"errors"
	"testing"

	"github.com/pblumer/temis/dmn"
	"github.com/pblumer/temis/flow"
)

const premiumID = "sha256:premium-model"

// premiumResolver builds a MapResolver over the premium model, whose decisions
// depend on leaf inputs transitively (FinalPremium → BasePremium → VehicleValue,
// TotalRiskScore → {BasePremium, RiskCategory}).
func premiumResolver(t *testing.T) flow.MapResolver {
	t.Helper()
	return flow.MapResolver{premiumID: loadModel(t, "testdata/premium.dmn")}
}

// TestFlowTransitiveInputWired: a step on FinalPremium may wire VehicleValue —
// which FinalPremium needs only transitively through BasePremium — and the value
// is passed all the way down, so the result is the correct computed number rather
// than the null the pre-fix per-decision path produced.
func TestFlowTransitiveInputWired(t *testing.T) {
	src := `{
      "flow":"premium",
      "inputs":[{"name":"VehicleValue","type":"number"},{"name":"RiskScore","type":"number"},{"name":"RegionLoad","type":"number"}],
      "steps":[
        {"id":"final","model":"sha256:premium-model","decision":"FinalPremium",
         "in":{"RiskScore":"RiskScore","RegionLoad":"RegionLoad","VehicleValue":"VehicleValue"}}
      ],
      "output":{"Premium":"final.FinalPremium"}
    }`
	f := compile(t, src)
	r := premiumResolver(t)

	// Wiring the transitive input is now valid (previously FLOW_UNKNOWN_INPUT).
	if d := f.Validate(context.Background(), r); d.HasErrors() {
		t.Fatalf("Validate returned diagnostics: %v", d)
	}

	res, err := f.Evaluate(context.Background(),
		dmn.Input{"VehicleValue": 1000, "RiskScore": 3, "RegionLoad": 100}, r)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	// BasePremium = 1000*5 = 5000; FinalPremium = 5000 + 3*10 + 100 = 5130.
	if got := res.Outputs["Premium"]; got != "5130" {
		t.Fatalf("Premium = %v (%T), want \"5130\" (transitive VehicleValue reached BasePremium)", got, got)
	}
}

// TestFlowCompositeDecisionNoDirectInputs: TotalRiskScore references only other
// decisions and declares no direct input; a step wiring its purely-transitive
// leaf inputs is valid and evaluates correctly.
func TestFlowCompositeDecisionNoDirectInputs(t *testing.T) {
	src := `{
      "flow":"total",
      "inputs":[{"name":"VehicleValue","type":"number"},{"name":"RiskScore","type":"number"}],
      "steps":[
        {"id":"total","model":"sha256:premium-model","decision":"TotalRiskScore",
         "in":{"VehicleValue":"VehicleValue","RiskScore":"RiskScore"}}
      ],
      "output":{"Total":"total.TotalRiskScore"}
    }`
	f := compile(t, src)
	r := premiumResolver(t)

	if d := f.Validate(context.Background(), r); d.HasErrors() {
		t.Fatalf("Validate returned diagnostics: %v", d)
	}

	res, err := f.Evaluate(context.Background(),
		dmn.Input{"VehicleValue": 1000, "RiskScore": 60}, r)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	// BasePremium = 5000; RiskCategory (60>50) = 2; TotalRiskScore = 5002.
	if got := res.Outputs["Total"]; got != "5002" {
		t.Fatalf("Total = %v, want \"5002\"", got)
	}
}

// TestFlowTransitiveUnknownRejected: a wiring target that appears nowhere in the
// target decision's cone is still FLOW_UNKNOWN_INPUT — the fix widens the allowed
// set to the reachable inputs, it does not accept genuine unknowns.
func TestFlowTransitiveUnknownRejected(t *testing.T) {
	src := `{
      "flow":"premium",
      "inputs":[{"name":"VehicleValue","type":"number"},{"name":"RiskScore","type":"number"},{"name":"RegionLoad","type":"number"}],
      "steps":[
        {"id":"final","model":"sha256:premium-model","decision":"FinalPremium",
         "in":{"RiskScore":"RiskScore","RegionLoad":"RegionLoad","VehicleValue":"VehicleValue","Bogus":"RiskScore"}}
      ]
    }`
	f := compile(t, src)
	diags := f.Validate(context.Background(), premiumResolver(t))
	if !hasCode(diags, flow.CodeUnknownInput) {
		t.Fatalf("want FLOW_UNKNOWN_INPUT for Bogus, got %v", diags)
	}
}

// TestFlowTransitiveRequiredUnwired: a required leaf input reached only
// transitively (VehicleValue via BasePremium) that is left unwired is reported as
// FLOW_INPUT_UNWIRED, not silently evaluated to null.
func TestFlowTransitiveRequiredUnwired(t *testing.T) {
	src := `{
      "flow":"premium",
      "inputs":[{"name":"RiskScore","type":"number"},{"name":"RegionLoad","type":"number"}],
      "steps":[
        {"id":"final","model":"sha256:premium-model","decision":"FinalPremium",
         "in":{"RiskScore":"RiskScore","RegionLoad":"RegionLoad"}}
      ]
    }`
	f := compile(t, src)
	diags := f.Validate(context.Background(), premiumResolver(t))
	if !hasCode(diags, flow.CodeInputUnwired) {
		t.Fatalf("want FLOW_INPUT_UNWIRED for VehicleValue, got %v", diags)
	}
}

// TestFlowTransitiveCoercion: a numeric transitive input fed from an earlier
// step's output (dmn renders numbers as decimal strings) must reach the sub-
// decision as a number, not a string. If coercion did not fire for the transitive
// input, BasePremium would see a string and the result would not be 140.
func TestFlowTransitiveCoercion(t *testing.T) {
	src := `{
      "flow":"coerce",
      "inputs":[{"name":"RiskScore","type":"number"},{"name":"RegionLoad","type":"number"},{"name":"CatRisk","type":"number"}],
      "steps":[
        {"id":"cat","model":"sha256:premium-model","decision":"RiskCategory","in":{"RiskScore":"CatRisk"}},
        {"id":"final","model":"sha256:premium-model","decision":"FinalPremium",
         "in":{"RiskScore":"RiskScore","RegionLoad":"RegionLoad","VehicleValue":"cat.RiskCategory"}}
      ],
      "output":{"Premium":"final.FinalPremium"}
    }`
	f := compile(t, src)
	res, err := f.Evaluate(context.Background(),
		dmn.Input{"RiskScore": 3, "RegionLoad": 100, "CatRisk": 60}, premiumResolver(t))
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	// cat: RiskCategory(60>50) = 2, rendered "2"; wired as VehicleValue and coerced
	// back to a number → BasePremium = 2*5 = 10; FinalPremium = 10 + 30 + 100 = 140.
	if got := res.Outputs["Premium"]; got != "140" {
		t.Fatalf("Premium = %v (%T), want \"140\" (transitive numeric coercion)", got, got)
	}
}

// TestFlowTransitiveTypeMismatch: a value that passes wiring (correct name) but is
// the wrong type for a transitive leaf input surfaces as an error, not a silent
// null — the cone-scoped strict check still catches it.
func TestFlowTransitiveTypeMismatch(t *testing.T) {
	src := `{
      "flow":"premium",
      "inputs":[{"name":"VehicleValue","type":"string"},{"name":"RiskScore","type":"number"},{"name":"RegionLoad","type":"number"}],
      "steps":[
        {"id":"final","model":"sha256:premium-model","decision":"FinalPremium",
         "in":{"RiskScore":"RiskScore","RegionLoad":"RegionLoad","VehicleValue":"VehicleValue"}}
      ],
      "output":{"Premium":"final.FinalPremium"}
    }`
	f := compile(t, src)
	// "oops" is not numeric, so coerce leaves it a string and the reachable-schema
	// validation rejects it (VehicleValue expects number).
	_, err := f.Evaluate(context.Background(),
		dmn.Input{"VehicleValue": "oops", "RiskScore": 3, "RegionLoad": 100}, premiumResolver(t))
	if err == nil {
		t.Fatalf("expected a type-mismatch error for the transitive input")
	}
	var ie *dmn.InputError
	if !errors.As(err, &ie) {
		t.Fatalf("want *dmn.InputError, got %T: %v", err, err)
	}
}
