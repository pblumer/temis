package dmn_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// badXML is not a well-formed DMN document, so every Decode-based editor must
// return an error from it — covering each editor's decode-failure branch.
const badXML = `<<<not xml`

// TestEditorsRejectMalformedXML covers the dmnxml.Decode error branch of every
// XML-patching editor in one sweep.
func TestEditorsRejectMalformedXML(t *testing.T) {
	src := []byte(badXML)
	checks := []struct {
		name string
		fn   func() ([]byte, error)
	}{
		{"ApplyEdits", func() ([]byte, error) { return dmn.ApplyEdits(src, []dmn.NodeEdit{{ID: "x", Name: strptr("y")}}) }},
		{"ApplyGraph", func() ([]byte, error) {
			return dmn.ApplyGraph(src, dmn.GraphEdit{Nodes: []dmn.GraphNodeEdit{{ID: "x", Type: "decision"}}})
		}},
		{"ApplyTableEdit", func() ([]byte, error) { return dmn.ApplyTableEdit(src, "x", dmn.TableEdit{}) }},
		{"CreateDecisionTable", func() ([]byte, error) { return dmn.CreateDecisionTable(src, "x") }},
		{"SetBoxedContext", func() ([]byte, error) {
			return dmn.SetBoxedContext(src, "x", dmn.ContextEdit{Entries: []dmn.ContextEntryView{{Name: "a", Text: "1"}}})
		}},
		{"CreateBoxedContext", func() ([]byte, error) { return dmn.CreateBoxedContext(src, "x") }},
		{"SetBKMFunction", func() ([]byte, error) { return dmn.SetBKMFunction(src, "x", dmn.BKMFunctionEdit{BodyText: "1"}) }},
		{"SetLiteralExpression", func() ([]byte, error) { return dmn.SetLiteralExpression(src, "x", "1", "") }},
		{"SetItemDefinition", func() ([]byte, error) { return dmn.SetItemDefinition(src, dmn.ItemType{Name: "T", TypeRef: "string"}) }},
		{"RemoveItemDefinition", func() ([]byte, error) { return dmn.RemoveItemDefinition(src, "T") }},
	}
	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			if _, err := c.fn(); err == nil {
				t.Errorf("%s should reject malformed XML", c.name)
			}
		})
	}
}

// TestModelNameAndDecisionAccessors covers ModelName and CompiledDecision.ID.
func TestModelNameAndDecisionAccessors(t *testing.T) {
	defs := compileModel(t, "dish_15.dmn")
	if defs.ModelName() == "" {
		t.Error("ModelName should be non-empty for the dish fixture")
	}
	dec, err := defs.Decision("Dish")
	if err != nil {
		t.Fatal(err)
	}
	if dec.ID() == "" {
		t.Error("decision ID should be non-empty")
	}
	if dec.Name() != "Dish" {
		t.Errorf("Name = %q, want Dish", dec.Name())
	}
}

// TestCreateDecisionTableUnknownAndAlreadyLogic covers the failure branch of
// CreateDecisionTable: a decision that already carries logic cannot get a table.
func TestCreateDecisionTableUnknownAndAlreadyLogic(t *testing.T) {
	src := readModel(t, "dish_15.dmn")
	if _, err := dmn.CreateDecisionTable(src, "id_dish"); err == nil {
		t.Error("CreateDecisionTable on a decision that already has a table should error")
	}
	if _, err := dmn.CreateDecisionTable(src, "no-such-id"); err == nil {
		t.Error("CreateDecisionTable on an unknown id should error")
	}
}

// TestCreateBoxedContextRejects covers CreateBoxedContext's failure path on a
// decision that already has logic.
func TestCreateBoxedContextRejects(t *testing.T) {
	src := readModel(t, "dish_15.dmn")
	if _, err := dmn.CreateBoxedContext(src, "id_dish"); err == nil {
		t.Error("CreateBoxedContext on a decision that already has a table should error")
	}
}

// TestBKMFunctionNotFound covers the not-found branch of BKMFunction (ok=false).
func TestBKMFunctionNotFound(t *testing.T) {
	defs := compileModel(t, "bkm_invocation_15.dmn")
	if _, ok := defs.BKMFunction("Nope"); ok {
		t.Error("BKMFunction(unknown) should report ok=false")
	}
}

