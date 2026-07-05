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

// TestDecisionServiceInvokedFromFEEL covers WP-41.20: a decision service can be
// called by name from a decision's FEEL expression (DMN §10.4, TCK 0085). Its
// parameters are the input data then the input decisions; a single output
// decision yields that decision's value.
func TestDecisionServiceInvokedFromFEEL(t *testing.T) {
	const model = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" name="svc" namespace="t">
  <decisionService name="Svc" id="_svc">
    <variable name="Svc"/>
    <outputDecision href="#_out"/>
    <inputDecision href="#_arg"/>
  </decisionService>
  <decision name="out" id="_out">
    <variable name="out"/>
    <informationRequirement><requiredDecision href="#_arg"/></informationRequirement>
    <literalExpression><text>"foo " + arg</text></literalExpression>
  </decision>
  <decision name="arg" id="_arg">
    <variable name="arg"/>
    <literalExpression><text>"unused"</text></literalExpression>
  </decision>
  <decision name="caller" id="_caller">
    <variable name="caller"/>
    <knowledgeRequirement><requiredKnowledge href="#_svc"/></knowledgeRequirement>
    <literalExpression><text>Svc("bar")</text></literalExpression>
  </decision>
</definitions>`
	defs := mustCompile(t, model)
	dec, err := defs.Decision("caller")
	if err != nil {
		t.Fatal(err)
	}
	res, err := dec.Evaluate(context.Background(), dmn.Input{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outputs["caller"] != "foo bar" {
		t.Errorf(`Svc("bar") = %v, want "foo bar"`, res.Outputs["caller"])
	}
}

// TestDecisionServiceInvokedMultiOutput covers WP-41.20: invoking a service with
// more than one output decision yields a context keyed by output name.
func TestDecisionServiceInvokedMultiOutput(t *testing.T) {
	const model = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" name="svc2" namespace="t">
  <decisionService name="Two" id="_two">
    <variable name="Two"/>
    <outputDecision href="#_a"/>
    <outputDecision href="#_b"/>
  </decisionService>
  <decision name="a" id="_a"><variable name="a"/><literalExpression><text>"A"</text></literalExpression></decision>
  <decision name="b" id="_b"><variable name="b"/><literalExpression><text>"B"</text></literalExpression></decision>
  <decision name="caller2" id="_caller2">
    <variable name="caller2"/>
    <knowledgeRequirement><requiredKnowledge href="#_two"/></knowledgeRequirement>
    <literalExpression><text>Two().a + Two().b</text></literalExpression>
  </decision>
</definitions>`
	defs := mustCompile(t, model)
	dec, _ := defs.Decision("caller2")
	res, err := dec.Evaluate(context.Background(), dmn.Input{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outputs["caller2"] != "AB" {
		t.Errorf("Two().a + Two().b = %v, want AB", res.Outputs["caller2"])
	}
}
