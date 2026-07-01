package flow

import "strings"

// Diagnostic is a single problem found while compiling, validating or evaluating
// a flow. Code is a stable, machine-readable class (one of the Code* constants);
// Message is human-readable and not stable. Step names the offending step when
// applicable.
type Diagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Step    string `json:"step,omitempty"`
}

// Diagnostics is a collection of flow diagnostics. Every flow diagnostic is an
// error (a structural or wiring fault), so HasErrors reports whether any exist.
type Diagnostics []Diagnostic

// HasErrors reports whether any diagnostic is present.
func (d Diagnostics) HasErrors() bool { return len(d) > 0 }

// Error renders the diagnostics as a single "; "-joined string.
func (d Diagnostics) Error() string {
	if len(d) == 0 {
		return ""
	}
	parts := make([]string, len(d))
	for i, x := range d {
		if x.Step != "" {
			parts[i] = x.Code + " [" + x.Step + "]: " + x.Message
		} else {
			parts[i] = x.Code + ": " + x.Message
		}
	}
	return strings.Join(parts, "; ")
}

// Stable diagnostic codes (SemVer surface; extended additively).
const (
	CodeMalformed       = "FLOW_MALFORMED"        // descriptor cannot be parsed
	CodeNoSteps         = "FLOW_NO_STEPS"         // flow has no steps
	CodeMissingField    = "FLOW_MISSING_FIELD"    // a step lacks id/model/decision
	CodeDuplicateStep   = "FLOW_DUPLICATE_STEP"   // two steps share an id
	CodeUnknownRef      = "FLOW_UNKNOWN_REF"      // a reference resolves to nothing
	CodeMappingInvalid  = "FLOW_MAPPING_INVALID"  // a FEEL mapping expression does not compile
	CodeCycle           = "FLOW_CYCLE"            // step references form a cycle
	CodeModelUnresolved = "FLOW_MODEL_UNRESOLVED" // Resolver could not supply a model
	CodeTargetNotFound  = "FLOW_TARGET_NOT_FOUND" // model has no such decision/service
	CodeUnknownInput    = "FLOW_UNKNOWN_INPUT"    // wiring targets an input the decision lacks
	CodeInputUnwired    = "FLOW_INPUT_UNWIRED"    // a required input is not wired
	CodeMaxSteps        = "FLOW_MAX_STEPS"        // step count exceeds the guard
)

// Error wraps flow Diagnostics as a Go error, so Evaluate/Validate failures can
// be inspected via errors.As.
type Error struct {
	Diagnostics Diagnostics
}

func (e *Error) Error() string { return "flow: " + e.Diagnostics.Error() }
