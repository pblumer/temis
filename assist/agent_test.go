package assist

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// fakeProvider replays a fixed script of responses, one per Complete call, and
// records the requests it received so tests can assert on the loop's behaviour.
type fakeProvider struct {
	script []Response
	calls  []Request
	err    error
}

func (f *fakeProvider) Name() string { return "fake" }

func (f *fakeProvider) Complete(_ context.Context, req Request) (Response, error) {
	f.calls = append(f.calls, req)
	if f.err != nil {
		return Response{}, f.err
	}
	if len(f.calls) > len(f.script) {
		return Response{Text: "(out of script)"}, nil
	}
	return f.script[len(f.calls)-1], nil
}

// fakeExec is a tiny executor exposing one "echo" tool that returns its args, or
// fails when asked to.
type fakeExec struct {
	fail bool
}

func (e fakeExec) Tools() []Tool {
	return []Tool{{Name: "echo", Description: "echo args", Schema: json.RawMessage(`{"type":"object"}`)}}
}

func (e fakeExec) Execute(_ context.Context, name string, args json.RawMessage) (string, error) {
	if e.fail {
		return "", errors.New("boom")
	}
	return name + ":" + string(args), nil
}

func TestAgentReply(t *testing.T) {
	t.Run("plain text answer ends immediately", func(t *testing.T) {
		p := &fakeProvider{script: []Response{{Text: "hello"}}}
		a := New(p, fakeExec{}, WithSystem("be helpful"))
		res, err := a.Reply(context.Background(), []Message{{Role: RoleUser, Text: "hi"}})
		if err != nil {
			t.Fatalf("Reply: %v", err)
		}
		if res.Reply != "hello" {
			t.Errorf("reply = %q, want %q", res.Reply, "hello")
		}
		if len(res.Steps) != 0 {
			t.Errorf("steps = %d, want 0", len(res.Steps))
		}
		if len(p.calls) != 1 {
			t.Fatalf("provider calls = %d, want 1", len(p.calls))
		}
		if p.calls[0].System != "be helpful" {
			t.Errorf("system = %q, want %q", p.calls[0].System, "be helpful")
		}
		if len(p.calls[0].Tools) != 1 {
			t.Errorf("tools offered = %d, want 1", len(p.calls[0].Tools))
		}
	})

	t.Run("runs a tool then answers", func(t *testing.T) {
		p := &fakeProvider{script: []Response{
			{ToolCalls: []ToolCall{{ID: "c1", Name: "echo", Args: json.RawMessage(`{"x":1}`)}}},
			{Text: "done"},
		}}
		a := New(p, fakeExec{})
		res, err := a.Reply(context.Background(), []Message{{Role: RoleUser, Text: "go"}})
		if err != nil {
			t.Fatalf("Reply: %v", err)
		}
		if res.Reply != "done" {
			t.Errorf("reply = %q, want %q", res.Reply, "done")
		}
		if len(res.Steps) != 1 || res.Steps[0].Tool != "echo" || res.Steps[0].Error {
			t.Fatalf("steps = %+v, want one successful echo", res.Steps)
		}
		if want := `echo:{"x":1}`; res.Steps[0].Result != want {
			t.Errorf("result = %q, want %q", res.Steps[0].Result, want)
		}
		// Second turn must carry the assistant tool-call turn and the tool result.
		if len(p.calls) != 2 {
			t.Fatalf("provider calls = %d, want 2", len(p.calls))
		}
		last := p.calls[1].Messages
		if len(last) != 3 {
			t.Fatalf("second-turn messages = %d, want 3 (user, assistant, tool)", len(last))
		}
		if last[1].Role != RoleAssistant || len(last[1].ToolCalls) != 1 {
			t.Errorf("expected assistant tool-call turn, got %+v", last[1])
		}
		if last[2].Role != RoleTool || len(last[2].ToolResults) != 1 || last[2].ToolResults[0].ID != "c1" {
			t.Errorf("expected tool-result turn tied to c1, got %+v", last[2])
		}
	})

	t.Run("tool failure is fed back, not fatal", func(t *testing.T) {
		p := &fakeProvider{script: []Response{
			{ToolCalls: []ToolCall{{ID: "c1", Name: "echo", Args: json.RawMessage(`{}`)}}},
			{Text: "recovered"},
		}}
		a := New(p, fakeExec{fail: true})
		res, err := a.Reply(context.Background(), []Message{{Role: RoleUser, Text: "go"}})
		if err != nil {
			t.Fatalf("Reply: %v", err)
		}
		if res.Reply != "recovered" {
			t.Errorf("reply = %q, want %q", res.Reply, "recovered")
		}
		if len(res.Steps) != 1 || !res.Steps[0].Error {
			t.Fatalf("expected one failed step, got %+v", res.Steps)
		}
		tr := p.calls[1].Messages[2].ToolResults[0]
		if !tr.IsError || tr.Content != "boom" {
			t.Errorf("tool result = %+v, want error 'boom'", tr)
		}
	})

	t.Run("step budget is bounded", func(t *testing.T) {
		// A model that always asks for a tool must not loop forever.
		loop := Response{ToolCalls: []ToolCall{{ID: "c", Name: "echo", Args: json.RawMessage(`{}`)}}}
		p := &fakeProvider{script: []Response{loop, loop, loop, loop, loop}}
		a := New(p, fakeExec{}, WithMaxSteps(3))
		res, err := a.Reply(context.Background(), []Message{{Role: RoleUser, Text: "go"}})
		if !errors.Is(err, ErrMaxSteps) {
			t.Fatalf("err = %v, want ErrMaxSteps", err)
		}
		if len(res.Steps) != 3 {
			t.Errorf("steps = %d, want 3 (the budget)", len(res.Steps))
		}
		if len(p.calls) != 3 {
			t.Errorf("provider calls = %d, want 3", len(p.calls))
		}
	})

	t.Run("provider error propagates", func(t *testing.T) {
		p := &fakeProvider{err: errors.New("network down")}
		a := New(p, fakeExec{})
		_, err := a.Reply(context.Background(), []Message{{Role: RoleUser, Text: "go"}})
		if err == nil || err.Error() != "network down" {
			t.Fatalf("err = %v, want network down", err)
		}
	})

	t.Run("nil executor runs as plain chat", func(t *testing.T) {
		p := &fakeProvider{script: []Response{{Text: "no tools"}}}
		a := New(p, nil)
		res, err := a.Reply(context.Background(), []Message{{Role: RoleUser, Text: "hi"}})
		if err != nil {
			t.Fatalf("Reply: %v", err)
		}
		if res.Reply != "no tools" {
			t.Errorf("reply = %q", res.Reply)
		}
		if len(p.calls[0].Tools) != 0 {
			t.Errorf("tools offered = %d, want 0", len(p.calls[0].Tools))
		}
	})
}

func TestAPIError(t *testing.T) {
	e := &APIError{Provider: "openai", Status: 429, Message: "rate limited"}
	if got, want := e.Error(), "assist: openai API error (status 429): rate limited"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
