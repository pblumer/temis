package flow_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/pblumer/temis/dmn"
	"github.com/pblumer/temis/flow"
)

const (
	riskID = "sha256:risk-model"
	loanID = "sha256:loan-model"
	svcID  = "sha256:svc-model"
)

// loadModel compiles a testdata DMN file into a *dmn.Definitions.
func loadModel(t *testing.T, path string) *dmn.Definitions {
	t.Helper()
	xml, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	defs, diags, err := dmn.New().Compile(context.Background(), xml)
	if err != nil {
		t.Fatalf("compile %s: %v", path, err)
	}
	if diags.HasErrors() {
		t.Fatalf("compile %s: diagnostics: %v", path, diags)
	}
	return defs
}

// resolver builds a MapResolver over the two flow testdata models.
func resolver(t *testing.T) flow.MapResolver {
	t.Helper()
	return flow.MapResolver{
		riskID: loadModel(t, "testdata/risk.dmn"),
		loanID: loadModel(t, "testdata/loan.dmn"),
		svcID:  loadModel(t, "testdata/service.dmn"),
	}
}

const loanFlowJSON = `{
  "flow": "loan-decisioning",
  "inputs": [
    {"name": "Credit Score", "type": "number"},
    {"name": "Applicant Age", "type": "number"}
  ],
  "steps": [
    {"id": "risk", "model": "sha256:risk-model", "decision": "Risk Level",
     "in": {"Credit Score": "Credit Score"}},
    {"id": "decide", "model": "sha256:loan-model", "decision": "Loan Decision",
     "in": {"Risk": "risk.Risk Level", "Applicant Age": "Applicant Age"}}
  ],
  "output": {"Decision": "decide.Loan Decision"}
}`

func compile(t *testing.T, src string) *flow.Flow {
	t.Helper()
	f, diags, err := flow.Compile([]byte(src))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("compile diagnostics: %v", diags)
	}
	return f
}

func TestEvaluate(t *testing.T) {
	f := compile(t, loanFlowJSON)
	r := resolver(t)

	cases := []struct {
		name  string
		score int
		age   int
		want  string
	}{
		{"good score, adult -> approve", 750, 30, "approve"},
		{"bad score -> decline", 550, 30, "decline"},
		{"medium score, minor -> decline", 650, 16, "decline"},
		{"medium score, adult -> review", 650, 40, "review"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := f.Evaluate(context.Background(),
				dmn.Input{"Credit Score": tc.score, "Applicant Age": tc.age}, r)
			if err != nil {
				t.Fatalf("evaluate: %v", err)
			}
			if got := res.Outputs["Decision"]; got != tc.want {
				t.Fatalf("Decision = %v, want %q", got, tc.want)
			}
			// The intermediate step output is exposed for transparency.
			if _, ok := res.Decisions["risk.Risk Level"]; !ok {
				t.Fatalf("Decisions missing risk.Risk Level: %v", res.Decisions)
			}
		})
	}
}

func TestEvaluateDeterministic(t *testing.T) {
	f := compile(t, loanFlowJSON)
	r := resolver(t)
	in := dmn.Input{"Credit Score": 650, "Applicant Age": 40}

	first, err := f.Evaluate(context.Background(), in, r)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	for i := 0; i < 5; i++ {
		got, err := f.Evaluate(context.Background(), in, r)
		if err != nil {
			t.Fatalf("evaluate: %v", err)
		}
		if got.Outputs["Decision"] != first.Outputs["Decision"] {
			t.Fatalf("non-deterministic: %v vs %v", got.Outputs, first.Outputs)
		}
	}
}

// TestTopologicalOrder proves the evaluation order follows the reference graph,
// not the array order: "decide" is listed before its dependency "risk".
func TestTopologicalOrder(t *testing.T) {
	src := `{
      "flow": "reordered",
      "inputs": [{"name":"Credit Score","type":"number"},{"name":"Applicant Age","type":"number"}],
      "steps": [
        {"id": "decide", "model": "sha256:loan-model", "decision": "Loan Decision",
         "in": {"Risk": "risk.Risk Level", "Applicant Age": "Applicant Age"}},
        {"id": "risk", "model": "sha256:risk-model", "decision": "Risk Level",
         "in": {"Credit Score": "Credit Score"}}
      ],
      "output": {"Decision": "decide.Loan Decision"}
    }`
	f := compile(t, src)
	res, err := f.Evaluate(context.Background(),
		dmn.Input{"Credit Score": 750, "Applicant Age": 30}, resolver(t))
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got := res.Outputs["Decision"]; got != "approve" {
		t.Fatalf("Decision = %v, want approve", got)
	}
}

