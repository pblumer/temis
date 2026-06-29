package dmn_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// TestDecisionServiceEncapsulated covers WP-29: an encapsulated decision is
// evaluated internally from the supplied input data.
func TestDecisionServiceEncapsulated(t *testing.T) {
	defs := compileModel(t, "decisionservice_15.dmn")
	svc, err := defs.Service("Approval")
	if err != nil {
		t.Fatal(err)
	}

	res, err := svc.Evaluate(context.Background(), dmn.Input{"Applicant Age": 20})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outputs["Routing"] != "ACCEPT" {
		t.Errorf("Approval(age 20).Routing = %v, want ACCEPT", res.Outputs["Routing"])
	}
	want := map[string]any{"Eligibility": "ELIGIBLE", "Routing": "ACCEPT"}
	if !reflect.DeepEqual(res.Decisions, want) {
		t.Errorf("Decisions = %#v, want %#v", res.Decisions, want)
	}
}

// TestDecisionServiceInputDecisionBoundary covers WP-29: an input decision is
// supplied by the caller and is never computed by the service.
func TestDecisionServiceInputDecisionBoundary(t *testing.T) {
	defs := compileModel(t, "decisionservice_15.dmn")
	svc, _ := defs.Service("Routing Only")

	// Supplied input decision is used directly.
	res, err := svc.Evaluate(context.Background(), dmn.Input{"Eligibility": "ELIGIBLE"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outputs["Routing"] != "ACCEPT" {
		t.Errorf("Routing Only(Eligibility=ELIGIBLE) = %v, want ACCEPT", res.Outputs["Routing"])
	}
	// The boundary decision must not appear among evaluated decisions.
	if _, ok := res.Decisions["Eligibility"]; ok {
		t.Errorf("input decision should not be evaluated: %#v", res.Decisions)
	}

	// Without it supplied, the boundary is null (not computed from Applicant Age),
	// so Routing declines rather than chaining into Eligibility.
	res, _ = svc.Evaluate(context.Background(), dmn.Input{"Applicant Age": 20})
	if res.Outputs["Routing"] != "DECLINE" {
		t.Errorf("Routing Only(age 20, no Eligibility) = %v, want DECLINE (boundary not computed)", res.Outputs["Routing"])
	}
}

func TestServiceNotFound(t *testing.T) {
	defs := compileModel(t, "decisionservice_15.dmn")
	if _, err := defs.Service("Nope"); err == nil {
		t.Error("Service(unknown) should error")
	}
}

func TestServiceAccessors(t *testing.T) {
	defs := compileModel(t, "decisionservice_15.dmn")
	svc, _ := defs.Service("Approval")
	if svc.Name() != "Approval" || svc.ID() != "id_approval" {
		t.Errorf("accessors = %q/%q, want Approval/id_approval", svc.Name(), svc.ID())
	}
}

const serviceBadOutput = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/badsvc" name="BadSvc" id="def_bad">
  <decisionService id="svc" name="Bad">
    <outputDecision href="#missing"/>
  </decisionService>
</definitions>`

func TestServiceUnresolvedOutput(t *testing.T) {
	_, diags, err := dmn.New().Compile(context.Background(), []byte(serviceBadOutput))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range diags {
		if d.Code == "SERVICE_OUTPUT_UNRESOLVED" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected SERVICE_OUTPUT_UNRESOLVED, got %+v", diags)
	}
}
