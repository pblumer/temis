package dmn_test

import (
	"context"
	"strings"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// ADR-0041: what a caller supplies to evaluate a decision is the leaf inputs of
// its whole requirements cone, and everything that reads an input against a
// schema must read that set.
//
// composedDateModel is the smallest model that poses the question. `Urteil`
// requires only the decision `Frist` and declares no input of its own, so its own
// input schema is empty; `stichtag` — the only value a caller sends — is declared
// one level down, as a date.
const composedDateModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" id="cdm" name="Composed" namespace="http://temis/test/composed">
  <inputData id="in_stichtag" name="stichtag"><variable name="stichtag" typeRef="date"/></inputData>
  <decision id="dec_frist" name="Frist">
    <variable name="frist" typeRef="boolean"/>
    <informationRequirement><requiredInput href="#in_stichtag"/></informationRequirement>
    <literalExpression><text>stichtag &lt; date("2026-06-01")</text></literalExpression>
  </decision>
  <decision id="dec_urteil" name="Urteil">
    <variable name="urteil" typeRef="string"/>
    <informationRequirement><requiredDecision href="#dec_frist"/></informationRequirement>
    <literalExpression><text>if frist then "vorher" else "nachher"</text></literalExpression>
  </decision>
  <decisionService id="svc" name="Pruefung">
    <outputDecision href="#dec_urteil"/>
    <encapsulatedDecision href="#dec_frist"/>
    <inputData href="#in_stichtag"/>
  </decisionService>
</definitions>`

func composedDefs(t *testing.T) *dmn.Definitions {
	t.Helper()
	defs, diags, err := dmn.New().Compile(context.Background(), []byte(composedDateModel))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("compile diagnostics: %v", diags)
	}
	return defs
}

// The premise, asserted so the tests below cannot pass for the wrong reason: the
// composed decision declares nothing itself, and its reachable schema is where
// the date lives.
func TestADR0041AComposedDecisionDeclaresNothingButReachesTheInput(t *testing.T) {
	defs := composedDefs(t)
	dec, err := defs.Decision("Urteil")
	if err != nil {
		t.Fatalf("decision: %v", err)
	}
	if len(dec.InputSchema()) != 0 {
		t.Fatalf("Urteil declares %+v directly, want nothing — the premise is gone", dec.InputSchema())
	}
	reach, err := defs.ReachableInputSchema("Urteil")
	if err != nil {
		t.Fatalf("reachable schema: %v", err)
	}
	if len(reach) != 1 || reach[0].Name != "stichtag" || reach[0].Type != "date" {
		t.Fatalf("reachable schema = %+v, want stichtag:date", reach)
	}
}

// The conversion follows the cone: a date declared one level down is a date when
// the caller evaluates the decision above it (ADR-0040, corrected here).
func TestADR0041ADeclaredDateIsConvertedForAComposedDecision(t *testing.T) {
	defs := composedDefs(t)
	for _, c := range []struct{ stichtag, want string }{
		{"2026-03-01", "vorher"},
		{"2026-09-01", "nachher"},
	} {
		dec, err := defs.Decision("Urteil")
		if err != nil {
			t.Fatalf("decision: %v", err)
		}
		res, err := dec.Evaluate(context.Background(), dmn.Input{"stichtag": c.stichtag})
		if err != nil {
			t.Fatalf("Evaluate(%s): %v", c.stichtag, err)
		}
		if got := res.Outputs["urteil"]; got != c.want {
			t.Errorf("Urteil(%s) = %v, want %q — the date reached Frist as text", c.stichtag, got, c.want)
		}
	}
}

// Strict validation follows the same cone: a transitive input is legitimate
// rather than UNKNOWN_INPUT, and a wrongly-typed one is reported rather than
// waved through because this decision happens to declare nothing.
func TestADR0041StrictValidationOfAComposedDecisionUsesTheCone(t *testing.T) {
	defs := composedDefs(t)
	dec, err := defs.Decision("Urteil")
	if err != nil {
		t.Fatalf("decision: %v", err)
	}

	if _, err := dec.Evaluate(context.Background(), dmn.Input{"stichtag": "2026-03-01"}, dmn.WithStrictInput()); err != nil {
		t.Errorf("strict evaluation of a correct transitive input: %v, want it accepted", err)
	}

	_, err = dec.Evaluate(context.Background(), dmn.Input{"stichtag": 42}, dmn.WithStrictInput())
	var ie *dmn.InputError
	if !asInputError(err, &ie) {
		t.Fatalf("strict evaluation of a wrongly-typed transitive input: err = %v, want an *InputError", err)
	}
	if len(ie.Problems) != 1 || ie.Problems[0].Code != "TYPE_MISMATCH" || ie.Problems[0].Input != "stichtag" {
		t.Errorf("problems = %+v, want one TYPE_MISMATCH on stichtag", ie.Problems)
	}
}

// And the service boundary, whose working set is built from its output decisions:
// an output decision that reaches its inputs through others used to contribute
// nothing, so the conversion never ran for exactly the services that encapsulate.
func TestADR0041AServiceOverAComposedDecisionConvertsItsInputs(t *testing.T) {
	defs := composedDefs(t)
	svc, err := defs.Service("Pruefung")
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	res, err := svc.Evaluate(context.Background(), dmn.Input{"stichtag": "2026-03-01"})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if got := res.Outputs["urteil"]; got != "vorher" {
		t.Errorf("Pruefung = %v, want \"vorher\" — the date reached the service as text", got)
	}
}

func asInputError(err error, target **dmn.InputError) bool {
	if err == nil {
		return false
	}
	ie, ok := err.(*dmn.InputError)
	if ok {
		*target = ie
	}
	return ok
}

// A leaf decision is unchanged: its cone is itself, so nothing about it moves.
func TestADR0041ALeafDecisionIsUnchanged(t *testing.T) {
	defs := composedDefs(t)
	dec, err := defs.Decision("Frist")
	if err != nil {
		t.Fatalf("decision: %v", err)
	}
	direct, reach := dec.InputSchema(), func() []dmn.InputField {
		r, _ := defs.ReachableInputSchema("Frist")
		return r
	}()
	if len(direct) != 1 || len(reach) != 1 || direct[0].Name != reach[0].Name {
		t.Errorf("leaf: direct = %+v, reachable = %+v, want the same single input", direct, reach)
	}
	if _, err := dec.Evaluate(context.Background(), dmn.Input{"stichtag": "nonsense"}, dmn.WithStrictInput()); err == nil ||
		!strings.Contains(err.Error(), "stichtag") {
		t.Errorf("leaf strict on unparseable text: err = %v, want it named", err)
	}
}