// TestOutputFallback: with no declared output, the flow returns the last step's
// outputs.
func TestOutputFallback(t *testing.T) {
	src := `{
      "flow": "fallback",
      "inputs": [{"name":"Credit Score","type":"number"}],
      "steps": [
        {"id": "risk", "model": "sha256:risk-model", "decision": "Risk Level",
         "in": {"Credit Score": "Credit Score"}}
      ]
    }`
	f := compile(t, src)
	res, err := f.Evaluate(context.Background(), dmn.Input{"Credit Score": 800}, resolver(t))
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got := res.Outputs["Risk Level"]; got != "low" {
		t.Fatalf("Risk Level = %v, want low", got)
	}
}

func TestCompileStructural(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		wantErr bool // true when Compile returns a Go error (malformed JSON)
		code    string
	}{
		{"malformed json", `{not json`, true, flow.CodeMalformed},
		{"no steps", `{"flow":"x","steps":[]}`, false, flow.CodeNoSteps},
		{"duplicate step", `{"flow":"x","steps":[
            {"id":"a","model":"m","decision":"d"},
            {"id":"a","model":"m","decision":"d"}]}`, false, flow.CodeDuplicateStep},
		{"missing model", `{"flow":"x","steps":[{"id":"a","decision":"d"}]}`, false, flow.CodeMissingField},
		{"cycle", `{"flow":"x","steps":[
            {"id":"a","model":"m","decision":"d","in":{"x":"b.d"}},
            {"id":"b","model":"m","decision":"d","in":{"y":"a.d"}}]}`, false, flow.CodeCycle},
		// A name that is neither a step output nor a declared input is an
		// undeclared FEEL variable → the mapping does not compile.
		{"undeclared name", `{"flow":"x","inputs":[{"name":"Known"}],"steps":[
            {"id":"a","model":"m","decision":"d","in":{"x":"Missing"}}]}`, false, flow.CodeMappingInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, diags, err := flow.Compile([]byte(tc.src))
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got none")
			}
			if !hasCode(diags, tc.code) {
				t.Fatalf("expected diagnostic %s, got %v", tc.code, diags)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	r := resolver(t)
	cases := []struct {
		name string
		src  string
		code string
	}{
		{
			"unresolved model",
			`{"flow":"x","inputs":[{"name":"Credit Score","type":"number"}],"steps":[
                {"id":"a","model":"sha256:missing","decision":"Risk Level","in":{"Credit Score":"Credit Score"}}]}`,
			flow.CodeModelUnresolved,
		},
		{
			"target not found",
			`{"flow":"x","inputs":[{"name":"Credit Score","type":"number"}],"steps":[
                {"id":"a","model":"sha256:risk-model","decision":"Nope","in":{"Credit Score":"Credit Score"}}]}`,
			flow.CodeTargetNotFound,
		},
		{
			"required input unwired",
			`{"flow":"x","steps":[
                {"id":"a","model":"sha256:risk-model","decision":"Risk Level","in":{}}]}`,
			flow.CodeInputUnwired,
		},
		{
			"unknown input wired",
			`{"flow":"x","inputs":[{"name":"Credit Score","type":"number"}],"steps":[
                {"id":"a","model":"sha256:risk-model","decision":"Risk Level",
                 "in":{"Credit Score":"Credit Score","Bogus":"Credit Score"}}]}`,
			flow.CodeUnknownInput,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, _, err := flow.Compile([]byte(tc.src))
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			diags := f.Validate(context.Background(), r)
			if !hasCode(diags, tc.code) {
				t.Fatalf("expected diagnostic %s, got %v", tc.code, diags)
			}
			// Evaluate must refuse with the same diagnostics.
			if _, err := f.Evaluate(context.Background(), dmn.Input{}, r); err == nil {
				t.Fatalf("expected Evaluate to refuse")
			} else {
				var fe *flow.Error
				if !errors.As(err, &fe) {
					t.Fatalf("expected *flow.Error, got %T: %v", err, err)
				}
			}
		})
	}
}

