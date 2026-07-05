// Package tck holds the runner for the official DMN Technology Compatibility
// Kit: it reads TCK .dmn models plus their test definitions (the standard
// `testCases` XML), evaluates each case through the public engine and reports
// pass/fail per case. The TCK is the central correctness measure for the project
// (docs/50-testing-strategy.md). Conformance over the full official corpus is
// measured in WP-41; this package is the runner it builds on.
package tck

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/pblumer/temis/dmn"
	"github.com/pblumer/temis/internal/value"
)

// CaseResult is the outcome of one decision check within a test case.
type CaseResult struct {
	Case     string // test case id
	Decision string // the decision (resultNode) checked
	Pass     bool
	Expected any
	Got      any
	Err      error // non-nil when the evaluation itself failed
}

// Report is the result of running a test-case suite against a model.
type Report struct {
	Model   string
	Results []CaseResult
}

// Passed reports the number of passing checks.
func (r *Report) Passed() int {
	n := 0
	for _, c := range r.Results {
		if c.Pass {
			n++
		}
	}
	return n
}

// Failed reports the number of failing checks.
func (r *Report) Failed() int { return len(r.Results) - r.Passed() }

// OK reports whether every check passed.
func (r *Report) OK() bool { return r.Failed() == 0 }

// Summary renders a one-line-per-failure human report ending with a tally.
func (r *Report) Summary() string {
	s := fmt.Sprintf("TCK %s: %d/%d passed", r.Model, r.Passed(), len(r.Results))
	for _, c := range r.Results {
		if c.Pass {
			continue
		}
		if c.Err != nil {
			s += fmt.Sprintf("\n  FAIL %s/%s: %v", c.Case, c.Decision, c.Err)
		} else {
			s += fmt.Sprintf("\n  FAIL %s/%s: got %v, want %v", c.Case, c.Decision, c.Got, c.Expected)
		}
	}
	return s
}

// Run compiles modelXML and executes every test case in casesXML against it,
// returning a Report. A compile error (or malformed cases XML) is returned as an
// error; per-case evaluation failures are recorded as failing results, not
// errors, so one bad case does not abort the suite.
func Run(ctx context.Context, eng *dmn.Engine, modelXML, casesXML []byte) (*Report, error) {
	if eng == nil {
		eng = dmn.New()
	}
	var suite testCases
	if err := xml.Unmarshal(casesXML, &suite); err != nil {
		return nil, fmt.Errorf("tck: decode test cases: %w", err)
	}

	defs, _, err := eng.Compile(ctx, modelXML)
	if err != nil {
		return nil, fmt.Errorf("tck: compile model: %w", err)
	}
	if defs == nil {
		return nil, fmt.Errorf("tck: model did not compile")
	}
	// Note: per-decision compile diagnostics do NOT abort the suite. Each case
	// evaluates its own target decision (runCheck); a decision that failed to
	// compile fails only the cases that reference it, which is the correct TCK
	// scoring — a single unsupported decision must not zero out a whole model.

	rep := &Report{Model: suite.ModelName}
	for _, tc := range suite.Cases {
		input := caseInput(tc)
		for _, rn := range tc.Results {
			if tc.Type == "decisionService" {
				rep.Results = append(rep.Results, runServiceCheck(ctx, defs, tc, rn, input))
				continue
			}
			rep.Results = append(rep.Results, runCheck(ctx, defs, tc.ID, rn, input))
		}
	}
	return rep, nil
}

// runServiceCheck evaluates one resultNode of a decision-service test case: the
// service named by invocableName is evaluated directly, which applies the
// service's output-type coercion (a case the TCK exercises with non-conforming
// outputs). The checked resultNode names one of the service's output decisions.
func runServiceCheck(ctx context.Context, defs *dmn.Definitions, tc testCase, rn resultNode, input dmn.Input) CaseResult {
	res := CaseResult{Case: tc.ID, Decision: rn.Name, Expected: goOf(rn.Expected.toValue())}
	svc, err := defs.Service(tc.Invoked)
	if err != nil {
		res.Err = err
		return res
	}
	out, err := svc.Evaluate(ctx, input)
	if err != nil {
		res.Err = err
		return res
	}
	res.Got = out.Outputs[rn.Name]
	res.Pass = resultEqual(res.Got, res.Expected)
	return res
}

