package flow_test

import (
	"context"
	"errors"
	"testing"

	"github.com/pblumer/temis/dmn"
	"github.com/pblumer/temis/flow"
)

// A step that targets a decision service is typed like one that targets a
// decision (dmn ADR-0042). Before the service published an input schema, nothing
// about a service step's wiring or values was checked: Validate skipped it, and
// Evaluate built its input with no schema to coerce against or validate with.

func serviceFlow(in string) string {
	return `{
      "flow": "svc",
      "inputs": [{"name":"Applicant Age","type":"number"}],
      "steps": [
        {"id": "appr", "model": "sha256:svc-model", "decision": "Approval",
         "in": ` + in + `}
      ],
      "output": {"Result": "appr.Routing"}
    }`
}

func TestAServiceStepWiringIsChecked(t *testing.T) {
	r := resolver(t)

	diags := compile(t, serviceFlow(`{"Applicant Age": "Applicant Age", "Bogus": "Applicant Age"}`)).Validate(context.Background(), r)
	if !hasCode(diags, flow.CodeUnknownInput) {
		t.Errorf("wiring an input the service does not read: diagnostics = %v, want FLOW_UNKNOWN_INPUT", diags)
	}

	diags = compile(t, serviceFlow(`{}`)).Validate(context.Background(), r)
	if !hasCode(diags, flow.CodeInputUnwired) {
		t.Errorf("leaving the service's input unwired: diagnostics = %v, want FLOW_INPUT_UNWIRED", diags)
	}

	if diags := compile(t, serviceFlow(`{"Applicant Age": "Applicant Age"}`)).Validate(context.Background(), r); diags.HasErrors() {
		t.Errorf("a correctly wired service step: diagnostics = %v, want none", diags)
	}
}

func TestAServiceStepRefusesAWronglyTypedInput(t *testing.T) {
	f := compile(t, serviceFlow(`{"Applicant Age": "Applicant Age"}`))
	_, err := f.Evaluate(context.Background(), dmn.Input{"Applicant Age": "abc"}, resolver(t))
	var ie *dmn.InputError
	if !errors.As(err, &ie) {
		t.Fatalf("err = %v, want an *InputError — the service step used to evaluate this and answer", err)
	}
	if len(ie.Problems) != 1 || ie.Problems[0].Code != "TYPE_MISMATCH" || ie.Problems[0].Input != "Applicant Age" {
		t.Errorf("problems = %+v, want one TYPE_MISMATCH on Applicant Age", ie.Problems)
	}
}

// dmn renders a number as its exact decimal string, so a number handed from one
// step to the next arrives as text. A decision step coerces it back against the
// target's declared type; a service step had no schema to coerce against, so the
// text reached `Applicant Age >= 18`, the comparison was null, and the service
// answered DECLINE for an adult. The flow input carries the same string here.
func TestAServiceStepReceivesADecimalStringAsANumber(t *testing.T) {
	f := compile(t, serviceFlow(`{"Applicant Age": "Applicant Age"}`))
	res, err := f.Evaluate(context.Background(), dmn.Input{"Applicant Age": "30"}, resolver(t))
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got := res.Outputs["Result"]; got != "ACCEPT" {
		t.Errorf("Result = %v, want ACCEPT — the decimal string reached the service as text", got)
	}
}
