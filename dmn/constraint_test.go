package dmn_test

import (
	"context"
	"errors"
	"testing"

	"github.com/pblumer/temis/dmn"
)

const itemTypeModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/it" name="IT" id="def_it">
  <itemDefinition id="t_color" name="Color">
    <typeRef>string</typeRef>
    <allowedValues><text>"red","green","blue"</text></allowedValues>
  </itemDefinition>
  <itemDefinition id="t_person" name="Person">
    <itemComponent id="pa" name="age"><typeRef>number</typeRef></itemComponent>
    <itemComponent id="pn" name="pname"><typeRef>string</typeRef></itemComponent>
  </itemDefinition>
  <inputData id="i_shade" name="Shade"><variable name="Shade" typeRef="Color"/></inputData>
  <inputData id="i_p" name="P"><variable name="P" typeRef="Person"/></inputData>
  <decision id="d_pick" name="Pick">
    <variable name="Pick" typeRef="string"/>
    <informationRequirement><requiredInput href="#i_shade"/></informationRequirement>
    <informationRequirement><requiredInput href="#i_p"/></informationRequirement>
    <literalExpression><text>if P.age >= 18 then Shade else "n/a"</text></literalExpression>
  </decision>
</definitions>`

func compileInline(t *testing.T, xml string) *dmn.Definitions {
	t.Helper()
	defs, diags, err := dmn.New().Compile(context.Background(), []byte(xml))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected error diagnostics: %+v", diags)
	}
	return defs
}

// TestAllowedValuesConstraint covers WP-31: a value outside a typed input's
// allowed values is rejected under strict validation, an allowed one passes.
func TestAllowedValuesConstraint(t *testing.T) {
	defs := compileInline(t, itemTypeModel)
	dec, _ := defs.Decision("Pick")
	person := map[string]any{"age": 20, "pname": "Ann"}

	// Allowed value evaluates fine.
	res, err := dec.Evaluate(context.Background(), dmn.Input{"Shade": "red", "P": person}, dmn.WithStrictInput())
	if err != nil {
		t.Fatalf("strict eval with allowed value: %v", err)
	}
	if res.Outputs["Pick"] != "red" {
		t.Errorf("Pick = %v, want red", res.Outputs["Pick"])
	}

	// Disallowed value is rejected with VALUE_NOT_ALLOWED.
	_, err = dec.Evaluate(context.Background(), dmn.Input{"Shade": "purple", "P": person}, dmn.WithStrictInput())
	assertInputProblem(t, err, "VALUE_NOT_ALLOWED", "Shade")
}

// TestStructTypeConstraint covers WP-31: a struct-typed input must be a context.
func TestStructTypeConstraint(t *testing.T) {
	defs := compileInline(t, itemTypeModel)
	dec, _ := defs.Decision("Pick")

	_, err := dec.Evaluate(context.Background(), dmn.Input{"Shade": "red", "P": 5}, dmn.WithStrictInput())
	assertInputProblem(t, err, "TYPE_MISMATCH", "P")
}

// TestSchemaSurfacesCustomTypeAndConstraint covers WP-31's self-description.
func TestSchemaSurfacesCustomTypeAndConstraint(t *testing.T) {
	defs := compileInline(t, itemTypeModel)
	fields, err := defs.InputSchema("Pick")
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]dmn.InputField{}
	for _, f := range fields {
		by[f.Name] = f
	}
	if f := by["Shade"]; f.Type != "Color" || f.Constraint != `"red","green","blue"` {
		t.Errorf("Shade field = %+v, want type Color with allowed values", f)
	}
	if f := by["P"]; f.Type != "Person" {
		t.Errorf("P field type = %q, want Person", f.Type)
	}
}

const outputConstraintModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/oc" name="OC" id="def_oc">
  <inputData id="i_score" name="Score"><variable name="Score" typeRef="number"/></inputData>
  <decision id="d_grade" name="Grade">
    <variable name="Grade" typeRef="string"/>
    <informationRequirement><requiredInput href="#i_score"/></informationRequirement>
    <decisionTable hitPolicy="UNIQUE">
      <input id="in1"><inputExpression typeRef="number"><text>Score</text></inputExpression></input>
      <output name="Grade" typeRef="string"><outputValues><text>"A","B"</text></outputValues></output>
      <rule><inputEntry><text>&gt;= 90</text></inputEntry><outputEntry><text>"A"</text></outputEntry></rule>
      <rule><inputEntry><text>&lt; 90</text></inputEntry><outputEntry><text>"C"</text></outputEntry></rule>
    </decisionTable>
  </decision>
</definitions>`

// TestOutputConstraintWarning covers WP-31's static output check: a constant
// output cell outside the output clause's allowed values is a positioned warning.
func TestOutputConstraintWarning(t *testing.T) {
	_, diags, err := dmn.New().Compile(context.Background(), []byte(outputConstraintModel))
	if err != nil {
		t.Fatal(err)
	}
	w := typeWarnings(diags)
	if len(w) != 1 {
		t.Fatalf("got %d type warnings, want 1 (output \"C\"): %+v", len(w), w)
	}
	if w[0].DecisionID != "d_grade" {
		t.Errorf("warning on %q, want d_grade", w[0].DecisionID)
	}
}

const aliasChainModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/al" name="AL" id="def_al">
  <itemDefinition id="t_amount" name="Amount"><typeRef>number</typeRef></itemDefinition>
  <itemDefinition id="t_price" name="Price"><typeRef>Amount</typeRef></itemDefinition>
  <inputData id="i_p" name="P"><variable name="P" typeRef="Price"/></inputData>
  <decision id="d_x" name="X">
    <variable name="X"/>
    <informationRequirement><requiredInput href="#i_p"/></informationRequirement>
    <literalExpression><text>P + "oops"</text></literalExpression>
  </decision>
</definitions>`

// TestItemTypeAliasChain covers WP-31: a type that references another item
// definition resolves through the chain (Price → Amount → number), so an
// arithmetic-with-string use of it is flagged.
func TestItemTypeAliasChain(t *testing.T) {
	_, diags, err := dmn.New().Compile(context.Background(), []byte(aliasChainModel))
	if err != nil {
		t.Fatal(err)
	}
	if len(typeWarnings(diags)) != 1 {
		t.Errorf("want 1 type warning (Price resolves to number, + string), got %+v", typeWarnings(diags))
	}
}

func assertInputProblem(t *testing.T, err error, code, input string) {
	t.Helper()
	if err == nil {
		t.Fatalf("want an *InputError with %s on %q, got nil", code, input)
	}
	var ie *dmn.InputError
	if !errors.As(err, &ie) {
		t.Fatalf("want *InputError, got %T: %v", err, err)
	}
	for _, p := range ie.Problems {
		if p.Code == code && p.Input == input {
			return
		}
	}
	t.Errorf("want a %s problem on %q, got %+v", code, input, ie.Problems)
}