// RunFile runs the suite in the testCases file at path, resolving its model from
// the <modelName> element relative to the file's directory.
func RunFile(ctx context.Context, eng *dmn.Engine, path string) (*Report, error) {
	casesXML, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var suite testCases
	if err := xml.Unmarshal(casesXML, &suite); err != nil {
		return nil, fmt.Errorf("tck: decode %s: %w", path, err)
	}
	modelXML, err := os.ReadFile(filepath.Join(filepath.Dir(path), suite.ModelName))
	if err != nil {
		return nil, fmt.Errorf("tck: read model %q: %w", suite.ModelName, err)
	}
	return Run(ctx, eng, modelXML, casesXML)
}

// runCheck evaluates one resultNode and compares the engine output to the
// expected value.
func runCheck(ctx context.Context, defs *dmn.Definitions, caseID string, rn resultNode, input dmn.Input) CaseResult {
	res := CaseResult{Case: caseID, Decision: rn.Name, Expected: goOf(rn.Expected.toValue())}
	dec, err := defs.Decision(rn.Name)
	if err != nil {
		res.Err = err
		return res
	}
	out, err := dec.Evaluate(ctx, input)
	if err != nil {
		res.Err = err
		return res
	}
	res.Got = out.Outputs[rn.Name]
	res.Pass = resultEqual(res.Got, res.Expected)
	return res
}

// caseInput builds the evaluation input from a case's input nodes, passing FEEL
// values straight through (dmn.Input accepts the engine's value type).
func caseInput(tc testCase) dmn.Input {
	in := make(dmn.Input, len(tc.Inputs))
	for _, n := range tc.Inputs {
		in[n.Name] = n.toValue()
	}
	return in
}

// goOf renders a FEEL value in the same Go representation the engine returns from
// Evaluate, so an expected value compares equal to an actual output (numbers and
// temporals as their canonical string, lists as []any, contexts as
// map[string]any, null as nil).
func goOf(v value.Value) any {
	if value.IsNull(v) {
		return nil
	}
	switch x := v.(type) {
	case value.Bool:
		return bool(x)
	case value.Str:
		return string(x)
	case value.Number:
		return x.String()
	case value.List:
		out := make([]any, len(x.Elements))
		for i, e := range x.Elements {
			out[i] = goOf(e)
		}
		return out
	case *value.Context:
		out := make(map[string]any, x.Len())
		for _, k := range x.Keys() {
			ev, _ := x.Get(k)
			out[k] = goOf(ev)
		}
		return out
	default:
		return x.String()
	}
}

// resultEqual compares an engine result to a TCK expected value. It matches
// reflect.DeepEqual structurally (lists element-wise, contexts field-wise) but,
// for two numbers, treats them as equal when they agree to the precision the
// expected value carries — see numClose. It is additive: it never fails a pair
// that reflect.DeepEqual would accept.
func resultEqual(got, want any) bool {
	switch w := want.(type) {
	case string:
		if g, ok := got.(string); ok {
			return g == w || numClose(g, w)
		}
	case []any:
		g, ok := got.([]any)
		if !ok || len(g) != len(w) {
			return false
		}
		for i := range w {
			if !resultEqual(g[i], w[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok || len(g) != len(w) {
			return false
		}
		for k, wv := range w {
			gv, ok := g[k]
			if !ok || !resultEqual(gv, wv) {
				return false
			}
		}
		return true
	}
	return reflect.DeepEqual(got, want)
}

// numClose reports whether two FEEL number strings agree at the fractional
// precision the expected value states. The engine follows decimal128 (34
// significant digits, ADR-0007); transcendental and irrational results (exp, log,
// sqrt, `**` with a non-integer exponent, statistics) carry more digits than the
// TCK's expected value, whose author rounded to a finite precision. Rounding the
// actual result to the expected value's number of fractional digits and comparing
// honours that stated precision without weakening exact-arithmetic checks: an
// integer or full-precision expectation still compares exactly, and any deviation
// larger than the last stated digit still fails. Values that do not both parse as
// numbers are never treated as close.
func numClose(got, want string) bool {
	dot := -1
	for i := 0; i < len(want); i++ {
		if want[i] == '.' {
			dot = i
			break
		}
	}
	if dot < 0 { // an integer expectation is compared exactly
		return false
	}
	scale := len(want) - dot - 1
	gn, err1 := value.ParseNumber(got)
	wn, err2 := value.ParseNumber(want)
	if err1 != nil || err2 != nil {
		return false
	}
	gr, ok := gn.RoundHalfUp(int32(scale))
	if !ok {
		return false
	}
	return gr.Cmp(wn) == 0
}
