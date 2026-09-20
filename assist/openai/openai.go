// Package openai implements assist.Provider over OpenAI's Chat Completions API
// using only the standard library (ADR-0024, consistent with ADR-0014): no
// official SDK, no new dependency. It is a thin request/response client — one
// non-streaming model turn per Complete call — that maps assist's
// provider-neutral messages and tools to the Chat Completions wire format and
// back. It also works against any OpenAI-compatible endpoint via WithBaseURL.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pblumer/temis/assist"
)

// DefaultBaseURL is OpenAI's API root. Override with WithBaseURL for an
// Azure/OpenAI-compatible endpoint, a proxy or a test server.
const DefaultBaseURL = "https://api.openai.com"

// DefaultModel is the model used when a request leaves Request.Model empty. It
// is overridable per request (and via temisd's -llm-model flag).
const DefaultModel = "gpt-4o"

// Client is an assist.Provider backed by OpenAI's Chat Completions API.
type Client struct {
	apiKey  string
	baseURL string
	model   string
	http    *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the API root (for a compatible endpoint or a test server).
func WithBaseURL(u string) Option {
	return func(c *Client) {
		if u != "" {
			c.baseURL = strings.TrimRight(u, "/")
		}
	}
}

// WithModel sets the default model id used when a request does not specify one.
func WithModel(m string) Option {
	return func(c *Client) {
		if m != "" {
			c.model = m
		}
	}
}

// WithHTTPClient sets the HTTP client used for requests (for timeouts or tests).
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			c.http = h
		}
	}
}

// DefaultTimeout bounds a single provider request so a hung LLM endpoint cannot
// block the calling handler goroutine indefinitely (audit finding H4).
const DefaultTimeout = 120 * time.Second

// New builds a Client authenticating with apiKey. The key may be a server-side
// token or a per-request bring-your-own-key (ADR-0024); construct one Client per
// distinct key.
func New(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:  apiKey,
		baseURL: DefaultBaseURL,
		model:   DefaultModel,
		// A dedicated client with a deadline, not http.DefaultClient (no timeout):
		// a stalled provider must not hang the calling handler forever (H4).
		http: &http.Client{Timeout: DefaultTimeout},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Name identifies the provider.
func (c *Client) Name() string { return "openai" }

// --- wire types (Chat Completions) ---

type chatRequest struct {
	Model     string        `json:"model"`
	MaxTokens int           `json:"max_tokens,omitempty"`
	Messages  []wireMessage `json:"messages"`
	Tools     []wireTool    `json:"tools,omitempty"`
}

type wireTool struct {
	Type     string        `json:"type"`
	Function wireFunctionT `json:"function"`
}

type wireFunctionT struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

type wireMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type wireToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function wireFunction `json:"function"`
}

// wireFunction carries the called function's name and its arguments as a JSON
// string (the Chat Completions convention), not a JSON object.
type wireFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content   string         `json:"content"`
			ToolCalls []wireToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// Complete runs one model turn over the Chat Completions API.
func (c *Client) Complete(ctx context.Context, req assist.Request) (assist.Response, error) {
	if c.apiKey == "" {
		return assist.Response{}, assist.ErrNoToken
	}

	model := req.Model
	if model == "" {
		model = c.model
	}

	body := chatRequest{
		Model:     model,
		MaxTokens: req.MaxTokens,
		Messages:  toWireMessages(req.System, req.Messages),
		Tools:     toWireTools(req.Tools),
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return assist.Response{}, fmt.Errorf("openai: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return assist.Response{}, fmt.Errorf("openai: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return assist.Response{}, fmt.Errorf("openai: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return assist.Response{}, fmt.Errorf("openai: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return assist.Response{}, &assist.APIError{Provider: "openai", Status: resp.StatusCode, Message: errorMessage(raw)}
	}

	var cr chatResponse
	if err := json.Unmarshal(raw, &cr); err != nil {
		return assist.Response{}, fmt.Errorf("openai: decode response: %w", err)
	}
	if cr.Error != nil {
		return assist.Response{}, &assist.APIError{Provider: "openai", Status: resp.StatusCode, Message: cr.Error.Message}
	}
	if len(cr.Choices) == 0 {
		return assist.Response{}, fmt.Errorf("openai: response had no choices")
	}

	choice := cr.Choices[0]
	out := assist.Response{Text: choice.Message.Content, StopReason: choice.FinishReason}
	for _, tc := range choice.Message.ToolCalls {
		args := json.RawMessage(tc.Function.Arguments)
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		out.ToolCalls = append(out.ToolCalls, assist.ToolCall{ID: tc.ID, Name: tc.Function.Name, Args: args})
	}
	return out, nil
}

func toWireTools(tools []assist.Tool) []wireTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]wireTool, len(tools))
	for i, t := range tools {
		schema := t.Schema
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object"}`)
		}
		out[i] = wireTool{Type: "function", Function: wireFunctionT{Name: t.Name, Description: t.Description, Parameters: schema}}
	}
	return out
}

// toWireMessages maps assist messages to Chat Completions messages. The system
// prompt becomes a leading system message; each ToolResult becomes its own
// role:"tool" message (the Chat Completions convention).
func toWireMessages(system string, msgs []assist.Message) []wireMessage {
	out := make([]wireMessage, 0, len(msgs)+1)
	if system != "" {
		out = append(out, wireMessage{Role: "system", Content: system})
	}
	for _, m := range msgs {
		switch m.Role {
		case assist.RoleUser:
			out = append(out, wireMessage{Role: "user", Content: m.Text})
		case assist.RoleAssistant:
			wm := wireMessage{Role: "assistant", Content: m.Text}
			for _, tc := range m.ToolCalls {
				args := string(tc.Args)
				if args == "" {
					args = "{}"
				}
				wm.ToolCalls = append(wm.ToolCalls, wireToolCall{
					ID:       tc.ID,
					Type:     "function",
					Function: wireFunction{Name: tc.Name, Arguments: args},
				})
			}
			out = append(out, wm)
		case assist.RoleTool:
			for _, tr := range m.ToolResults {
				out = append(out, wireMessage{Role: "tool", ToolCallID: tr.ID, Content: tr.Content})
			}
		}
	}
	return out
}

// errorMessage extracts the OpenAI error message from a non-2xx body, falling
// back to a truncated raw body.
func errorMessage(raw []byte) string {
	var e chatResponse
	if json.Unmarshal(raw, &e) == nil && e.Error != nil && e.Error.Message != "" {
		return e.Error.Message
	}
	return truncate(string(raw), 300)
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
