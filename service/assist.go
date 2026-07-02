package service

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pblumer/temis/assist"
	"github.com/pblumer/temis/assist/anthropic"
	"github.com/pblumer/temis/assist/openai"
)

// AssistConfig enables and configures the built-in modeling assistant (ADR-0024:
// POST /v1/chat). It is opt-in — the endpoint is dormant until a config is passed
// via WithAssist. The Token is the server-side default API key; with AllowBYOK a
// caller may instead send its own key in the X-LLM-Token header (it is used only
// for that request and never stored).
type AssistConfig struct {
	// Provider is the default LLM backend, "anthropic" or "openai". A per-request
	// "provider" field overrides it. Empty defaults to "anthropic".
	Provider string
	// Token is the server-side API key for the provider. May be empty when
	// AllowBYOK is set and callers bring their own key.
	Token string
	// Model overrides the provider's default model id (e.g. a specific Claude or
	// GPT model). A per-request "model" field overrides it; empty uses the
	// provider default.
	Model string
	// BaseURL overrides the provider's API root (for a proxy or a compatible
	// endpoint). Empty uses the provider default.
	BaseURL string
	// AllowBYOK lets a caller supply its own API key via the X-LLM-Token header,
	// taking precedence over Token for that request.
	AllowBYOK bool
	// System overrides the assistant's system prompt. Empty uses the built-in
	// DMN-modeling prompt.
	System string
	// MaxSteps bounds the tool-calling loop per request; 0 uses the assist
	// package default.
	MaxSteps int
	// HTTPClient is the HTTP client used for provider calls (for timeouts or
	// tests). Nil uses the default client.
	HTTPClient *http.Client
}

// WithAssist enables the modeling assistant at POST /v1/chat, backed by the given
// LLM provider configuration (ADR-0024). Without it the endpoint reports the
// assistant as disabled. The endpoint is gated by the same optional bearer token
// as the other /v1 routes.
func WithAssist(cfg AssistConfig) Option {
	return func(s *Server) {
		c := cfg
		s.assist = &c
	}
}

// handleChat runs one assistant turn: it takes the conversation so far, lets the
// configured LLM drive temis's tools (inspect/evaluate/build decisions) and
// returns the assistant's reply, the tool steps it took and the id of any model
// it produced (so the modeler can reload it). It is a 503 when the assistant is
// not enabled on this server.
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	cfg := s.assist
	if cfg == nil {
		writeProblem(w, http.StatusServiceUnavailable, "ASSIST_DISABLED",
			"the modeling assistant is not enabled on this server")
		return
	}

	var req chatRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if len(req.Messages) == 0 {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", "missing messages")
		return
	}

	provider := firstNonEmpty(req.Provider, cfg.Provider, "anthropic")
	model := firstNonEmpty(req.Model, cfg.Model)
	token := cfg.Token
	if cfg.AllowBYOK {
		if byok := strings.TrimSpace(r.Header.Get("X-LLM-Token")); byok != "" {
			token = byok
		}
	}
	if token == "" {
		writeProblem(w, http.StatusBadRequest, "ASSIST_NO_TOKEN",
			"no LLM API token configured; set one on the server or send X-LLM-Token")
		return
	}

	prov, err := s.buildProvider(provider, token, model, cfg.BaseURL, cfg.HTTPClient)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "ASSIST_BAD_PROVIDER", err.Error())
		return
	}

	exec := newAssistExecutor(s)
	ag := assist.New(prov, exec,
		assist.WithSystem(firstNonEmpty(cfg.System, defaultAssistSystem)),
		assist.WithModel(model),
		assist.WithMaxSteps(cfg.MaxSteps),
	)

	res, err := ag.Reply(r.Context(), toAssistMessages(req.Messages))
	if err != nil && !errors.Is(err, assist.ErrMaxSteps) {
		s.metrics.llmFailed.Add(1)
		var apiErr *assist.APIError
		if errors.As(err, &apiErr) {
			writeProblem(w, http.StatusBadGateway, "ASSIST_PROVIDER_ERROR", apiErr.Error())
			return
		}
		writeProblem(w, http.StatusInternalServerError, "ASSIST_FAILED", err.Error())
		return
	}
	// A reply came back (possibly after hitting the step limit): count it as a
	// successful provider call for the status endpoint (WP-111).
	s.metrics.llmOk.Add(1)
	s.metrics.llmLastOk.Store(time.Now().Unix())

	reply := res.Reply
	if errors.Is(err, assist.ErrMaxSteps) && reply == "" {
		reply = "Ich habe das Schritt-Limit erreicht, bevor ich fertig wurde. Bitte zerlege die Aufgabe in kleinere Schritte."
	}
	writeJSON(w, http.StatusOK, chatResponse{
		Reply:    reply,
		Steps:    res.Steps,
		ModelID:  exec.lastModel,
		Provider: prov.Name(),
	})
}

// buildProvider constructs the assist.Provider for a provider name, key, model
// and optional base URL/HTTP client.
func (s *Server) buildProvider(name, token, model, baseURL string, hc *http.Client) (assist.Provider, error) {
	switch name {
	case "anthropic":
		return anthropic.New(token,
			anthropic.WithModel(model), anthropic.WithBaseURL(baseURL), anthropic.WithHTTPClient(hc)), nil
	case "openai":
		return openai.New(token,
			openai.WithModel(model), openai.WithBaseURL(baseURL), openai.WithHTTPClient(hc)), nil
	default:
		return nil, fmt.Errorf("unknown LLM provider %q (want \"anthropic\" or \"openai\")", name)
	}
}

// --- chat DTOs ---

type chatMessage struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

type chatRequest struct {
	Messages []chatMessage `json:"messages"`
	Provider string        `json:"provider,omitempty"`
	Model    string        `json:"model,omitempty"`
}

type chatResponse struct {
	Reply    string        `json:"reply"`
	Steps    []assist.Step `json:"steps,omitempty"`
	ModelID  string        `json:"modelId,omitempty"`
	Provider string        `json:"provider"`
}

// toAssistMessages maps the client's plain role/text turns to assist messages.
// Only user and assistant text turns are carried; tool calls are internal to the
// server-side loop and are not replayed from the client.
func toAssistMessages(msgs []chatMessage) []assist.Message {
	out := make([]assist.Message, 0, len(msgs))
	for _, m := range msgs {
		role := assist.RoleUser
		if strings.EqualFold(m.Role, "assistant") {
			role = assist.RoleAssistant
		}
		out = append(out, assist.Message{Role: role, Text: m.Text})
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
