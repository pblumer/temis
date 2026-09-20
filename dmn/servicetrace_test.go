package dmn_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// A decision service could not be traced: CompiledService.Evaluate took no
// EvalOption, so WithTrace had nowhere to go and a caller who reached a model
// through its published interface got outputs and nothing about how they were
// reached. The decisions inside a service are usually tables — exactly the thing
// a trace exists to show — so the gap was widest where it was least affordable:
// a caller accounting for a case downstream cannot tell "no rules were recorded"
// from "no rules matched" (see the consumer report in Atlas ADR-0398).
//
// These tests drive the seam from outside the package, over a model whose
// decisions are real tables, because a trace of literal expressions proves
// nothing about rules.

func compileServiceModel(t *testing.T, name string) *dmn.CompiledService {
	t.Helper()
	defs := compileModel(t, "decisionservice_trace_15.dmn")
	svc, err := defs.Service(name)
	if err != nil {
		t.Fatalf("service %q: %v", name, err)
	}
	return svc
}

// The default path stays as it was: no option, no trace, no allocation for one.
func TestServiceNoTraceByDefault(t *testing.T) {
	svc := compileServiceModel(t, "Approval")
	res, err := svc.Evaluate(context.Background(), dmn.Input{"Applicant Age": 20})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.Trace != nil {
		t.Errorf("Trace should be nil without WithTrace, got %+v", res.Trace)
	}
}

// WithTrace over a service records every table the service actually ran, in the
// order it ran them — the encapsulated decision first, because the output
// decision needs its result.
func TestServiceTraceCoversEveryTableItRan(t *testing.T) {
	svc := compileServiceModel(t, "Approval")
	res, err := svc.Evaluate(context.Background(), dmn.Input{"Applicant Age": 20}, dmn.WithTrace())
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.Outputs["Routing"] != "ACCEPT" {
		t.Fatalf("Routing = %v, want ACCEPT", res.Outputs["Routing"])
	}
	if res.Trace == nil {
		t.Fatal("Trace is nil with WithTrace")
	}
	if len(res.Trace.Tables) != 2 {
		t.Fatalf("traced %d tables, want 2 (Eligibility then Routing): %+v", len(res.Trace.Tables), res.Trace)
	}

	// Eligibility ran first: 20 is not < 18, and is >= 18, so rule 2 carried it.
	elig := res.Trace.Tables[0]
	if elig.HitPolicy != "U" {
		t.Errorf("Eligibility hit policy = %q, want U", elig.HitPolicy)
	}
	if got := len(elig.Inputs); got != 1 || elig.Inputs[0].Expression != "Applicant Age" {
		t.Fatalf("Eligibility inputs = %+v, want one column over Applicant Age", elig.Inputs)
	}
	if !reflect.DeepEqual(elig.Matched, []int{1}) {
		t.Errorf("Eligibility matched = %v, want [1]", elig.Matched)
	}
	if got := elig.Rules[1].Outputs; len(got) != 1 || got[0] != "ELIGIBLE" {
		t.Errorf("Eligibility rule 2 outputs = %v, want [ELIGIBLE]", got)
	}
	// And the rule that did not fire says which condition ruled it out, which is
	// the half of a trace somebody debugging is actually reading.
	if elig.Rules[0].Matched {
		t.Error("Eligibility rule 1 (< 18) matched for age 20")
	}
	if c := elig.Rules[0].Conditions; len(c) != 1 || c[0].Entry != "< 18" || c[0].Matched {
		t.Errorf("Eligibility rule 1 conditions = %+v, want the failed < 18 test", c)
	}

	// Routing ran second, over the value Eligibility produced.
	route := res.Trace.Tables[1]
	if got := len(route.Inputs); got != 1 || route.Inputs[0].Expression != "Eligibility" {
		t.Fatalf("Routing inputs = %+v, want one column over Eligibility", route.Inputs)
	}
	if route.Inputs[0].Value != "ELIGIBLE" {
		t.Errorf("Routing read Eligibility = %v, want ELIGIBLE", route.Inputs[0].Value)
	}
	if !reflect.DeepEqual(route.Matched, []int{0}) {
		t.Errorf("Routing matched = %v, want [0]", route.Matched)
	}
}

// The boundary holds in the trace as well as in the result: an input decision is
// supplied, never computed, so its table never ran and must not be reported as
// though it had. A trace that claimed otherwise would be an account of an
// evaluation that did not happen.
func TestServiceTraceStopsAtTheBoundary(t *testing.T) {
	svc := compileServiceModel(t, "Routing Only")
	res, err := svc.Evaluate(context.Background(),
		dmn.Input{"Eligibility": "ELIGIBLE"}, dmn.WithTrace())
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.Outputs["Routing"] != "ACCEPT" {
		t.Fatalf("Routing = %v, want ACCEPT", res.Outputs["Routing"])
	}
	if res.Trace == nil || len(res.Trace.Tables) != 1 {
		t.Fatalf("traced %d tables, want only Routing's: %+v", len(res.Trace.Tables), res.Trace)
	}
	if got := res.Trace.Tables[0].Inputs[0].Expression; got != "Eligibility" {
		t.Errorf("traced table reads %q, want the Routing table over Eligibility", got)
	}
}

// Asking for a trace must not change what the service answers. The trace is an
// observation of the evaluation, not a second evaluation.
func TestServiceTraceDoesNotChangeTheResult(t *testing.T) {
	for _, c := range []struct {
		service string
		in      dmn.Input
	}{
		{"Approval", dmn.Input{"Applicant Age": 20}},
		{"Approval", dmn.Input{"Applicant Age": 12}},
		{"Routing Only", dmn.Input{"Eligibility": "INELIGIBLE"}},
	} {
		t.Run(c.service, func(t *testing.T) {
			svc := compileServiceModel(t, c.service)
			plain, err := svc.Evaluate(context.Background(), c.in)
			if err != nil {
				t.Fatalf("evaluate: %v", err)
			}
			traced, err := svc.Evaluate(context.Background(), c.in, dmn.WithTrace())
			if err != nil {
				t.Fatalf("evaluate with trace: %v", err)
			}
			if !reflect.DeepEqual(plain.Outputs, traced.Outputs) {
				t.Errorf("Outputs differ with and without a trace: %#v vs %#v", plain.Outputs, traced.Outputs)
			}
			if !reflect.DeepEqual(plain.Decisions, traced.Decisions) {
				t.Errorf("Decisions differ with and without a trace: %#v vs %#v", plain.Decisions, traced.Decisions)
			}
		})
	}
}

// An unsupplied boundary evaluates to null rather than being computed, and that
// stays true when a trace is asked for: Routing's table runs and matches
// nothing, which is a real answer about the model and has to be visible.
func TestServiceTraceShowsAnUnsuppliedBoundaryMatchingNothing(t *testing.T) {
	svc := compileServiceModel(t, "Routing Only")
	res, err := svc.Evaluate(context.Background(), dmn.Input{}, dmn.WithTrace())
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.Outputs["Routing"] != nil {
		t.Errorf("Routing = %v, want nil for an unsupplied boundary", res.Outputs["Routing"])
	}
	if res.Trace == nil || len(res.Trace.Tables) != 1 {
		t.Fatalf("traced %d tables, want Routing's: %+v", len(res.Trace.Tables), res.Trace)
	}
	if got := res.Trace.Tables[0].Matched; len(got) != 0 {
		t.Errorf("matched = %v, want none — no rule tests true against null", got)
	}
}
