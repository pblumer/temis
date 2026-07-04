package tck

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pblumer/temis/dmn"
	"github.com/pblumer/temis/internal/value"
)

// TestScalarValueTemporals exercises the time/datetime/duration branches of
// scalarValue (both the parse-success and the unparsable-null path) plus the
// default-typed string fallback, which the existing convert_test does not reach.
func TestScalarValueTemporals(t *testing.T) {
	ok := []struct{ typ, text string }{
		{"xsd:time", "03:04:05"},
		{"xsd:dayTimeDuration", "P1D"},
		{"xsd:yearMonthDuration", "P1Y"},
	}
	for _, c := range ok {
		if got := scalarValue(c.typ, c.text); value.IsNull(got) {
			t.Errorf("scalarValue(%q,%q) is null, want a value", c.typ, c.text)
		}
	}

	// Unparsable typed values collapse to null on every typed branch.
	for _, c := range []struct{ typ, text string }{
		{"xsd:date", "nope"},
		{"xsd:time", "nope"},
		{"xsd:dateTime", "nope"},
		{"xsd:dayTimeDuration", "nope"},
	} {
		if !value.IsNull(scalarValue(c.typ, c.text)) {
			t.Errorf("scalarValue(%q,%q) should be null", c.typ, c.text)
		}
	}

	// A non-empty value of an unrecognised type falls through to a string.
	if got := scalarValue("xsd:anyURI", "http://x"); got.String() != "http://x" {
		t.Errorf("unknown-typed value = %s, want http://x", got)
	}
}

// TestTckValueNestedValue covers the v.Value != nil branch of tckValue.toValue,
// where a <value> directly nests another <value>.
func TestTckValueNestedValue(t *testing.T) {
	v := tckValue{valueContent: valueContent{
		Value: &tckValue{Type: "xsd:decimal", Text: "5"},
	}}
	if got := v.toValue(); got.String() != "5" {
		t.Errorf("nested value toValue = %s, want 5", got)
	}

	// A tckValue carrying a structured <list>.
	lst := tckValue{valueContent: valueContent{List: &tckList{Values: []tckValue{
		{Type: "xsd:decimal", Text: "1"}, {Type: "xsd:decimal", Text: "2"},
	}}}}
	if got := lst.toValue(); got.String() != "[1, 2]" {
		t.Errorf("tckValue list toValue = %s, want [1, 2]", got)
	}

	// A tckValue carrying <component>s (a context).
	cmp := tckValue{valueContent: valueContent{Components: []tckComponent{
		{Name: "x", tckValue: tckValue{Type: "xsd:string", Text: "hi"}},
	}}}
	if got := cmp.toValue(); got.String() != "{x: hi}" {
		t.Errorf("tckValue components toValue = %s, want {x: hi}", got)
	}
}

// TestRunFileErrors covers RunFile's three failure paths: missing cases file,
// malformed cases XML, and a model file that cannot be read.
func TestRunFileErrors(t *testing.T) {
	// Missing cases file.
	if _, err := RunFile(context.Background(), nil, filepath.Join(t.TempDir(), "absent.xml")); err == nil {
		t.Error("missing cases file should error")
	}

	// Malformed cases XML.
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.xml")
	if err := os.WriteFile(bad, []byte("<testCases><not-closed>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := RunFile(context.Background(), nil, bad); err == nil {
		t.Error("malformed cases XML should error")
	}

	// Valid cases XML but the referenced model file is absent.
	noModel := filepath.Join(dir, "nomodel.xml")
	xml := `<?xml version="1.0"?>
<testCases xmlns="http://www.omg.org/spec/DMN/20160719/testcase">
  <modelName>does-not-exist.dmn</modelName>
</testCases>`
	if err := os.WriteFile(noModel, []byte(xml), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := RunFile(context.Background(), nil, noModel); err == nil {
		t.Error("missing model file should error")
	}
}

// TestRunDecodeError covers Run's cases-XML decode-error path.
func TestRunDecodeError(t *testing.T) {
	model := readTestdata(t, "tckdemo.dmn")
	if _, err := Run(context.Background(), nil, model, []byte("<testCases><unterminated>")); err == nil {
		t.Error("malformed cases XML should error")
	}
}

// TestRunModelDiagErrors covers the per-case behaviour when a decision fails to
// compile: Run no longer aborts the whole suite (correct TCK scoring). A case
// targeting the broken decision must FAIL (not pass, not error the suite), so an
// unsupported decision only costs its own cases.
func TestRunModelDiagErrors(t *testing.T) {
	model := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/bad" name="Bad" id="def_bad">
  <decision id="d1" name="Bad"><variable name="Bad"/><literalExpression><text>1 + + +</text></literalExpression></decision>
</definitions>`)
	cases := []byte(`<?xml version="1.0"?>
<testCases xmlns="http://www.omg.org/spec/DMN/20160719/testcase">
  <modelName>bad.dmn</modelName>
  <testCase id="c1">
    <resultNode name="Bad"><expected><value>2</value></expected></resultNode>
  </testCase>
</testCases>`)
	rep, err := Run(context.Background(), nil, model, cases)
	if err != nil {
		t.Fatalf("Run should not abort the suite on a per-decision compile error: %v", err)
	}
	if rep.Passed() != 0 || rep.Failed() != 1 {
		t.Errorf("expected the broken decision's case to fail (pass=0 fail=1), got pass=%d fail=%d", rep.Passed(), rep.Failed())
	}
}

// TestRunCheckEvaluateError covers runCheck's Evaluate-error branch (distinct
// from the unknown-decision branch) by passing a cancelled context.
func TestRunCheckEvaluateError(t *testing.T) {
	model := readTestdata(t, "tckdemo.dmn")
	defs, diags, err := dmn.New().Compile(context.Background(), model)
	if err != nil || diags.HasErrors() {
		t.Fatalf("compile: err=%v diags=%v", err, diags)
	}
	cctx, cancel := context.WithCancel(context.Background())
	cancel()
	rn := resultNode{Name: "Discount", Expected: expectedNode{}}
	res := runCheck(cctx, defs, "c", rn, dmn.Input{"Member": value.BoolOf(true)})
	if res.Err == nil {
		t.Errorf("cancelled context should surface an Evaluate error: %+v", res)
	}
}

// TestSummaryErrFailure covers Summary's Err != nil failure branch.
func TestSummaryErrFailure(t *testing.T) {
	rep := &Report{Model: "m.dmn", Results: []CaseResult{
		{Case: "a", Decision: "D", Pass: false, Err: context.Canceled},
	}}
	s := rep.Summary()
	if !strings.Contains(s, "FAIL a/D") || !strings.Contains(s, "canceled") {
		t.Errorf("summary = %q, want an error-shaped failure line", s)
	}
}