// TestInputSchemaByIDAndName covers Definitions.InputSchema lookup by id, by name,
// and the not-found error.
func TestInputSchemaByIDAndName(t *testing.T) {
	defs := compileModel(t, "dish_15.dmn")
	if _, err := defs.InputSchema("Dish"); err != nil {
		t.Errorf("InputSchema by name: %v", err)
	}
	if _, err := defs.InputSchema("id_dish"); err != nil {
		t.Errorf("InputSchema by id: %v", err)
	}
	if _, err := defs.InputSchema("nope"); err == nil {
		t.Error("InputSchema(unknown) should error")
	}
}

// TestServiceEvaluateContextCancelled covers CompiledService.Evaluate's ctx-error
// branch.
func TestServiceEvaluateContextCancelled(t *testing.T) {
	defs := compileModel(t, "decisionservice_15.dmn")
	svc, _ := defs.Service("Approval")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := svc.Evaluate(ctx, dmn.Input{"Applicant Age": 20}); err == nil {
		t.Error("cancelled context should fail service evaluate")
	}
}

// TestServiceEvaluateBadInput covers CompiledService.Evaluate's input-conversion
// error branch.
func TestServiceEvaluateBadInput(t *testing.T) {
	defs := compileModel(t, "decisionservice_15.dmn")
	svc, _ := defs.Service("Approval")
	if _, err := svc.Evaluate(context.Background(), dmn.Input{"Applicant Age": struct{}{}}); err == nil {
		t.Error("unsupported input value should fail service evaluate")
	}
}

// uniqueMultiModel is a UNIQUE-policy table whose two rules both match the same
// input, so evaluating it is a runtime error — exercising EvaluateGraph's
// per-decision error recording.
const uniqueMultiModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/um" name="UM" id="def_um">
  <inputData id="i_n" name="N"><variable name="N" typeRef="number"/></inputData>
  <decision id="d_u" name="Pick">
    <informationRequirement><requiredInput href="#i_n"/></informationRequirement>
    <decisionTable hitPolicy="UNIQUE">
      <input id="in1" label="N"><inputExpression typeRef="number"><text>N</text></inputExpression></input>
      <output id="out1" name="r" typeRef="string"/>
      <rule><inputEntry><text>&gt; 0</text></inputEntry><outputEntry><text>"a"</text></outputEntry></rule>
      <rule><inputEntry><text>&gt; 1</text></inputEntry><outputEntry><text>"b"</text></outputEntry></rule>
    </decisionTable>
  </decision>
</definitions>`

// TestEvaluateGraphRecordsPerDecisionError covers the EvaluateGraph error branch
// (a decision that fails at runtime lands in Errors, not Values).
func TestEvaluateGraphRecordsPerDecisionError(t *testing.T) {
	defs, diags, err := dmn.New().Compile(context.Background(), []byte(uniqueMultiModel))
	if err != nil {
		t.Fatal(err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected compile errors: %+v", diags)
	}
	// N=5 matches both ">0" and ">1": a UNIQUE multiple-match runtime error.
	res, err := defs.EvaluateGraph(context.Background(), dmn.Input{"N": 5})
	if err != nil {
		t.Fatalf("EvaluateGraph itself should not error: %v", err)
	}
	if _, ok := res.Errors["Pick"]; !ok {
		t.Errorf("want Pick in Errors, got Errors=%+v Values=%+v", res.Errors, res.Values)
	}
	if _, ok := res.Values["Pick"]; ok {
		t.Errorf("a failed decision must not appear in Values: %+v", res.Values)
	}
}

// sharedInputModel has two decisions consuming the same input "Color": the first
// declares it untyped with table-cell suggestions, the second declares its type
// and a closed allowed-values enumeration. ModelInputSchema must merge them:
// type taken from the typed decision, a closed enumeration winning over the
// suggestions, and Required true because one decision requires it. This drives the
// merge branches of ModelInputSchema and mergedConstraints.
const sharedInputModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/shared" name="Shared" id="def_shared">
  <itemDefinition id="t_color" name="ColorType">
    <typeRef>string</typeRef>
    <allowedValues><text>"red","green","blue"</text></allowedValues>
  </itemDefinition>
  <inputData id="i_color" name="Color"><variable name="Color"/></inputData>
  <decision id="d_first" name="First">
    <informationRequirement><requiredInput href="#i_color"/></informationRequirement>
    <decisionTable hitPolicy="UNIQUE">
      <input id="fi1" label="Color"><inputExpression><text>Color</text></inputExpression></input>
      <output id="fo1" name="r1" typeRef="string"/>
      <rule><inputEntry><text>"red"</text></inputEntry><outputEntry><text>"warm"</text></outputEntry></rule>
      <rule><inputEntry><text>"blue"</text></inputEntry><outputEntry><text>"cool"</text></outputEntry></rule>
    </decisionTable>
  </decision>
  <decision id="d_second" name="Second">
    <informationRequirement><requiredInput href="#i_color"/></informationRequirement>
    <decisionTable hitPolicy="UNIQUE">
      <input id="si1" label="Color"><inputExpression typeRef="ColorType"><text>Color</text></inputExpression></input>
      <output id="so1" name="r2" typeRef="string"/>
      <rule><inputEntry><text>-</text></inputEntry><outputEntry><text>"ok"</text></outputEntry></rule>
    </decisionTable>
  </decision>
</definitions>`