func TestMaxSteps(t *testing.T) {
	f := compile(t, loanFlowJSON)
	_, err := f.Evaluate(context.Background(),
		dmn.Input{"Credit Score": 750, "Applicant Age": 30}, resolver(t), flow.WithMaxSteps(1))
	if err == nil {
		t.Fatalf("expected MaxSteps error")
	}
	var fe *flow.Error
	if !errors.As(err, &fe) || !hasCode(fe.Diagnostics, flow.CodeMaxSteps) {
		t.Fatalf("expected FLOW_MAX_STEPS, got %v", err)
	}
}

// TestServiceStep composes a decision *service* as a flow step.
func TestServiceStep(t *testing.T) {
	src := `{
      "flow": "svc",
      "inputs": [{"name":"Applicant Age","type":"number"}],
      "steps": [
        {"id": "appr", "model": "sha256:svc-model", "decision": "Approval",
         "in": {"Applicant Age": "Applicant Age"}}
      ],
      "output": {"Result": "appr.Routing"}
    }`
	f := compile(t, src)
	r := resolver(t)

	for _, tc := range []struct {
		age  int
		want string
	}{{30, "ACCEPT"}, {16, "DECLINE"}} {
		res, err := f.Evaluate(context.Background(), dmn.Input{"Applicant Age": tc.age}, r)
		if err != nil {
			t.Fatalf("evaluate age=%d: %v", tc.age, err)
		}
		if got := res.Outputs["Result"]; got != tc.want {
			t.Fatalf("age=%d: Result = %v, want %q", tc.age, got, tc.want)
		}
	}
}

// TestEvaluateRuntimeError: a value that passes wiring validation but fails the
// step's strict input check surfaces as a wrapped error, not a silent null.
func TestEvaluateRuntimeError(t *testing.T) {
	f := compile(t, loanFlowJSON)
	// "abc" is not numeric, so coerce leaves it a string and the risk decision's
	// strict validation rejects it (number expected).
	_, err := f.Evaluate(context.Background(),
		dmn.Input{"Credit Score": "abc", "Applicant Age": 30}, resolver(t))
	if err == nil {
		t.Fatalf("expected a runtime evaluation error")
	}
}

func TestAccessors(t *testing.T) {
	f := compile(t, loanFlowJSON)
	if f.Name() != "loan-decisioning" {
		t.Fatalf("Name = %q", f.Name())
	}
	if f.Diagnostics().HasErrors() {
		t.Fatalf("unexpected structural diagnostics: %v", f.Diagnostics())
	}
}

func TestErrorFormatting(t *testing.T) {
	d := flow.Diagnostics{
		{Code: flow.CodeCycle, Message: "cyclic"},
		{Code: flow.CodeInputUnwired, Step: "a", Message: "missing"},
	}
	if !d.HasErrors() {
		t.Fatalf("HasErrors should be true")
	}
	got := d.Error()
	want := "FLOW_CYCLE: cyclic; FLOW_INPUT_UNWIRED [a]: missing"
	if got != want {
		t.Fatalf("Diagnostics.Error() = %q, want %q", got, want)
	}
	fe := &flow.Error{Diagnostics: d}
	if fe.Error() != "flow: "+want {
		t.Fatalf("Error.Error() = %q", fe.Error())
	}
	if (flow.Diagnostics{}).Error() != "" {
		t.Fatalf("empty diagnostics should render empty string")
	}
}

// TestEvaluateWithTrace: explain aggregates the decision-table traces of every
// decision step in evaluation order.
func TestEvaluateWithTrace(t *testing.T) {
	f := compile(t, loanFlowJSON)
	res, err := f.Evaluate(context.Background(),
		dmn.Input{"Credit Score": 750, "Applicant Age": 30}, resolver(t), flow.WithTrace())
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.Trace == nil {
		t.Fatalf("expected a trace")
	}
	// One table per decision step (risk, decide).
	if len(res.Trace.Tables) != 2 {
		t.Fatalf("expected 2 table traces, got %d", len(res.Trace.Tables))
	}
	// Without the option, no trace.
	plain, err := f.Evaluate(context.Background(),
		dmn.Input{"Credit Score": 750, "Applicant Age": 30}, resolver(t))
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if plain.Trace != nil {
		t.Fatalf("expected no trace without WithTrace")
	}
}

