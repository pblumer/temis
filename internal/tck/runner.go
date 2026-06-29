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

	defs, diags, err := eng.Compile(ctx, modelXML)
	if err != nil {
		return nil, fmt.Errorf("tck: compile model: %w", err)
	}
	if diags.HasErrors() {
		return nil, fmt.Errorf("tck: model has compile errors: %v", diags)
	}

	rep := &Report{Model: suite.ModelName}
	for _, tc := range suite.Cases {
		input := caseInput(tc)
		for _, rn := range tc.Results {
			rep.Results = append(rep.Results, runCheck(ctx, defs, tc.ID, rn, input))
		}
	}
	return rep, nil
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
	res.Pass = reflect.DeepEqual(res.Got, res.Expected)
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
