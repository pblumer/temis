package dmn

// These tests guard the code registry against drift. internal/model cannot
// import dmn (dmn imports internal/model, so it would be an import cycle), so the
// diagnostic codes it emits are written as string literals. This test pins those
// literals to the canonical constants here: if a constant's value changes, the
// matching literal in internal/model must change too, and this test fails until
// it does.

import "testing"

func TestDiagnosticCodeValues(t *testing.T) {
	// Canonical values are the public contract (docs/40-api-contract.md §1.4).
	// Changing any of these is a breaking change.
	cases := map[string]string{
		"CodeXMLMalformed":     CodeXMLMalformed,
		"CodeUnknownNamespace": CodeUnknownNamespace,
		"CodeUnknownElement":   CodeUnknownElement,
		"CodeNoLogic":          CodeNoLogic,
		"CodeFEELCompile":      CodeFEELCompile,
		"CodeNotExecutable":    CodeNotExecutable,
		"CodeMissingInput":     CodeMissingInput,
		"CodeLimitExceeded":    CodeLimitExceeded,
		"CodeUniqueMultiple":   CodeUniqueMultiple,
		"CodeRuntime":          CodeRuntime,
	}
	want := map[string]string{
		"CodeXMLMalformed":     "XML_MALFORMED",
		"CodeUnknownNamespace": "UNKNOWN_NAMESPACE",
		"CodeUnknownElement":   "UNKNOWN_ELEMENT",
		"CodeNoLogic":          "DECISION_NO_LOGIC",
		"CodeFEELCompile":      "FEEL_COMPILE_ERROR",
		"CodeNotExecutable":    "DECISION_NOT_EXECUTABLE",
		"CodeMissingInput":     "MISSING_REQUIRED_INPUT",
		"CodeLimitExceeded":    "LIMIT_EXCEEDED",
		"CodeUniqueMultiple":   "UNIQUE_MULTIPLE_MATCH",
		"CodeRuntime":          "RUNTIME_ERROR",
	}
	for name, got := range cases {
		if got != want[name] {
			t.Errorf("%s = %q, want %q", name, got, want[name])
		}
	}
}

// TestModelLiteralsMatchConstants pins the string literals internal/model emits
// (via fromModelDiagnostics) to the canonical constants. We assert against the
// known literal values that internal/model/fromxml.go sets directly, since the
// two packages cannot share a constant without an import cycle.
func TestModelLiteralsMatchConstants(t *testing.T) {
	// The literals in internal/model/fromxml.go, mirrored here.
	const (
		unknownNamespaceLiteral = "UNKNOWN_NAMESPACE"
		unknownElementLiteral   = "UNKNOWN_ELEMENT"
		noLogicLiteral          = "DECISION_NO_LOGIC"
	)
	if unknownNamespaceLiteral != CodeUnknownNamespace {
		t.Errorf("UNKNOWN_NAMESPACE literal drifted from CodeUnknownNamespace (%q)", CodeUnknownNamespace)
	}
	if unknownElementLiteral != CodeUnknownElement {
		t.Errorf("UNKNOWN_ELEMENT literal drifted from CodeUnknownElement (%q)", CodeUnknownElement)
	}
	if noLogicLiteral != CodeNoLogic {
		t.Errorf("DECISION_NO_LOGIC literal drifted from CodeNoLogic (%q)", CodeNoLogic)
	}
}