// TestFeelMappingArithmetic: a mapping is a full FEEL expression — arithmetic on
// a flow input changes the outcome (800-200=600 → medium, not low).
func TestFeelMappingArithmetic(t *testing.T) {
	src := `{
      "flow":"feel",
      "inputs":[{"name":"Credit Score","type":"number"},{"name":"Applicant Age","type":"number"}],
      "steps":[
        {"id":"risk","model":"sha256:risk-model","decision":"Risk Level","in":{"Credit Score":"Credit Score - 200"}},
        {"id":"decide","model":"sha256:loan-model","decision":"Loan Decision","in":{"Risk":"risk.Risk Level","Applicant Age":"Applicant Age"}}
      ],
      "output":{"Decision":"decide.Loan Decision"}
    }`
	f := compile(t, src)
	res, err := f.Evaluate(context.Background(), dmn.Input{"Credit Score": 800, "Applicant Age": 40}, resolver(t))
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got := res.Outputs["Decision"]; got != "review" {
		t.Fatalf("Decision = %v, want review (FEEL arithmetic 800-200=600→medium)", got)
	}
}

// TestFeelMappingConditional: an `if` expression over a flow input.
func TestFeelMappingConditional(t *testing.T) {
	src := `{
      "flow":"cond",
      "inputs":[{"name":"Credit Score","type":"number"},{"name":"Under Age","type":"boolean"}],
      "steps":[
        {"id":"risk","model":"sha256:risk-model","decision":"Risk Level","in":{"Credit Score":"Credit Score"}},
        {"id":"decide","model":"sha256:loan-model","decision":"Loan Decision",
         "in":{"Risk":"risk.Risk Level","Applicant Age":"if Under Age then 16 else 40"}}
      ],
      "output":{"Decision":"decide.Loan Decision"}
    }`
	f := compile(t, src)
	r := resolver(t)
	for _, tc := range []struct {
		under bool
		want  string
	}{{true, "decline"}, {false, "approve"}} { // 750→low; minor→decline, adult→approve
		res, err := f.Evaluate(context.Background(), dmn.Input{"Credit Score": 750, "Under Age": tc.under}, r)
		if err != nil {
			t.Fatalf("evaluate: %v", err)
		}
		if got := res.Outputs["Decision"]; got != tc.want {
			t.Fatalf("Under Age=%v: Decision = %v, want %q", tc.under, got, tc.want)
		}
	}
}

// TestFeelMappingDependencyOrder: a FEEL mapping that names another step (inside
// an expression) still creates a dependency, so the topological order is correct
// even when the dependent step is listed first.
func TestFeelMappingDependencyOrder(t *testing.T) {
	src := `{
      "flow":"deporder",
      "inputs":[{"name":"Credit Score","type":"number"},{"name":"Applicant Age","type":"number"}],
      "steps":[
        {"id":"decide","model":"sha256:loan-model","decision":"Loan Decision",
         "in":{"Risk":"risk.Risk Level","Applicant Age":"if get value(risk, \"Risk Level\") = \"high\" then 16 else Applicant Age"}},
        {"id":"risk","model":"sha256:risk-model","decision":"Risk Level","in":{"Credit Score":"Credit Score"}}
      ],
      "output":{"Decision":"decide.Loan Decision"}
    }`
	f := compile(t, src)
	// If ordering were wrong (decide before risk), risk.Risk Level would be unresolved.
	res, err := f.Evaluate(context.Background(), dmn.Input{"Credit Score": 750, "Applicant Age": 40}, resolver(t))
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got := res.Outputs["Decision"]; got != "approve" {
		t.Fatalf("Decision = %v, want approve", got)
	}
}

func TestFeelMappingInvalid(t *testing.T) {
	for _, tc := range []struct{ name, expr string }{
		{"syntax error", "Credit Score +"},
		{"undeclared name", "Bogus + 1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := fmt.Sprintf(`{"flow":"bad","inputs":[{"name":"Credit Score","type":"number"}],"steps":[
                {"id":"risk","model":"sha256:risk-model","decision":"Risk Level","in":{"Credit Score":%q}}]}`, tc.expr)
			_, diags, err := flow.Compile([]byte(src))
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if !hasCode(diags, flow.CodeMappingInvalid) {
				t.Fatalf("want FLOW_MAPPING_INVALID, got %v", diags)
			}
		})
	}
}

func hasCode(diags flow.Diagnostics, code string) bool {
	for _, d := range diags {
		if d.Code == code {
			return true
		}
	}
	return false
}
