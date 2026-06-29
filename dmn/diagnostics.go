package dmn

import "github.com/pblumer/temis/internal/model"

// Severity classifies a Diagnostic.
type Severity int

// Diagnostic severities.
const (
	SevError Severity = iota
	SevWarning
	SevInfo
)

// String returns the lowercase severity name.
func (s Severity) String() string {
	switch s {
	case SevError:
		return "error"
	case SevWarning:
		return "warning"
	case SevInfo:
		return "info"
	default:
		return "unknown"
	}
}

// Diagnostic is a single problem found while compiling or evaluating a model. It
// is never a panic: user errors are reported, not fatal.
type Diagnostic struct {
	Severity Severity
	// Code is the stable, machine-readable error class, one of the Code*
	// constants (e.g. CodeFEELCompile). It names the failure class, never the
	// severity, and is part of the SemVer surface — callers may program against
	// it. Message is human-readable and NOT stable; do not parse it.
	Code       string
	Message    string
	DecisionID string
	Line, Col  int // source position; 0 when not applicable
}

// Diagnostics is a collection of diagnostics returned by Compile or Evaluate.
type Diagnostics []Diagnostic

// HasErrors reports whether any diagnostic has error severity.
func (d Diagnostics) HasErrors() bool {
	for _, diag := range d {
		if diag.Severity == SevError {
			return true
		}
	}
	return false
}

// fromModelDiagnostics maps internal model diagnostics into the public type.
func fromModelDiagnostics(in []model.Diagnostic) Diagnostics {
	if len(in) == 0 {
		return nil
	}
	out := make(Diagnostics, len(in))
	for i, d := range in {
		out[i] = Diagnostic{
			Severity:   fromModelSeverity(d.Severity),
			Code:       d.Code,
			Message:    d.Message,
			DecisionID: d.DecisionID,
		}
	}
	return out
}

func fromModelSeverity(s model.Severity) Severity {
	switch s {
	case model.SeverityError:
		return SevError
	case model.SeverityWarning:
		return SevWarning
	default:
		return SevInfo
	}
}
