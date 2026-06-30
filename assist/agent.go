package assist

import (
	"context"
	"encoding/json"
	"errors"
)

// Default agent settings, used when an Option leaves a field unset.
const (
	// DefaultMaxSteps bounds how many provider turns a single Reply may take
	// before giving up (golden rule 7: bound a looping model).
	DefaultMaxSteps = 8
	// DefaultMaxTokens caps each provider turn's response length when the caller
	// does not set one.
	DefaultMaxTokens = 1024
)

// Executor is the tool surface the assistant may drive. It is implemented by the
// caller (the service) over temis's real operations, so this package stays free
// of service- and internal-package concerns (the same boundary discipline as
// vcs.Reader). Tools returns the catalog exposed to the model; Execute runs one
// named tool with the model's JSON arguments and returns a text (typically JSON)
// result. A returned error is reported back to the model as a failed tool result
// so it can recover, not propagated as a fatal error.
type Executor interface {
	// Tools returns the tool catalog exposed to the model.
	Tools() []Tool
	// Execute runs the named tool with JSON arguments and returns its result.
	Execute(ctx context.Context, name string, args json.RawMessage) (string, error)
}

// Agent runs a tool-calling loop over a Provider and an Executor: it asks the
// model, runs any tools it requests, feeds the results back and repeats until
// the model answers in plain text or the step budget is exhausted.
type Agent struct {
	provider  Provider
	exec      Executor
	system    string
	model     string
	maxTokens int
	maxSteps  int
}

// Option configures an Agent.
type Option func(*Agent)

// WithSystem sets the system prompt sent on every provider turn.
func WithSystem(prompt string) Option { return func(a *Agent) { a.system = prompt } }

// WithModel overrides the provider's default model id.
func WithModel(model string) Option { return func(a *Agent) { a.model = model } }

// WithMaxTokens caps each provider turn's response length.
func WithMaxTokens(n int) Option {
	return func(a *Agent) {
		if n > 0 {
			a.maxTokens = n
		}
	}
}

// WithMaxSteps bounds how many provider turns a single Reply may take.
func WithMaxSteps(n int) Option {
	return func(a *Agent) {
		if n > 0 {
			a.maxSteps = n
		}
	}
}

// New builds an Agent over the given provider and tool executor. A nil executor
// is allowed: the agent then runs as a plain chat with no tools.
func New(p Provider, e Executor, opts ...Option) *Agent {
	a := &Agent{
		provider:  p,
		exec:      e,
		maxTokens: DefaultMaxTokens,
		maxSteps:  DefaultMaxSteps,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Step records one tool invocation the agent performed, for the UI to show what
// the assistant did on the way to its answer.
type Step struct {
	// Tool is the tool name that was called.
	Tool string `json:"tool"`
	// Args is the JSON arguments the model supplied.
	Args json.RawMessage `json:"args,omitempty"`
	// Result is the tool's result text (or the error message when Error).
	Result string `json:"result,omitempty"`
	// Error reports whether the tool call failed.
	Error bool `json:"error,omitempty"`
}

// Result is the outcome of a Reply: the model's final text plus the trace of
// tool steps it took to get there.
type Result struct {
	// Reply is the assistant's final natural-language answer.
	Reply string `json:"reply"`
	// Steps is the ordered trace of tool calls made during the turn.
	Steps []Step `json:"steps,omitempty"`
}

// Reply continues the conversation in history and returns the assistant's final
// answer. It drives the tool-calling loop: each provider turn that requests
// tools has them executed and their results appended, until the model answers in
// plain text. If the loop runs MaxSteps turns without a final answer it returns
// the partial Result with ErrMaxSteps.
func (a *Agent) Reply(ctx context.Context, history []Message) (Result, error) {
	msgs := append([]Message(nil), history...)
	var tools []Tool
	if a.exec != nil {
		tools = a.exec.Tools()
	}

	var res Result
	for step := 0; step < a.maxSteps; step++ {
		resp, err := a.provider.Complete(ctx, Request{
			System:    a.system,
			Messages:  msgs,
			Tools:     tools,
			Model:     a.model,
			MaxTokens: a.maxTokens,
		})
		if err != nil {
			return res, err
		}

		// A turn with no tool calls is the final answer.
		if len(resp.ToolCalls) == 0 {
			res.Reply = resp.Text
			return res, nil
		}

		// Record the assistant's tool-call turn, then run each tool and feed the
		// results back as one RoleTool message.
		msgs = append(msgs, Message{Role: RoleAssistant, Text: resp.Text, ToolCalls: resp.ToolCalls})
		results := make([]ToolResult, 0, len(resp.ToolCalls))
		for _, tc := range resp.ToolCalls {
			st := Step{Tool: tc.Name, Args: tc.Args}
			tr := ToolResult{ID: tc.ID}
			out, err := a.execute(ctx, tc)
			if err != nil {
				tr.Content, tr.IsError = err.Error(), true
				st.Result, st.Error = err.Error(), true
			} else {
				tr.Content, st.Result = out, out
			}
			results = append(results, tr)
			res.Steps = append(res.Steps, st)
		}
		msgs = append(msgs, Message{Role: RoleTool, ToolResults: results})
	}
	return res, ErrMaxSteps
}

// execute runs one tool call, guarding against a nil executor (which should not
// happen once the model is offered tools, but keeps the loop total).
func (a *Agent) execute(ctx context.Context, tc ToolCall) (string, error) {
	if a.exec == nil {
		return "", errors.New("assist: tool call without an executor")
	}
	return a.exec.Execute(ctx, tc.Name, tc.Args)
}