// TestModelInputSchemaMergesAcrossDecisions covers the dedup/merge branches of
// ModelInputSchema: the shared "Color" input picks up the typed decision's type
// and the closed enumeration, and stays a single deduped field.
func TestModelInputSchemaMergesAcrossDecisions(t *testing.T) {
	defs, diags, err := dmn.New().Compile(context.Background(), []byte(sharedInputModel))
	if err != nil {
		t.Fatal(err)
	}
	if diags.HasErrors() {
		t.Fatalf("compile errors: %+v", diags)
	}
	fields := defs.ModelInputSchema()
	var color *dmn.InputField
	for i := range fields {
		if fields[i].Name == "Color" {
			color = &fields[i]
		}
	}
	if color == nil {
		t.Fatalf("Color missing from model schema: %+v", fields)
	}
	if len(fields) != 1 {
		t.Errorf("Color should be deduped to one field, got %d (%+v)", len(fields), fields)
	}
	if color.Type != "ColorType" {
		t.Errorf("merged type = %q, want ColorType (from the typed decision)", color.Type)
	}
	// The closed enumeration from ColorType wins over the table-cell suggestions.
	if !color.ValuesClosed {
		t.Errorf("merged Values should be the closed enumeration, got %+v", color)
	}

	// ValidateModelInput uses the merged constraint: an out-of-set value is rejected.
	probs := defs.ValidateModelInput(dmn.Input{"Color": "purple"})
	found := false
	for _, p := range probs {
		if p.Code == "VALUE_NOT_ALLOWED" && p.Input == "Color" {
			found = true
		}
	}
	if !found {
		t.Errorf("want VALUE_NOT_ALLOWED for purple, got %+v", probs)
	}
}

// itemRefModel exercises resolveTypeRefSeen: an item definition whose typeRef
// names ANOTHER (collection) item definition, which in turn references a built-in.
// This forces on-demand resolution of a not-yet-built item type, the collection
// wrapping and the deeper recursion the simple buildItemTypes path skips.
const itemRefModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/ir" name="IR" id="def_ir">
  <itemDefinition id="t_alias" name="Alias">
    <typeRef>Scores</typeRef>
  </itemDefinition>
  <itemDefinition id="t_scores" name="Scores" isCollection="true">
    <typeRef>number</typeRef>
  </itemDefinition>
  <itemDefinition id="t_wrap" name="Wrap">
    <itemComponent id="c_inner" name="inner"><typeRef>Alias</typeRef></itemComponent>
  </itemDefinition>
  <inputData id="i_w" name="W"><variable name="W" typeRef="Wrap"/></inputData>
  <decision id="d_use" name="UseW">
    <informationRequirement><requiredInput href="#i_w"/></informationRequirement>
    <literalExpression><text>sum(W.inner)</text></literalExpression>
  </decision>
