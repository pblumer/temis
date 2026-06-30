// Package anthropic implements assist.Provider over Anthropic's Messages API
// using only the standard library (ADR-0024, consistent with ADR-0014): no
// official SDK, no new dependency. It is a thin request/response client — one
// non-streaming model turn per Complete call — that maps assist's
// provider-neutral messages and tools to the Messages API wire format and back.
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/pblumer/temis/assist"
)

// DefaultBaseURL is Anthropic's API root. Override with WithBaseURL for a proxy
// or a test server.
const DefaultBaseURL = "https://api.anthropic.com"

// DefaultModel is the model used when a request leaves Request.Model empty. It
// is overridable per request (and via temisd's -llm-model flag).
const DefaultModel = "claude-sonnet-5"

// apiVersion is the Anthropic API version header value the Messages API expects.
const apiVersion = "2023-06-01"

// Client is an assist.Provider backed by Anthropic's Messages API.
type Client struct {
	apiKey  string
	baseURL string
	model   string
	http    *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the API root (for a proxy or a test server).
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

// New builds a Client authenticating with apiKey. The key may be a server-side
// token or a per-request bring-your-own-key (ADR-0024); construct one Client per
// distinct key.
func New(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:  apiKey,
		baseURL: DefaultBaseURL,
		model:   DefaultModel,
		http:    http.DefaultClient,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Name identifies the provider.
func (c *Client) Name() string { return "anthropic" }

// --- wire types (Messages API) ---

type messagesRequest struct {
	Model     string        `json:"model"`
	MaxTokens int           `json:"max_tokens"`
	System    string        `json:"system,omitempty"`
	Tools     []wireTool    `json:"tools,omitempty"`
	Messages  []wireMessage `json:"messages"`
}

type wireTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type wireMessage struct {
	Role    string      `json:"role"`
	Content []wireBlock `json:"content"`
}

// wireBlock is a content block. The shape depends on Type: text carries Text;
// tool_use carries ID/Name/Input; tool_result carries ToolUseID/Content/IsError.
type wireBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

type messagesResponse struct {
	Content    []wireBlock `json:"content"`
	StopReason string      `json:"stop_reason"`
	Error      *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// Complete runs one model turn over the Messages API.
func (c *Client) Complete(ctx context.Context, req assist.Request) (assist.Response, error) {
	if c.apiKey == "" {
		return assist.Response{}, assist.ErrNoToken
	}

	model := req.Model
	if model == "" {
		model = c.model
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = assist.DefaultMaxTokens
	}

	body := messagesRequest{
		Model:     model,
		MaxTokens: maxTokens,
		System:    req.System,
		Tools:     toWireTools(req.Tools),
		Messages:  toWireMessages(req.Messages),
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return assist.Response{}, fmt.Errorf("anthropic: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(buf))
	if err != nil {
		return assist.Response{}, fmt.Errorf("anthropic: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Api-Key", c.apiKey)
	httpReq.Header.Set("Anthropic-Version", apiVersion)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return assist.Response{}, fmt.Errorf("anthropic: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return assist.Response{}, fmt.Errorf("anthropic: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return assist.Response{}, &assist.APIError{Provider: "anthropic", Status: resp.StatusCode, Message: errorMessage(raw)}
	}

	var mr messagesResponse
	if err := json.Unmarshal(raw, &mr); err != nil {
		return assist.Response{}, fmt.Errorf("anthropic: decode response: %w", err)
	}
	if mr.Error != nil {
		return assist.Response{}, &assist.APIError{Provider: "anthropic", Status: resp.StatusCode, Message: mr.Error.Message}
	}

	out := assist.Response{StopReason: mr.StopReason}
	var text strings.Builder
	for _, b := range mr.Content {
		switch b.Type {
		case "text":
			text.WriteString(b.Text)
		case "tool_use":
			out.ToolCalls = append(out.ToolCalls, assist.ToolCall{ID: b.ID, Name: b.Name, Args: b.Input})
		}
	}
	out.Text = text.String()
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
		out[i] = wireTool{Name: t.Name, Description: t.Description, InputSchema: schema}
	}
	return out
}

// toWireMessages maps assist messages to Messages-API messages. Tool results are
// sent as a user message of tool_result blocks (the Messages API convention).
func toWireMessages(msgs []assist.Message) []wireMessage {
	out := make([]wireMessage, 0, len(msgs))
	for _, m := range msgs {
		switch m.Role {
		case assist.RoleUser:
			out = append(out, wireMessage{Role: "user", Content: []wireBlock{{Type: "text", Text: m.Text}}})
		case assist.RoleAssistant:
			var blocks []wireBlock
			if m.Text != "" {
				blocks = append(blocks, wireBlock{Type: "text", Text: m.Text})
			}
			for _, tc := range m.ToolCalls {
				input := tc.Args
				if len(input) == 0 {
					input = json.RawMessage(`{}`)
				}
				blocks = append(blocks, wireBlock{Type: "tool_use", ID: tc.ID, Name: tc.Name, Input: input})
			}
			out = append(out, wireMessage{Role: "assistant", Content: blocks})
		case assist.RoleTool:
			blocks := make([]wireBlock, 0, len(m.ToolResults))
			for _, tr := range m.ToolResults {
				blocks = append(blocks, wireBlock{Type: "tool_result", ToolUseID: tr.ID, Content: tr.Content, IsError: tr.IsError})
			}
			out = append(out, wireMessage{Role: "user", Content: blocks})
		}
	}
	return out
}

// errorMessage extracts the Anthropic error message from a non-2xx body, falling
// back to a truncated raw body.
func errorMessage(raw []byte) string {
	var e messagesResponse
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
