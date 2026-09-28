package dmn_test

import (
	"context"
	"strings"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// ADR-0042: a decision service publishes what a caller supplies to evaluate it,
// and that one set drives conversion, strict validation and the schema.
//
// boundaryModel puts one value of each kind a service takes where it can be
// told apart from the others:
//
//   - `betrag` is input data read behind the interface, by the encapsulated
//     `Tragbar`, and declared a number.
//   - `stichtag` is an input decision, declared a date. The caller supplies it;
//     the encapsulated `Frist` compares it against a date literal, so it only
//     works if the text a JSON caller sends becomes a date.
//   - `eingang` is input data that only the input decision's own logic reads. The
//     service never computes `Stichtag`, so it never reads `eingang` either.
const boundaryModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" id="bm" name="Boundary" namespace="http://temis/test/boundary">
  <inputData id="in_eingang" name="eingang"><variable name="eingang" typeRef="date"/></inputData>
  <inputData id="in_betrag" name="betrag"><variable name="betrag" typeRef="number"/></inputData>
  <decision id="dec_stichtag" name="Stichtag">
    <variable name="stichtag" typeRef="date"/>
    <informationRequirement><requiredInput href="#in_eingang"/></informationRequirement>
    <literalExpression><text>eingang + duration("P30D")</text></literalExpression>
  </decision>
  <decision id="dec_frist" name="Frist">
    <variable name="frist" typeRef="boolean"/>
    <informationRequirement><requiredDecision href="#dec_stichtag"/></informationRequirement>
    <literalExpression><text>stichtag &lt; date("2026-06-01")</text></literalExpression>
  </decision>
  <decision id="dec_tragbar" name="Tragbar">
    <variable name="tragbar" typeRef="boolean"/>
    <informationRequirement><requiredInput href="#in_betrag"/></informationRequirement>
    <literalExpression><text>betrag &lt;= 50000</text></literalExpression>
  </decision>
  <decision id="dec_urteil" name="Urteil">
    <variable name="urteil" typeRef="string"/>
    <informationRequirement><requiredDecision href="#dec_frist"/></informationRequirement>
    <informationRequirement><requiredDecision href="#dec_tragbar"/></informationRequirement>
    <literalExpression><text>if frist and tragbar then "bewilligt" else "abgelehnt"</text></literalExpression>
  </decision>
  <decisionService id="svc_freigabe" name="Freigabe">
    <outputDecision href="#dec_urteil"/>
    <encapsulatedDecision href="#dec_frist"/>
    <encapsulatedDecision href="#dec_tragbar"/>
    <inputDecision href="#dec_stichtag"/>
    <inputData href="#in_betrag"/>
  </decisionService>
</definitions>`

func boundaryService(t *testing.T, src string) *dmn.CompiledService {
	t.Helper()
	defs, diags, err := dmn.New().Compile(context.Background(), []byte(src))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("compile diagnostics: %v", diags)
	}
	svc, err := defs.Service("Freigabe")
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	return svc
}

func fieldsByName(fields []dmn.InputField) map[string]dmn.InputField {
	out := make(map[string]dmn.InputField, len(fields))
	for _, f := range fields {
		out[f.Name] = f
	}
	return out
}

func codesOf(probs []dmn.InputProblem) map[string]string {
	out := make(map[string]string, len(probs))
	for _, p := range probs {
		out[p.Input] = p.Code
	}
	return out
}

// The schema names the input data behind the interface and the input decision at
// its boundary, each with its declared type, and nothing only the input decision
// reads.
func TestADR0042AServicePublishesWhatItsCallerSupplies(t *testing.T) {
	svc := boundaryService(t, boundaryModel)
	got := svc.InputSchema()
	if len(got) != 2 {
		t.Fatalf("InputSchema = %+v, want exactly betrag and stichtag", got)
	}
	byName := fieldsByName(got)
	if f := byName["betrag"]; f.Type != "number" || !f.Required {
		t.Errorf("betrag = %+v, want a required number", f)
	}
	if f := byName["stichtag"]; f.Type != "date" || !f.Required {
		t.Errorf("stichtag = %+v, want a required date — an input decision is typed by its variable", f)
	}
	if _, ok := byName["eingang"]; ok {
		t.Errorf("eingang is in the schema; only the input decision reads it, and the service never computes that")
	}
}

// An input decision declared `date` is converted like input data. Before this
// record the service's working set was built from input data only, so the
// supplied text reached `Frist` as a string, the comparison was null, and the
// service answered "abgelehnt" with no error anywhere.
func TestADR0042AnInputDecisionIsConvertedByItsDeclaredType(t *testing.T) {
	svc := boundaryService(t, boundaryModel)
	for _, c := range []struct{ stichtag, want string }{
		{"2026-03-01", "bewilligt"},
		{"2026-09-01", "abgelehnt"},
	} {
		res, err := svc.Evaluate(context.Background(), dmn.Input{"betrag": 30000, "stichtag": c.stichtag})
		if err != nil {
			t.Fatalf("Evaluate(%s): %v", c.stichtag, err)
		}
		if got := res.Outputs["urteil"]; got != c.want {
			t.Errorf("Freigabe(stichtag=%s) = %v, want %q — the input decision reached Frist as text", c.stichtag, got, c.want)
		}
	}
}

func TestADR0042ValidateInputReportsBothKindsOfInput(t *testing.T) {
	svc := boundaryService(t, boundaryModel)

	if probs := svc.ValidateInput(dmn.Input{"betrag": 30000, "stichtag": "2026-03-01"}); len(probs) != 0 {
		t.Errorf("a correct input: problems = %+v, want none", probs)
	}

	codes := codesOf(svc.ValidateInput(dmn.Input{"betrag": "30000", "stichtag": 42}))
	if codes["betrag"] != "TYPE_MISMATCH" || codes["stichtag"] != "TYPE_MISMATCH" || len(codes) != 2 {
		t.Errorf("wrong types on both: codes = %v, want TYPE_MISMATCH on betrag and on stichtag", codes)
	}

	codes = codesOf(svc.ValidateInput(dmn.Input{"betrag": 30000}))
	if codes["stichtag"] != "MISSING_INPUT" || len(codes) != 1 {
		t.Errorf("no input decision: codes = %v, want MISSING_INPUT on stichtag only", codes)
	}
}

// A value beneath the boundary is not the service's to check: the service never
// reads it, so a wrong type there changes nothing, and refusing it would refuse a
// call that evaluates correctly. It is reported as what it is — unknown to this
// service — and nothing else.
func TestADR0042AValueBeneathTheBoundaryIsNotTypeChecked(t *testing.T) {
	svc := boundaryService(t, boundaryModel)
	probs := svc.ValidateInput(dmn.Input{"betrag": 30000, "stichtag": "2026-03-01", "eingang": 7})
	if len(probs) != 1 || probs[0].Input != "eingang" || probs[0].Code != "UNKNOWN_INPUT" {
		t.Fatalf("problems = %+v, want one UNKNOWN_INPUT on eingang", probs)
	}
	if !strings.Contains(probs[0].Message, `decision service "Freigabe"`) {
		t.Errorf("message = %q, want it to name the service", probs[0].Message)
	}
}

// WithStrictInput was accepted and ignored on a service. It now refuses, and the
// lenient default is unchanged.
func TestADR0042StrictInputIsHonouredByAService(t *testing.T) {
	svc := boundaryService(t, boundaryModel)
	bad := dmn.Input{"betrag": "30000", "stichtag": "2026-03-01"}

	_, err := svc.Evaluate(context.Background(), bad, dmn.WithStrictInput())
	var ie *dmn.InputError
	if !asInputError(err, &ie) {
		t.Fatalf("strict evaluation of a wrongly-typed input: err = %v, want an *InputError", err)
	}
	if len(ie.Problems) != 1 || ie.Problems[0].Input != "betrag" || ie.Problems[0].Code != "TYPE_MISMATCH" {
		t.Errorf("problems = %+v, want one TYPE_MISMATCH on betrag", ie.Problems)
	}

	if _, err := svc.Evaluate(context.Background(), bad); err != nil {
		t.Errorf("lenient evaluation: %v, want the default to stay lenient", err)
	}

	res, err := svc.Evaluate(context.Background(), dmn.Input{"betrag": 30000, "stichtag": "2026-03-01"}, dmn.WithStrictInput())
	if err != nil || res.Outputs["urteil"] != "bewilligt" {
		t.Errorf("strict evaluation of a correct input: %v, %v, want bewilligt", res.Outputs, err)
	}
}

// The schema follows what evaluation reads, not what the <decisionService>
// element lists. A declaration that forgets `betrag` — the state a model is left
// in when a decision is moved inside the service and nobody adds its input — still
// yields a schema with `betrag`, because `Tragbar` still reads it.
func TestADR0042AnIncompleteDeclarationDoesNotNarrowTheSchema(t *testing.T) {
	src := strings.Replace(boundaryModel, `<inputData href="#in_betrag"/>`, "", 1)
	if src == boundaryModel {
		t.Fatal("the fixture no longer declares betrag on the service — the premise is gone")
	}
	svc := boundaryService(t, src)
	f, ok := fieldsByName(svc.InputSchema())["betrag"]
	if !ok || f.Type != "number" {
		t.Fatalf("InputSchema = %+v, want betrag:number although the service does not list it", svc.InputSchema())
	}
	codes := codesOf(svc.ValidateInput(dmn.Input{"betrag": "30000", "stichtag": "2026-03-01"}))
	if codes["betrag"] != "TYPE_MISMATCH" {
		t.Errorf("codes = %v, want TYPE_MISMATCH on betrag", codes)
	}
}

// An input decision's allowed values constrain it like an input data's: a named
// item definition with an enumeration makes a value outside it VALUE_NOT_ALLOWED
// and offers the closed set to a picker.
func TestADR0042AnInputDecisionCarriesItsTypesAllowedValues(t *testing.T) {
	src := strings.Replace(boundaryModel,
		`<inputData id="in_eingang"`,
		`<itemDefinition name="tStufe"><typeRef>string</typeRef><allowedValues><text>"A","B"</text></allowedValues></itemDefinition>
  <inputData id="in_eingang"`, 1)
	src = strings.Replace(src,
		`<decisionService id="svc_freigabe" name="Freigabe">`,
		`<decision id="dec_stufe" name="Stufe">
    <variable name="stufe" typeRef="tStufe"/>
    <literalExpression><text>"A"</text></literalExpression>
  </decision>
  <decisionService id="svc_freigabe" name="Freigabe">
    <inputDecision href="#dec_stufe"/>`, 1)
	src = strings.Replace(src,
		`<informationRequirement><requiredDecision href="#dec_tragbar"/></informationRequirement>`,
		`<informationRequirement><requiredDecision href="#dec_tragbar"/></informationRequirement>
    <informationRequirement><requiredDecision href="#dec_stufe"/></informationRequirement>`, 1)

	svc := boundaryService(t, src)
	f, ok := fieldsByName(svc.InputSchema())["stufe"]
	if !ok || f.Type != "tStufe" || !f.ValuesClosed || len(f.Values) != 2 {
		t.Fatalf("stufe = %+v (ok=%v), want tStufe with the closed values A, B", f, ok)
	}
	codes := codesOf(svc.ValidateInput(dmn.Input{"betrag": 30000, "stichtag": "2026-03-01", "stufe": "C"}))
	if codes["stufe"] != "VALUE_NOT_ALLOWED" || len(codes) != 1 {
		t.Errorf("codes = %v, want VALUE_NOT_ALLOWED on stufe only", codes)
	}
}