</definitions>`

// TestItemDefinitionForwardAndCollectionRefs compiles a model whose item
// definitions reference one another (forward + collection), exercising
// resolveTypeRefSeen. It must compile cleanly (the references resolve) and
// evaluate.
func TestItemDefinitionForwardAndCollectionRefs(t *testing.T) {
	defs, diags, err := dmn.New().Compile(context.Background(), []byte(itemRefModel))
	if err != nil {
		t.Fatal(err)
	}
	if diags.HasErrors() {
		t.Fatalf("forward/collection item refs should resolve cleanly: %+v", diags)
	}
	dec, err := defs.Decision("UseW")
	if err != nil {
		t.Fatal(err)
	}
	res, err := dec.Evaluate(context.Background(), dmn.Input{"W": map[string]any{"inner": []any{1, 2, 3}}})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got := res.Outputs["UseW"]; got != "6" {
		t.Errorf("sum(W.inner) = %v, want 6", got)
	}
}

// cyclicItemModel has two item definitions that reference each other, forcing the
// cycle guard in resolveTypeRefSeen. It must still compile (the cycle yields Any,
// not a crash).
const cyclicItemModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/cyc" name="Cyc" id="def_cyc">
  <itemDefinition id="t_a" name="A"><typeRef>B</typeRef></itemDefinition>
  <itemDefinition id="t_b" name="B"><typeRef>A</typeRef></itemDefinition>
  <inputData id="i_x" name="X"><variable name="X" typeRef="A"/></inputData>
  <decision id="d_c" name="UseX">
    <informationRequirement><requiredInput href="#i_x"/></informationRequirement>
    <literalExpression><text>X</text></literalExpression>
  </decision>
</definitions>`

// TestCyclicItemDefinitionsCompile covers the cycle guard in resolveTypeRefSeen.
func TestCyclicItemDefinitionsCompile(t *testing.T) {
	_, diags, err := dmn.New().Compile(context.Background(), []byte(cyclicItemModel))
	if err != nil {
		t.Fatalf("cyclic item definitions should not crash compile: %v", err)
	}
	// No assertion on diags beyond "did not blow up"; the cycle resolves to Any.
	_ = diags
}

// TestApplyGraphUnknownNodeType covers ApplyGraph's unknown-node-type error.
func TestApplyGraphUnknownNodeType(t *testing.T) {
	src := readModel(t, "dish_15.dmn")
	edit := graphEdit(t, src)
	edit.Nodes = append(edit.Nodes, dmn.GraphNodeEdit{ID: "weird", Type: "mystery", Name: "Weird", X: 1, Y: 1, Width: 10, Height: 10})
	if _, err := dmn.ApplyGraph(src, edit); err == nil || !strings.Contains(err.Error(), "unknown node type") {
		t.Errorf("ApplyGraph should reject an unknown node type, got %v", err)
	}
}

// TestCompiledDecisionEvaluateContextCancelled covers Evaluate's pre-evaluation
// context-error branch.
func TestCompiledDecisionEvaluateContextCancelled(t *testing.T) {
	dec := compileDecision(t, "dish_15.dmn", "Dish")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := dec.Evaluate(ctx, dmn.Input{"Season": "Winter", "Guest Count": 8})
	var ee *dmn.EvalError
	if !errors.As(err, &ee) {
		t.Fatalf("want *EvalError on cancelled context, got %v", err)
	}
}

// TestCompiledDecisionEvaluateBadInputConversion covers Evaluate's
// input-conversion error branch (an unsupported Go value on a non-required name).
func TestCompiledDecisionEvaluateBadInputConversion(t *testing.T) {
	dec := compileDecision(t, "dish_15.dmn", "Dish")
	// Supply both required inputs (so we pass the missing-input gate) plus an extra
	// name with an unsupported value to trip inputToValues.
	_, err := dec.Evaluate(context.Background(), dmn.Input{
		"Season": "Winter", "Guest Count": 8, "Extra": make(chan int),
	})
	if err == nil {
		t.Error("unsupported input value should fail evaluate")
	}
}
