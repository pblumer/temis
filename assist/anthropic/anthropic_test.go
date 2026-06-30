package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pblumer/temis/assist"
)

func TestComplete(t *testing.T) {
	t.Run("maps request and parses text + tool_use", func(t *testing.T) {
		var got messagesRequest
		var hdr http.Header
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/messages" || r.Method != http.MethodPost {
				t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			}
			hdr = r.Header.Clone()
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			_, _ = io.WriteString(w, `{"content":[{"type":"text","text":"hi"},{"type":"tool_use","id":"tu1","name":"evaluate","input":{"a":1}}],"stop_reason":"tool_use"}`)
		}))
		defer srv.Close()

		c := New("sk-test", WithBaseURL(srv.URL))
		resp, err := c.Complete(context.Background(), assist.Request{
			System:    "be a DMN helper",
			MaxTokens: 256,
			Tools:     []assist.Tool{{Name: "evaluate", Description: "run", Schema: json.RawMessage(`{"type":"object"}`)}},
			Messages: []assist.Message{
				{Role: assist.RoleUser, Text: "hello"},
				{Role: assist.RoleAssistant, Text: "ok", ToolCalls: []assist.ToolCall{{ID: "x", Name: "evaluate", Args: json.RawMessage(`{"a":1}`)}}},
				{Role: assist.RoleTool, ToolResults: []assist.ToolResult{{ID: "x", Content: "result"}}},
			},
		})
		if err != nil {
			t.Fatalf("Complete: %v", err)
		}

		// Auth + version headers.
		if hdr.Get("X-Api-Key") != "sk-test" {
			t.Errorf("X-Api-Key = %q", hdr.Get("X-Api-Key"))
		}
		if hdr.Get("Anthropic-Version") != apiVersion {
			t.Errorf("Anthropic-Version = %q", hdr.Get("Anthropic-Version"))
		}
		// Request mapping.
		if got.Model != DefaultModel || got.MaxTokens != 256 || got.System != "be a DMN helper" {
			t.Errorf("request meta = %+v", got)
		}
		if len(got.Tools) != 1 || got.Tools[0].Name != "evaluate" {
			t.Errorf("tools = %+v", got.Tools)
		}
		if len(got.Messages) != 3 {
			t.Fatalf("messages = %d, want 3", len(got.Messages))
		}
		if got.Messages[1].Role != "assistant" || got.Messages[1].Content[1].Type != "tool_use" {
			t.Errorf("assistant turn = %+v", got.Messages[1])
		}
		if got.Messages[2].Role != "user" || got.Messages[2].Content[0].Type != "tool_result" || got.Messages[2].Content[0].ToolUseID != "x" {
			t.Errorf("tool-result turn = %+v", got.Messages[2])
		}
		// Response parsing.
		if resp.Text != "hi" {
			t.Errorf("text = %q", resp.Text)
		}
		if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].ID != "tu1" || resp.ToolCalls[0].Name != "evaluate" {
			t.Fatalf("tool calls = %+v", resp.ToolCalls)
		}
		if string(resp.ToolCalls[0].Args) != `{"a":1}` {
			t.Errorf("args = %s", resp.ToolCalls[0].Args)
		}
		if resp.StopReason != "tool_use" {
			t.Errorf("stop reason = %q", resp.StopReason)
		}
	})

	t.Run("non-2xx becomes APIError", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"error":{"type":"rate_limit","message":"slow down"}}`)
		}))
		defer srv.Close()

		c := New("sk-test", WithBaseURL(srv.URL))
		_, err := c.Complete(context.Background(), assist.Request{Messages: []assist.Message{{Role: assist.RoleUser, Text: "hi"}}})
		var apiErr *assist.APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("err = %v, want APIError", err)
		}
		if apiErr.Status != http.StatusTooManyRequests || apiErr.Message != "slow down" || apiErr.Provider != "anthropic" {
			t.Errorf("APIError = %+v", apiErr)
		}
	})

	t.Run("missing key errors before any request", func(t *testing.T) {
		c := New("")
		_, err := c.Complete(context.Background(), assist.Request{})
		if !errors.Is(err, assist.ErrNoToken) {
			t.Fatalf("err = %v, want ErrNoToken", err)
		}
	})

	t.Run("WithModel overrides default", func(t *testing.T) {
		var got messagesRequest
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &got)
			_, _ = io.WriteString(w, `{"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn"}`)
		}))
		defer srv.Close()
		c := New("k", WithBaseURL(srv.URL), WithModel("claude-custom"))
		if _, err := c.Complete(context.Background(), assist.Request{Messages: []assist.Message{{Role: assist.RoleUser, Text: "hi"}}}); err != nil {
			t.Fatal(err)
		}
		if got.Model != "claude-custom" {
			t.Errorf("model = %q, want claude-custom", got.Model)
		}
	})

	if New("k").Name() != "anthropic" {
		t.Errorf("Name() = %q", New("k").Name())
	}
}
