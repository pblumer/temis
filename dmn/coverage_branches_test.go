package dmn_test

import (
	"context"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// compileWith compiles inline DMN and returns the definitions and diagnostics,
// failing only on a hard (non-diagnostic) error.
func compileSrc2(t *testing.T, xml string) (*dmn.Definitions, dmn.Diagnostics) {
	t.Helper()
	defs, diags, err := dmn.New().Compile(context.Background(), []byte(xml))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return defs, diags
}

// --- engine.go: compileBKMs skips + body compile error; compileDiagnostic ----

const bkmEdgeModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/bkmedge" name="BkmEdge" id="def_bkmedge">
  <businessKnowledgeModel id="bkm_empty" name="">
    <encapsulatedLogic><literalExpression><text>1</text></literalExpression></encapsulatedLogic>
  </businessKnowledgeModel>
  <businessKnowledgeModel id="bkm_nologic" name="NoLogic"/>
  <businessKnowledgeModel id="bkm_bad" name="BadBody">
    <encapsulatedLogic>
      <formalParameter name="x" typeRef="number"/>
      <literalExpression><text>x +</text></literalExpression>
    </encapsulatedLogic>
  </businessKnowledgeModel>
</definitions>`

// TestCompileBKMsEdgeCases covers compileBKMs's skip branches (an unnamed BKM and
// a BKM with no encapsulated logic) and its body-compile-error diagnostic.
func TestCompileBKMsEdgeCases(t *testing.T) {
	_, diags := compileSrc2(t, bkmEdgeModel)
	found := false
	for _, d := range diags {
		if d.Code == "FEEL_COMPILE_ERROR" {
			found = true
		}
	}
	if !found {
		t.Errorf("want a FEEL_COMPILE diagnostic for the bad BKM body, got %+v", diags)
	}
}

// --- engine.go: compileDecision diagnostic for a non-CompileError-ish failure
// is harder to force; the bad table below produces a compile diagnostic that
// drives compileDiagnostic. A decision-table cell that is not a valid unary test
// yields a decision-level compile error. ---

const badDecisionModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/baddec" name="BadDec" id="def_baddec">
  <inputData id="i_n" name="N"><variable name="N" typeRef="number"/></inputData>
  <decision id="d_bad" name="Bad">
    <informationRequirement><requiredInput href="#i_n"/></informationRequirement>
    <literalExpression><text>N +</text></literalExpression>
  </decision>
</definitions>`

// TestCompileDecisionDiagnostic covers compileDiagnostic: a decision whose logic
// fails to compile yields a positioned, decision-scoped diagnostic and a
// non-executable decision.
func TestCompileDecisionDiagnostic(t *testing.T) {
	defs, diags := compileSrc2(t, badDecisionModel)
	if !diags.HasErrors() {
		t.Fatalf("want a compile error for the bad literal, got %+v", diags)
	}
	if _, err := defs.Decision("Bad"); err == nil {
		t.Error("a decision that failed to compile must not be executable")
	}
}

// --- eval.go / service.go: a required decision with no executable logic ------

const reqNoLogicModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/reqnologic" name="ReqNoLogic" id="def_rnl">
  <inputData id="i_n" name="N"><variable name="N" typeRef="number"/></inputData>
  <decision id="d_sub" name="Sub">
    <informationRequirement><requiredInput href="#i_n"/></informationRequirement>
    <literalExpression><text>N +</text></literalExpression>
  </decision>
  <decision id="d_top" name="Top">
    <informationRequirement><requiredDecision href="#d_sub"/></informationRequirement>
    <literalExpression><text>Sub + 1</text></literalExpression>
  </decision>
</definitions>`

// TestEvalRequiredDecisionNoLogic covers eval's branch where a required decision
// has no executable logic: evaluating the requiring decision errors.
func TestEvalRequiredDecisionNoLogic(t *testing.T) {
	defs, diags := compileSrc2(t, reqNoLogicModel)
	if !diags.HasErrors() {
		t.Fatalf("want a compile error for the broken Sub, got %+v", diags)
	}
	// Top is itself executable (its own literal compiled), but its required Sub did
	// not — so evaluating Top hits the "required decision has no executable logic"
	// path in eval.
	top, err := defs.Decision("Top")
	if err != nil {
		t.Fatalf("Top should be executable on its own: %v", err)
	}
	if _, err := top.Evaluate(context.Background(), dmn.Input{"N": 1}); err == nil {
		t.Error("evaluating Top with a broken required decision should error")
	}
}

// --- service.go: an output decision that errors at runtime --------------------

const serviceRuntimeErrModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/svcerr" name="SvcErr" id="def_svcerr">
  <inputData id="i_n" name="N"><variable name="N" typeRef="number"/></inputData>
  <decision id="d_pick" name="Pick">
    <informationRequirement><requiredInput href="#i_n"/></informationRequirement>
    <decisionTable hitPolicy="UNIQUE">
      <input id="in1" label="N"><inputExpression typeRef="number"><text>N</text></inputExpression></input>
      <output id="out1" name="r" typeRef="string"/>
      <rule><inputEntry><text>&gt; 0</text></inputEntry><outputEntry><text>"a"</text></outputEntry></rule>
      <rule><inputEntry><text>&gt; 1</text></inputEntry><outputEntry><text>"b"</text></outputEntry></rule>
    </decisionTable>
  </decision>
  <decisionService id="svc" name="PickSvc">
    <outputDecision href="#d_pick"/>
  </decisionService>
</definitions>`

// TestServiceEvaluateRuntimeError covers CompiledService.Evaluate's error branch
// where an output decision fails at runtime (UNIQUE multiple match).
func TestServiceEvaluateRuntimeError(t *testing.T) {
	defs, diags := compileSrc2(t, serviceRuntimeErrModel)
	if diags.HasErrors() {
		t.Fatalf("unexpected compile errors: %+v", diags)
	}
	svc, err := defs.Service("PickSvc")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Evaluate(context.Background(), dmn.Input{"N": 5}); err == nil {
		t.Error("a UNIQUE multiple match in an output decision should error the service")
	}
}

// --- context.go: a boxed context with a nested (non-literal) entry -----------

const nestedContextModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/nestedctx" name="NestedCtx" id="def_nestedctx">
  <inputData id="i_p" name="Points"><variable name="Points" typeRef="number"/></inputData>
  <decision id="d_score" name="Score">
    <informationRequirement><requiredInput href="#i_p"/></informationRequirement>
    <context>
      <contextEntry>
        <variable name="Nested"/>
        <context>
          <contextEntry>
            <variable name="Inner"/>
            <literalExpression><text>Points * 2</text></literalExpression>
          </contextEntry>
        </context>
      </contextEntry>
      <contextEntry>
        <variable name="Plain"/>
        <literalExpression><text>Points + 1</text></literalExpression>
      </contextEntry>
    </context>
  </decision>
</definitions>`

// TestBoxedContextNotSimpleWithNestedEntry covers BoxedContext's non-literal-entry
// branch: a nested boxed entry makes the view not simply editable, still surfacing
// the named placeholder.
func TestBoxedContextNotSimpleWithNestedEntry(t *testing.T) {
	defs, diags := compileSrc2(t, nestedContextModel)
	if diags.HasErrors() {
		t.Fatalf("unexpected compile errors: %+v", diags)
	}
	v, ok := defs.BoxedContext("Score")
	if !ok {
		t.Fatal("Score has no boxed context")
	}
	if v.Simple {
		t.Error("a context with a nested boxed entry should not be simple")
	}
	// The nested entry is surfaced as a named placeholder (no text).
	foundNested := false
	for _, e := range v.Entries {
		if e.Name == "Nested" {
			foundNested = true
			if e.Text != "" {
				t.Errorf("nested entry should be a placeholder, got text %q", e.Text)
			}
		}
	}
	if !foundNested {
		t.Errorf("nested entry placeholder missing: %+v", v.Entries)
	}
}

// TestSetBoxedContextWrongLogic covers SetBoxedContext's "cannot set" failure on a
// decision with non-context logic (a literal expression).
func TestSetBoxedContextWrongLogic(t *testing.T) {
	// pricing's "Net Total" (id_net) is a literal expression, not a context.
	src := readModel(t, "pricing_15.dmn")
	_, err := dmn.SetBoxedContext(src, "id_net", dmn.ContextEdit{
		Entries: []dmn.ContextEntryView{{Name: "x", Text: "1"}},
	})
	if err == nil {
		t.Error("SetBoxedContext on a literal-expression decision should error")
	}
}

// --- types.go: SetItemDefinition rejects a structured (component) type --------

const structuredTypeModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/struct" name="Struct" id="def_struct">
  <itemDefinition id="t_person" name="Person">
    <itemComponent id="c_age" name="age"><typeRef>number</typeRef></itemComponent>
  </itemDefinition>
  <inputData id="i_x" name="X"><variable name="X" typeRef="number"/></inputData>
  <decision id="d_x" name="UseX">
    <informationRequirement><requiredInput href="#i_x"/></informationRequirement>
    <literalExpression><text>X + 1</text></literalExpression>
  </decision>
</definitions>`

// TestSetItemDefinitionRejectsStructured covers SetItemDefinition's structured-type
// guard: a type with item components cannot be overwritten by the simple editor.
func TestSetItemDefinitionRejectsStructured(t *testing.T) {
	defs, diags := compileSrc2(t, structuredTypeModel)
	if diags.HasErrors() {
		t.Fatalf("unexpected compile errors: %+v", diags)
	}
	_ = defs
	if _, err := dmn.SetItemDefinition([]byte(structuredTypeModel), dmn.ItemType{Name: "Person", TypeRef: "string"}); err == nil {
		t.Error("SetItemDefinition on a structured type should error")
	}
}

// --- typecheck.go: checkOutputConstraints with a non-constant output cell -----

const nonConstOutputModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/noncost" name="NonConst" id="def_noncost">
  <inputData id="i_n" name="N"><variable name="N" typeRef="number"/></inputData>
  <decision id="d_o" name="Out">
    <informationRequirement><requiredInput href="#i_n"/></informationRequirement>
    <decisionTable hitPolicy="UNIQUE">
      <input id="in1" label="N"><inputExpression typeRef="number"><text>N</text></inputExpression></input>
      <output id="out1" name="r" typeRef="string">
        <outputValues><text>"low","high"</text></outputValues>
      </output>
      <rule><inputEntry><text>&lt; 10</text></inputEntry><outputEntry><text>"low"</text></outputEntry></rule>
      <rule><inputEntry><text>&gt;= 10</text></inputEntry><outputEntry><text>if N > 100 then "high" else "low"</text></outputEntry></rule>
    </decisionTable>
  </decision>
</definitions>`

// TestCheckOutputConstraintsSkipsNonConstant covers checkOutputConstraints's
// non-constant branch: an expression output cell cannot be decided statically, so
// it is skipped (no false warning), while the constant cells are clean.
func TestCheckOutputConstraintsSkipsNonConstant(t *testing.T) {
	_, diags := compileSrc2(t, nonConstOutputModel)
	for _, d := range diags {
		if d.Code == "TYPE_ERROR" {
			t.Errorf("expression output cell should not be flagged: %+v", d)
		}
	}
}

// --- typecheck.go: resolveTypeRefSeen forward-ref to a structured type --------

const forwardStructModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/fwd" name="Fwd" id="def_fwd">
  <itemDefinition id="t_alias" name="Alias">
    <typeRef>Complex</typeRef>
  </itemDefinition>
  <itemDefinition id="t_complex" name="Complex" isCollection="true">
    <itemComponent id="c_v" name="v"><typeRef>number</typeRef></itemComponent>
  </itemDefinition>
  <inputData id="i_a" name="A"><variable name="A" typeRef="Alias"/></inputData>
  <decision id="d_a" name="UseA">
    <informationRequirement><requiredInput href="#i_a"/></informationRequirement>
    <literalExpression><text>count(A)</text></literalExpression>
  </decision>
</definitions>`

// TestResolveTypeRefSeenForwardStruct covers resolveTypeRefSeen resolving a
// forward reference to a structured, collection item definition on demand.
func TestResolveTypeRefSeenForwardStruct(t *testing.T) {
	defs, diags := compileSrc2(t, forwardStructModel)
	if diags.HasErrors() {
		t.Fatalf("forward struct ref should resolve cleanly: %+v", diags)
	}
	dec, err := defs.Decision("UseA")
	if err != nil {
		t.Fatal(err)
	}
	res, err := dec.Evaluate(context.Background(), dmn.Input{"A": []any{
		map[string]any{"v": 1}, map[string]any{"v": 2},
	}})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.Outputs["UseA"] != "2" {
		t.Errorf("count(A) = %v, want 2", res.Outputs["UseA"])
	}
}

// --- buildConstraints: unparsable allowed-values is dropped -------------------

const badAllowedModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/badallow" name="BadAllow" id="def_badallow">
  <inputData id="i_c" name="C"><variable name="C" typeRef="string"/></inputData>
  <decision id="d_c" name="UseC">
    <informationRequirement><requiredInput href="#i_c"/></informationRequirement>
    <decisionTable hitPolicy="UNIQUE">
      <input id="in1" label="C"><inputExpression typeRef="string"><text>C</text></inputExpression>
        <inputValues><text>"a" "b"</text></inputValues>
      </input>
      <output id="out1" name="r" typeRef="string"/>
      <rule><inputEntry><text>-</text></inputEntry><outputEntry><text>"ok"</text></outputEntry></rule>
    </decisionTable>
  </decision>
</definitions>`

// TestBuildConstraintsDropsUnparsableAllowedValues covers buildConstraints's
// branch where an allowed-values text fails to compile and is dropped, leaving the
// input unconstrained rather than failing the whole compile.
func TestBuildConstraintsDropsUnparsableAllowedValues(t *testing.T) {
	defs, diags := compileSrc2(t, badAllowedModel)
	if diags.HasErrors() {
		t.Fatalf("an unparsable constraint should be dropped, not error the compile: %+v", diags)
	}
	dec, err := defs.Decision("UseC")
	if err != nil {
		t.Fatal(err)
	}
	// With the constraint dropped, any string is accepted under strict validation.
	if probs := dec.ValidateInput(dmn.Input{"C": "anything"}); len(probs) != 0 {
		t.Errorf("constraint should have been dropped, got %+v", probs)
	}
}

// --- schema.go: ModelInputSchema merges a closed enum arriving from a later
// decision (ValuesClosed-second branch) ---------------------------------------

const closedSecondModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/closed2" name="Closed2" id="def_closed2">
  <inputData id="i_color" name="Color"><variable name="Color"/></inputData>
  <decision id="d_first" name="First">
    <informationRequirement><requiredInput href="#i_color"/></informationRequirement>
    <decisionTable hitPolicy="UNIQUE">
      <input id="fi1" label="Color"><inputExpression><text>Color</text></inputExpression></input>
      <output id="fo1" name="r1" typeRef="string"/>
      <rule><inputEntry><text>"red"</text></inputEntry><outputEntry><text>"x"</text></outputEntry></rule>
    </decisionTable>
  </decision>
  <decision id="d_second" name="Second">
    <informationRequirement><requiredInput href="#i_color"/></informationRequirement>
    <decisionTable hitPolicy="UNIQUE">
      <input id="si1" label="Color"><inputExpression><text>Color</text></inputExpression>
        <inputValues><text>"red","green","blue"</text></inputValues>
      </input>
      <output id="so1" name="r2" typeRef="string"/>
      <rule><inputEntry><text>-</text></inputEntry><outputEntry><text>"ok"</text></outputEntry></rule>
    </decisionTable>
  </decision>
</definitions>`

// TestModelInputSchemaClosedEnumFromLaterDecision covers ModelInputSchema's branch
// where a closed enumeration arrives from a later decision after an earlier
// decision contributed only open suggestions: the closed set wins.
func TestModelInputSchemaClosedEnumFromLaterDecision(t *testing.T) {
	defs, diags := compileSrc2(t, closedSecondModel)
	if diags.HasErrors() {
		t.Fatalf("compile errors: %+v", diags)
	}
	var color *dmn.InputField
	for _, f := range defs.ModelInputSchema() {
		if f.Name == "Color" {
			c := f
			color = &c
		}
	}
	if color == nil {
		t.Fatal("Color missing from model schema")
	}
	if !color.ValuesClosed {
		t.Errorf("a closed enumeration from a later decision should win: %+v", color)
	}
}

// --- graphedit.go: BKM upsert + an edge touching an unknown node -------------

// TestApplyGraphBKMAndDanglingEdge covers ApplyGraph's businessKnowledgeModel
// upsert branch and its skip of an edge whose endpoint is not a desired node.
func TestApplyGraphBKMAndDanglingEdge(t *testing.T) {
	src := readModel(t, "dish_15.dmn")
	edit := graphEdit(t, src)
	edit.Nodes = append(edit.Nodes, dmn.GraphNodeEdit{
		ID: "id_bkm_new", Type: "businessKnowledgeModel", Name: "Helper", X: 700, Y: 50, Width: 150, Height: 60,
	})
	// An edge referencing a node not in the desired set must be silently skipped.
	edit.Edges = append(edit.Edges, dmn.GraphEdgeEdit{
		Type: "knowledgeRequirement", Source: "id_bkm_new", Target: "ghost-node",
	})
	out, err := dmn.ApplyGraph(src, edit)
	if err != nil {
		t.Fatalf("ApplyGraph: %v", err)
	}
	defs, _, err := dmn.New().Compile(context.Background(), out)
	if err != nil {
		t.Fatalf("recompile: %v", err)
	}
	foundBKM := false
	for _, n := range defs.Graph().Nodes {
		if n.Type == "businessKnowledgeModel" && n.Name == "Helper" {
			foundBKM = true
		}
	}
	if !foundBKM {
		t.Error("the upserted BKM node is missing after ApplyGraph")
	}
}

// --- graphedit.go: a model WITHOUT DMNDI gets a synthesised diagram -----------

const noDIModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/nodi" name="NoDI" id="def_nodi">
  <inputData id="i_n" name="N"><variable name="N" typeRef="number"/></inputData>
  <decision id="d_n" name="UseN">
    <informationRequirement><requiredInput href="#i_n"/></informationRequirement>
    <literalExpression><text>N + 1</text></literalExpression>
  </decision>
</definitions>`

// TestApplyGraphSynthesisesDIWhenAbsent covers ApplyGraph's no-DMNDI branch: it
// builds a diagram from the supplied node bounds so the layout persists.
func TestApplyGraphSynthesisesDIWhenAbsent(t *testing.T) {
	src := []byte(noDIModel)
	defs, _ := compileSrc2(t, noDIModel)
	var edit dmn.GraphEdit
	for _, n := range defs.Graph().Nodes {
		edit.Nodes = append(edit.Nodes, dmn.GraphNodeEdit{
			ID: n.ID, Type: n.Type, Name: n.Name, DataType: n.DataType,
			X: 100, Y: 100, Width: 150, Height: 70,
		})
	}
	edit.Edges = append(edit.Edges, dmn.GraphEdgeEdit{Type: "informationRequirement", Source: "i_n", Target: "d_n"})
	out, err := dmn.ApplyGraph(src, edit)
	if err != nil {
		t.Fatalf("ApplyGraph: %v", err)
	}
	// The patched model now carries DMNDI, so the recompiled graph has bounds.
	recompiled, _ := compileSrc2(t, string(out))
	hasBounds := false
	for _, n := range recompiled.Graph().Nodes {
		if n.Width > 0 && n.Height > 0 {
			hasBounds = true
		}
	}
	if !hasBounds {
		t.Error("ApplyGraph should have synthesised a diagram with bounds")
	}
}

// --- graphedit.go: a node with an empty id is skipped -------------------------

// TestApplyGraphSkipsEmptyNodeID covers the empty-id skips in ApplyGraph's create
// and shape loops.
func TestApplyGraphSkipsEmptyNodeID(t *testing.T) {
	src := readModel(t, "dish_15.dmn")
	edit := graphEdit(t, src)
	edit.Nodes = append(edit.Nodes, dmn.GraphNodeEdit{ID: "", Type: "decision", Name: "Ghost", Width: 10, Height: 10})
	out, err := dmn.ApplyGraph(src, edit)
	if err != nil {
		t.Fatalf("ApplyGraph: %v", err)
	}
	// Still compiles; the empty-id node was ignored.
	compileSrc2(t, string(out))
}

// --- edit.go: an edit with an empty id is skipped ----------------------------

// TestApplyEditsSkipsEmptyID covers ApplyEdits's empty-id continue.
func TestApplyEditsSkipsEmptyID(t *testing.T) {
	src := readModel(t, "dish_15.dmn")
	out, err := dmn.ApplyEdits(src, []dmn.NodeEdit{{ID: "", Name: strptr("X")}})
	if err != nil {
		t.Fatalf("ApplyEdits: %v", err)
	}
	compileSrc2(t, string(out))
}

// --- bkm.go: BKMFunction view of a BKM with no body (nil) ---------------------

const bkmNilBodyModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/bkmnil" name="BkmNil" id="def_bkmnil">
  <businessKnowledgeModel id="bkm_empty" name="Empty">
    <encapsulatedLogic>
      <formalParameter name="x" typeRef="number"/>
    </encapsulatedLogic>
  </businessKnowledgeModel>
</definitions>`

// TestBKMFunctionNilBody covers BKMFunction's nil-body case: a BKM whose
// encapsulated logic has parameters but no body is still a simple (empty) function.
func TestBKMFunctionNilBody(t *testing.T) {
	defs, _ := compileSrc2(t, bkmNilBodyModel)
	v, ok := defs.BKMFunction("Empty")
	if !ok {
		t.Fatal("Empty BKM not found")
	}
	if !v.Simple {
		t.Error("a body-less BKM should still be simple")
	}
	if v.BodyText != "" {
		t.Errorf("body should be empty, got %q", v.BodyText)
	}
	if len(v.Params) != 1 || v.Params[0].Name != "x" {
		t.Errorf("params = %+v, want one 'x'", v.Params)
	}
}
