package dmn

// Diagnostic and EvalError codes. Each constant names a stable error *class* —
// never a severity. The same class can surface at different severities (a
// missing-logic decision is a warning, a non-executable decision reached by
// Evaluate is an error); the code stays the same so callers can program against
// it regardless of how it is reported.
//
// Stability: these string values are part of the public SemVer surface
// (docs/40-api-contract.md §1.4). They may only be extended additively. Renaming
// or removing a code, or repurposing its value, is a breaking change. Callers may
// program against Code; they must not rely on Message, which is human-readable
// and not stable.
const (
	// CodeXMLMalformed marks a document that could not be decoded at all. It is
	// always returned as an error from Compile, never as a diagnostic.
	CodeXMLMalformed = "XML_MALFORMED"

	// CodeUnknownNamespace marks a document whose DMN namespace is not
	// recognised; it is decoded leniently. Severity: warning.
	CodeUnknownNamespace = "UNKNOWN_NAMESPACE"

	// CodeUnknownElement marks an XML element the mapper did not understand and
	// ignored. Severity: warning.
	CodeUnknownElement = "UNKNOWN_ELEMENT"

	// CodeNoLogic marks a decision that carries neither a literal expression nor
	// a decision table, so it has no executable logic. Severity: warning.
	CodeNoLogic = "DECISION_NO_LOGIC"

	// CodeFEELCompile marks a decision whose logic failed to compile. It is
	// reported as an error-severity diagnostic from Compile; the decision is
	// present in the model but not executable.
	CodeFEELCompile = "FEEL_COMPILE_ERROR"

	// CodeNotExecutable marks an Evaluate call on a decision that did not compile
	// to executable logic. Returned as an error from Evaluate.
	CodeNotExecutable = "DECISION_NOT_EXECUTABLE"

	// CodeMissingInput marks an Evaluate call missing a required input data value
	// the model references. Returned as an error from Evaluate.
	CodeMissingInput = "MISSING_REQUIRED_INPUT"

	// CodeLimitExceeded marks a resource limit being exhausted during evaluation
	// (ADR-0008). Returned as an error from Evaluate.
	CodeLimitExceeded = "LIMIT_EXCEEDED"

	// CodeUniqueMultiple marks a UNIQUE hit-policy decision table matching more
	// than one rule. Returned as an error from Evaluate.
	CodeUniqueMultiple = "UNIQUE_MULTIPLE_MATCH"

	// CodeRuntime is an honest placeholder for runtime failures that are not yet
	// exposed as a typed cause and so cannot be reliably classified at the API
	// edge (e.g. a resource-limit breach is today a bare error indistinguishable
	// from other runtime failures). It is additive: narrowing a specific runtime
	// failure to a more precise code later is a behavioural refinement, not a
	// removal of this code.
	CodeRuntime = "RUNTIME_ERROR"
)
