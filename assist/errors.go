package assist

import (
	"errors"
	"fmt"
)

// ErrMaxSteps is returned by Agent.Reply when the tool-calling loop runs for
// MaxSteps turns without the model producing a final text answer. It bounds a
// hostile or looping model (golden rule 7); the partial Result still carries the
// steps taken.
var ErrMaxSteps = errors.New("assist: tool-calling loop exceeded the step budget")

// ErrNoToken indicates an LLM API token was required but none was configured
// (neither a server token nor a per-request bring-your-own-key).
var ErrNoToken = errors.New("assist: no API token configured")

// APIError is returned by a Provider when the LLM backend responds with a
// non-2xx status. It preserves the provider name, HTTP status and a short
// message so callers can surface a precise, machine-readable failure.
type APIError struct {
	// Provider is the backend that failed ("anthropic", "openai").
	Provider string
	// Status is the HTTP status code returned by the backend.
	Status int
	// Message is the backend's error message, or a truncated body.
	Message string
}

// Error implements error.
func (e *APIError) Error() string {
	return fmt.Sprintf("assist: %s API error (status %d): %s", e.Provider, e.Status, e.Message)
}
