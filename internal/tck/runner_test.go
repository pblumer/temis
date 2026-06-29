package tck

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunFilePasses(t *testing.T) {
	rep, err := RunFile(context.Background(), nil, filepath.Join("testdata", "tckdemo-test.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK() {
		t.Fatalf("expected all checks to pass:\n%s", rep.Summary())
	}
	// t1 has 3 checks, t2 has 2.
	if len(rep.Results) != 5 || rep.Passed() != 5 {
		t.Errorf("results = %d passed %d, want 5/5", len(rep.Results), rep.Passed())
	}
}

func TestRunReportsFailures(t *testing.T) {
	model := readTestdata(t, "tckdemo.dmn")
	// A wrong expectation and an unknown decision must surface as failures, not
	// abort the run.
	cases := []byte(`<?xml version="1.0"?>
<testCases xmlns="http://www.omg.org/spec/DMN/20160719/testcase" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <modelName>tckdemo.dmn</modelName>
  <testCase id="bad">
    <inputNode name="Age"><value xsi:type="xsd:decimal">20</value></inputNode>
    <resultNode name="Category" type="decision"><expected><value xsi:type="xsd:string">minor</value></expected></resultNode>
    <resultNode name="Nonexistent" type="decision"><expected><value xsi:type="xsd:string">x</value></expected></resultNode>
  </testCase>
</testCases>`)
	rep, err := Run(context.Background(), nil, model, cases)
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK() || rep.Failed() != 2 {
		t.Fatalf("want 2 failures, got %d:\n%s", rep.Failed(), rep.Summary())
	}
	// One failure is a mismatch (got "adult", want "minor"), the other an error.
	var sawMismatch, sawErr bool
	for _, c := range rep.Results {
		switch {
		case c.Decision == "Category" && !c.Pass && c.Err == nil && c.Got == "adult":
			sawMismatch = true
		case c.Decision == "Nonexistent" && c.Err != nil:
			sawErr = true
		}
	}
	if !sawMismatch || !sawErr {
		t.Errorf("expected a mismatch and an error result: %s", rep.Summary())
	}
}

func TestRunBadModel(t *testing.T) {
	cases := readTestdata(t, "tckdemo-test.xml")
	if _, err := Run(context.Background(), nil, []byte("<not-dmn/>"), cases); err == nil {
		t.Error("a model that does not compile should error")
	}
}

func TestSummaryFormat(t *testing.T) {
	rep := &Report{Model: "m.dmn", Results: []CaseResult{
		{Case: "a", Decision: "D", Pass: true},
		{Case: "b", Decision: "E", Pass: false, Got: "x", Expected: "y"},
	}}
	s := rep.Summary()
	if !strings.Contains(s, "1/2 passed") || !strings.Contains(s, "FAIL b/E") {
		t.Errorf("summary = %q", s)
	}
}

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}
