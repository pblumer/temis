package service

import (
	"context"
	"net/http"
	"testing"

	"connectrpc.com/connect"

	dmnv1 "github.com/pblumer/temis/internal/gen/dmnv1"
)

// discountModelXML is a literal-expression decision requiring two inputs. Omitting
// one input makes Evaluate fail with a non-InputError EvalError at runtime, which
// drives the generic evaluation-failure branches (HTTP EVALUATION_FAILED, gRPC
// FailedPrecondition).
const discountModelXML = `<?xml version="1.0" encoding="UTF-8"?>
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

// noLogicModelXML has an undecided decision, so compiling it produces a
// DECISION_NO_LOGIC diagnostic. It drives diagnosticsPB's non-empty mapping over
// gRPC Compile.
const noLogicModelXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" id="d2" name="NoLogic" namespace="ex">
  <inputData id="id_x" name="X"/>
  <decision id="id_undecided" name="Undecided">
    <informationRequirement><requiredInput href="#id_x"/></informationRequirement>
  </decision>
</definitions>`

// inputsOnlyModelXML declares only input data and no decisions, so its index has
// zero decisions — driving schemaOf's empty-decisions early return (nil schema).
const inputsOnlyModelXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" id="d3" name="InputsOnly" namespace="ex">
  <inputData id="id_a" name="A"/>
  <inputData id="id_b" name="B"/>
</definitions>`

// TestSchemaOfEmptyDecisions covers schemaOf's nil-on-empty-decisions branch via
// a model with no decisions: its response carries no schema.
func TestSchemaOfEmptyDecisions(t *testing.T) {
	h := newTestServer(t)
	rec := do(t, h, "POST", "/v1/models", "application/xml", []byte(inputsOnlyModelXML))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body %s)", rec.Code, rec.Body)
	}
	resp := decode[modelResponse](t, rec)
	if len(resp.Decisions) != 0 {
		t.Fatalf("decisions = %v, want none", resp.Decisions)
	}
	if resp.Schema != nil {
		t.Errorf("schema = %v, want nil for a decision-less model", resp.Schema)
	}
}

// TestEvaluateGenericFailure covers the evaluate() EVALUATION_FAILED branch: a
// missing required input makes Evaluate fail with an EvalError that is not an
// InputError, so the handler answers 422 EVALUATION_FAILED.
func TestEvaluateGenericFailure(t *testing.T) {
	h := newTestServer(t)
	id := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", []byte(discountModelXML))).ModelID

	body := mustJSON(t, evaluateModelRequest{Decision: "Discount", Input: map[string]any{"Amount": 200}})
	rec := do(t, h, "POST", "/v1/models/"+id+"/evaluate", "application/json", body)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 (body %s)", rec.Code, rec.Body)
	}
	if p := decode[problem](t, rec); p.Code != "EVALUATION_FAILED" {
		t.Errorf("code = %q, want EVALUATION_FAILED", p.Code)
	}
}

// TestGRPCEvaluateFailedPrecondition covers evalOnce's FailedPrecondition branch
// (a non-InputError evaluation error).
func TestGRPCEvaluateFailedPrecondition(t *testing.T) {
	client := newGRPCClient(t)
	_, err := client.Evaluate(context.Background(), connect.NewRequest(&dmnv1.EvaluateRequest{
		Model:    &dmnv1.EvaluateRequest_Xml{Xml: []byte(discountModelXML)},
		Decision: "Discount",
		Input:    mustStruct(t, map[string]any{"Amount": 200}),
	}))
	if err == nil {
		t.Fatal("missing required input: want error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("code = %v, want FailedPrecondition", connect.CodeOf(err))
	}
}

// TestGRPCCompileDiagnostics covers diagnosticsPB's non-empty mapping: compiling
// a model with an undecided decision yields a diagnostic in the response.
func TestGRPCCompileDiagnostics(t *testing.T) {
	client := newGRPCClient(t)
	resp, err := client.Compile(context.Background(), connect.NewRequest(&dmnv1.CompileRequest{Xml: []byte(noLogicModelXML)}))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(resp.Msg.GetDiagnostics()) == 0 {
		t.Fatal("expected a DECISION_NO_LOGIC diagnostic, got none")
	}
}

// TestClioSinkSubjectPrefixGetsLeadingSlash covers NewClioSink's branch that adds
// a leading slash to a prefix that lacks one.
func TestClioSinkSubjectPrefixGetsLeadingSlash(t *testing.T) {
	clio := &captureClio{}
	h := auditServer(t, clio, func(c *ClioConfig) { c.SubjectPrefix = "audit" })

	if rec := evalDish(t, h); rec.Code != http.StatusOK {
		t.Fatalf("evaluate = %d, want 200", rec.Code)
	}
	calls := clio.calls()
	if len(calls) != 1 {
		t.Fatalf("clio writes = %d, want 1", len(calls))
	}
	if got := calls[0].Events[0].Subject; got != "/audit/Dish" {
		t.Errorf("subject = %q, want /audit/Dish (leading slash added)", got)
	}
}

// TestClioSinkWriteTransportError covers write()'s client.Do error branch:
// pointing the sink at an unreachable URL makes the POST fail, which fail-closed
// surfaces as a 502.
func TestClioSinkWriteTransportError(t *testing.T) {
	// A reserved TEST-NET-1 address with a closed port: Do returns an error
	// without blocking on DNS. Use a short-timeout client to stay fast and
	// deterministic.
	sink, err := NewClioSink(ClioConfig{
		URL:        "http://127.0.0.1:1/never",
		Strict:     true,
		HTTPClient: &http.Client{},
	})
	if err != nil {
		t.Fatalf("NewClioSink: %v", err)
	}
	h := NewServer(nil, WithClioSink(sink)).Handler()

	rec := evalDish(t, h)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 (transport error, fail-closed) body=%s", rec.Code, rec.Body)
	}
}
