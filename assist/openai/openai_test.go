package openai

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
	t.Run("maps request and parses content + tool_calls", func(t *testing.T) {
		var got chatRequest
		var hdr http.Header
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/chat/completions" || r.Method != http.MethodPost {
				t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			}
			hdr = r.Header.Clone()
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"hi","tool_calls":[{"id":"tc1","type":"function","function":{"name":"evaluate","arguments":"{\"a\":1}"}}]},"finish_reason":"tool_calls"}]}`)
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

		if hdr.Get("Authorization") != "Bearer sk-test" {
			t.Errorf("Authorization = %q", hdr.Get("Authorization"))
		}
		if got.Model != DefaultModel || got.MaxTokens != 256 {
			t.Errorf("request meta = %+v", got)
		}
		if len(got.Tools) != 1 || got.Tools[0].Type != "function" || got.Tools[0].Function.Name != "evaluate" {
			t.Errorf("tools = %+v", got.Tools)
		}
		// system + user + assistant(tool_calls) + tool = 4 messages.
		if len(got.Messages) != 4 {
			t.Fatalf("messages = %d, want 4: %+v", len(got.Messages), got.Messages)
		}
		if got.Messages[0].Role != "system" || got.Messages[0].Content != "be a DMN helper" {
			t.Errorf("system message = %+v", got.Messages[0])
		}
		if got.Messages[2].Role != "assistant" || len(got.Messages[2].ToolCalls) != 1 || got.Messages[2].ToolCalls[0].Function.Arguments != `{"a":1}` {
			t.Errorf("assistant turn = %+v", got.Messages[2])
		}
		if got.Messages[3].Role != "tool" || got.Messages[3].ToolCallID != "x" || got.Messages[3].Content != "result" {
			t.Errorf("tool turn = %+v", got.Messages[3])
		}

		if resp.Text != "hi" {
			t.Errorf("text = %q", resp.Text)
		}
		if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].ID != "tc1" || resp.ToolCalls[0].Name != "evaluate" {
			t.Fatalf("tool calls = %+v", resp.ToolCalls)
		}
		if string(resp.ToolCalls[0].Args) != `{"a":1}` {
			t.Errorf("args = %s", resp.ToolCalls[0].Args)
		}
		if resp.StopReason != "tool_calls" {
			t.Errorf("stop reason = %q", resp.StopReason)
		}
	})

	t.Run("non-2xx becomes APIError", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":{"type":"auth","message":"bad key"}}`)
		}))
		defer srv.Close()

		c := New("sk-test", WithBaseURL(srv.URL))
		_, err := c.Complete(context.Background(), assist.Request{Messages: []assist.Message{{Role: assist.RoleUser, Text: "hi"}}})
		var apiErr *assist.APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("err = %v, want APIError", err)
		}
		if apiErr.Status != http.StatusUnauthorized || apiErr.Message != "bad key" || apiErr.Provider != "openai" {
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

	if New("k").Name() != "openai" {
		t.Errorf("Name() = %q", New("k").Name())
	}
}
