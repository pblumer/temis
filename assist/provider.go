package assist

import (
	"context"
	"encoding/json"
)

// Role identifies who produced a message in a conversation.
type Role string

const (
	// RoleUser is a message from the human.
	RoleUser Role = "user"
	// RoleAssistant is a message from the model, possibly carrying tool calls.
	RoleAssistant Role = "assistant"
	// RoleTool carries the results of tool calls back to the model.
	RoleTool Role = "tool"
)

// Message is one turn in a conversation, in a provider-neutral form. Each
// Provider maps it to its own wire format:
//
//   - A user turn sets Role=RoleUser and Text.
//   - An assistant turn that calls tools sets Role=RoleAssistant, optional Text
//     and one or more ToolCalls.
//   - The reply to those calls sets Role=RoleTool and one ToolResult per call,
//     each tied back by ID.
type Message struct {
	// Role is who produced the message.
	Role Role `json:"role"`
	// Text is the message's natural-language content (may be empty on a
	// tool-only assistant turn or a tool-result turn).
	Text string `json:"text,omitempty"`
	// ToolCalls are the tools the assistant asked to run (assistant turns only).
	ToolCalls []ToolCall `json:"toolCalls,omitempty"`
	// ToolResults are the outcomes of earlier tool calls (RoleTool turns only).
	ToolResults []ToolResult `json:"toolResults,omitempty"`
}

// Tool is a function the model may call. Schema is the JSON Schema of the tool's
// parameters (an object schema), passed verbatim to the provider.
type Tool struct {
	// Name is the tool's identifier, as the model refers to it.
	Name string `json:"name"`
	// Description tells the model what the tool does and when to use it.
	Description string `json:"description"`
	// Schema is the JSON-Schema object describing the tool's parameters.
	Schema json.RawMessage `json:"schema"`
}

// ToolCall is the model's request to invoke a tool with JSON arguments.
type ToolCall struct {
	// ID correlates the call with its ToolResult (provider-assigned).
	ID string `json:"id"`
	// Name is the tool to invoke.
	Name string `json:"name"`
	// Args is the JSON arguments object the model supplied.
	Args json.RawMessage `json:"args"`
}

// ToolResult is the outcome of running a ToolCall, fed back to the model.
type ToolResult struct {
	// ID matches the originating ToolCall.ID.
	ID string `json:"id"`
	// Content is the tool's result, as text the model can read (often JSON).
	Content string `json:"content"`
	// IsError marks the call as failed, so the model can recover or report it.
	IsError bool `json:"isError,omitempty"`
}

// Request is one model turn: the system prompt, the conversation so far, the
// available tools and generation settings.
type Request struct {
	// System is the system prompt (provider-specific placement).
	System string
	// Messages is the conversation history, oldest first.
	Messages []Message
	// Tools is the catalog the model may call (may be empty).
	Tools []Tool
	// Model is the provider model id; empty uses the provider's default.
	Model string
	// MaxTokens caps the response length; non-positive uses the provider default.
	MaxTokens int
}

// Response is a provider's reply for one turn: free text, any tool calls the
// model wants run, and the raw stop reason for diagnostics.
type Response struct {
	// Text is the model's natural-language output for this turn (may be empty
	// when it only requested tools).
	Text string
	// ToolCalls are the tools the model asked to run before continuing.
	ToolCalls []ToolCall
	// StopReason is the provider's raw stop/finish reason.
	StopReason string
}

// Provider is one LLM backend. It performs a single, non-streaming model turn.
// Implementations must be safe for concurrent use.
type Provider interface {
	// Name identifies the provider ("anthropic", "openai").
	Name() string
	// Complete runs one model turn and returns its text and/or tool calls.
	Complete(ctx context.Context, req Request) (Response, error)
}
