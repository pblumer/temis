package dmn

import "fmt"

// EvalError is the typed error returned by Evaluate when an evaluation could not
// be carried out: the decision is not executable, a required input is missing,
// the context was cancelled, a resource limit was exhausted, or another runtime
// failure occurred. Code is one of the Code* constants and is the stable,
// machine-readable classification; callers should switch on Code, not parse
// Message.
//
// A spec-conformant FEEL null is NOT an EvalError. It is an ordinary Result with
// a nil output and, optionally, a non-error (warning/info) diagnostic in Diags.
type EvalError struct {
	Code       string // one of the Code* constants
	DecisionID string // the decision being evaluated, when known
	Message    string // human-readable detail; not stable, do not parse
	Err        error  // wrapped cause, when one exists
}

// Error renders the error as `dmn: <CODE>: decision "<id>": <message>`. The
// decision clause is omitted when DecisionID is empty.
func (e *EvalError) Error() string {
	if e.DecisionID == "" {
		return fmt.Sprintf("dmn: %s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("dmn: %s: decision %q: %s", e.Code, e.DecisionID, e.Message)
}

// Unwrap returns the wrapped cause so errors.Is/As can traverse it.
func (e *EvalError) Unwrap() error { return e.Err }
