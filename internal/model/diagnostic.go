package model

import "strconv"

// Severity classifies a Diagnostic.
type Severity int

// Diagnostic severities.
const (
	SeverityError Severity = iota
	SeverityWarning
	SeverityInfo
)

// String returns the lowercase severity name.
func (s Severity) String() string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	case SeverityInfo:
		return "info"
	default:
		return "unknown"
	}
}

// MarshalJSON renders the severity as its name.
func (s Severity) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote(s.String())), nil
}

// Diagnostic reports a non-fatal issue found while decoding or mapping a model,
// such as an unknown element or an unrecognised DMN version. Line and Col are
// not populated by the XML mapper (encoding/xml does not expose positions) and
// are reserved for the FEEL compiler's diagnostics.
type Diagnostic struct {
	Severity Severity
	// Code is the stable error-class string for this diagnostic, mirroring the
	// canonical Code* constants in package dmn. It is a string literal here (not
	// a reference to those constants) because internal/model must not import dmn:
	// dmn imports internal/model, so the dependency would be a cycle. The
	// drift-guard test in dmn/codes_test.go keeps the literals in sync.
	Code       string `json:",omitempty"`
	Message    string
	Source     string `json:",omitempty"`
	DecisionID string `json:",omitempty"`
}
