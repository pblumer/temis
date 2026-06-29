// Package mcp exposes the Temis DMN engine to AI agents over the Model Context
// Protocol (MCP). It is a thin adapter that reaches the engine only through the
// public dmn package, never through internal/ (architecture D5/ADR-0005), and
// realises the first pillar of the Agent-First surface (ADR-0012, WP-50): an
// agent can discover and call temis as a native tool to delegate a rule-based
// decision and get a deterministic, reproducible answer back.
//
// The protocol is JSON-RPC 2.0 over the stdio transport, implemented with the
// standard library alone (no MCP SDK dependency — see ADR-0013). The server
// offers four tools: list_models, load_model, describe_decision and evaluate.
// Compiled models are cached in memory keyed by the SHA-256 of their XML, so
// re-loading the same document is idempotent and returns the same model id.
package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/pblumer/temis/dmn"
)

const (
	// serverName identifies this server to MCP clients.
	serverName = "temis-mcp"

	// defaultProtocolVersion is advertised when a client sends no protocol
	// version on initialize. When the client does send one the server echoes it,
	// since the tool surface here is version-stable.
	defaultProtocolVersion = "2024-11-05"
)

// Server exposes a dmn.Engine over MCP. It is safe for concurrent use: the
// engine is stateless and the model cache is guarded by a mutex. The zero value
// is not usable; construct one with NewServer.
type Server struct {
	engine  *dmn.Engine
	version string

	mu     sync.RWMutex
	models map[string]*storedModel
}

// Option configures a Server at construction time.
type Option func(*Server)

// WithVersion sets the server version reported to clients in the initialize
// handshake. An empty value is ignored.
func WithVersion(v string) Option {
	return func(s *Server) {
		if v != "" {
			s.version = v
		}
	}
}

// NewServer returns a Server backed by engine. If engine is nil a default engine
// is used.
func NewServer(engine *dmn.Engine, opts ...Option) *Server {
	if engine == nil {
		engine = dmn.New()
	}
	s := &Server{engine: engine, version: "dev", models: map[string]*storedModel{}}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// storedModel is a compiled model held in the cache together with its index and
// any diagnostics produced while compiling it.
type storedModel struct {
	id    string
	defs  *dmn.Definitions
	index dmn.ModelIndex
	diags dmn.Diagnostics
}

// --- initialize handshake ---

func (s *Server) handleInitialize(params json.RawMessage) (any, *rpcError) {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if len(params) > 0 {
		_ = json.Unmarshal(params, &p)
	}
	version := p.ProtocolVersion
	if version == "" {
		version = defaultProtocolVersion
	}
	return map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"serverInfo":      map[string]any{"name": serverName, "version": s.version},
	}, nil
}

// --- tool definitions ---

// toolSpec is an MCP tool descriptor: a name, a description and a JSON Schema
// for its arguments.
type toolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func obj(props map[string]any, required ...string) map[string]any {
	schema := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

// tools is the static tool catalogue advertised via tools/list.
var tools = []toolSpec{
	{
		Name: "list_models",
		Description: "List the DMN models currently loaded in this server's cache, " +
			"each with its evaluable decisions and input-data names.",
		InputSchema: obj(map[string]any{}),
	},
	{
		Name: "load_model",
		Description: "Compile a DMN 1.5 XML document and cache it, returning a stable " +
			"content-addressed modelId plus the decisions and inputs it declares. " +
			"Re-loading the same XML returns the same modelId (idempotent).",
		InputSchema: obj(map[string]any{
			"xml": str("The DMN 1.5 XML document to compile and cache."),
		}, "xml"),
	},
	{
		Name: "describe_decision",
		Description: "Describe a decision of a cached model: its name and the input-data " +
			"names the model declares. Use this to learn what to pass to evaluate. " +
			"(Per-decision input types arrive with WP-52.)",
		InputSchema: obj(map[string]any{
			"modelId":  str("The modelId returned by load_model."),
			"decision": str("The decision name or id to describe."),
		}, "modelId", "decision"),
	},
	{
		Name: "evaluate",
		Description: "Evaluate a decision and return its outputs deterministically. " +
			"Supply either modelId (for a cached model) or xml (compiled on the fly). " +
			"The result is reproducible: the same inputs always yield the same outputs. " +
			"Set explain=true to also get a trace of which rules matched and why — use " +
			"it to justify the decision, not just read it.",
		InputSchema: obj(map[string]any{
			"modelId":  str("A modelId from load_model. Provide this or xml."),
			"xml":      str("A DMN 1.5 XML document to compile and evaluate in one call. Provide this or modelId."),
			"decision": str("The decision name or id to evaluate."),
			"input": map[string]any{
				"type":        "object",
				"description": "The evaluation context: input-data name → value. Names the model does not reference are ignored; missing referenced names evaluate to null.",
			},
			"explain": map[string]any{
				"type":        "boolean",
				"description": "When true, include a decision trace (matched rules, satisfied/violated conditions, contributing outputs) so the decision can be justified.",
			},
		}, "decision"),
	},
}

// --- tool dispatch ---

func (s *Server) handleToolsCall(ctx context.Context, params json.RawMessage) (any, *rpcError) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{Code: codeInvalidParams, Message: "invalid params: " + err.Error()}
	}
	switch p.Name {
	case "list_models":
		return s.toolListModels()
	case "load_model":
		return s.toolLoadModel(ctx, p.Arguments)
	case "describe_decision":
		return s.toolDescribeDecision(p.Arguments)
	case "evaluate":
		return s.toolEvaluate(ctx, p.Arguments)
	default:
		return toolError("unknown tool: " + p.Name), nil
	}
}

func (s *Server) toolListModels() (any, *rpcError) {
	s.mu.RLock()
	summaries := make([]modelSummary, 0, len(s.models))
	for _, sm := range s.models {
		summaries = append(summaries, modelSummary{ModelID: sm.id, Decisions: sm.index.Decisions, Inputs: sm.index.Inputs})
	}
	s.mu.RUnlock()
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].ModelID < summaries[j].ModelID })
	return toolText(map[string]any{"models": summaries, "count": len(summaries)})
}

func (s *Server) toolLoadModel(ctx context.Context, raw json.RawMessage) (any, *rpcError) {
	var a struct {
		XML string `json:"xml"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return toolError("invalid arguments: " + err.Error()), nil
	}
	if a.XML == "" {
		return toolError("missing required argument: xml"), nil
	}
	sm, err := s.compileAndStore(ctx, []byte(a.XML))
	if err != nil {
		return toolError("could not compile model: " + err.Error()), nil
	}
	return toolText(modelResponse{
		ModelID:     sm.id,
		Decisions:   sm.index.Decisions,
		Inputs:      sm.index.Inputs,
		Diagnostics: toDiagnosticDTOs(sm.diags),
	})
}

func (s *Server) toolDescribeDecision(raw json.RawMessage) (any, *rpcError) {
	var a struct {
		ModelID  string `json:"modelId"`
		Decision string `json:"decision"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return toolError("invalid arguments: " + err.Error()), nil
	}
	if a.ModelID == "" {
		return toolError("missing required argument: modelId"), nil
	}
	sm, ok := s.lookup(a.ModelID)
	if !ok {
		return toolError("no model with id " + a.ModelID + "; load it first with load_model"), nil
	}
	if a.Decision == "" {
		return toolError("missing required argument: decision"), nil
	}
	dec, err := sm.defs.Decision(a.Decision)
	if err != nil {
		return toolError(err.Error()), nil
	}
	return toolText(map[string]any{
		"modelId":    sm.id,
		"decision":   dec.Name(),
		"decisionId": dec.ID(),
		"inputs":     sm.index.Inputs,
	})
}

func (s *Server) toolEvaluate(ctx context.Context, raw json.RawMessage) (any, *rpcError) {
	var a struct {
		ModelID  string         `json:"modelId"`
		XML      string         `json:"xml"`
		Decision string         `json:"decision"`
		Input    map[string]any `json:"input"`
		Explain  bool           `json:"explain"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return toolError("invalid arguments: " + err.Error()), nil
	}
	if a.Decision == "" {
		return toolError("missing required argument: decision"), nil
	}

	var defs *dmn.Definitions
	switch {
	case a.ModelID != "":
		sm, ok := s.lookup(a.ModelID)
		if !ok {
			return toolError("no model with id " + a.ModelID + "; load it first with load_model"), nil
		}
		defs = sm.defs
	case a.XML != "":
		sm, err := s.compileAndStore(ctx, []byte(a.XML))
		if err != nil {
			return toolError("could not compile model: " + err.Error()), nil
		}
		defs = sm.defs
	default:
		return toolError("provide either modelId or xml"), nil
	}

	dec, err := defs.Decision(a.Decision)
	if err != nil {
		return toolError(err.Error()), nil
	}
	var opts []dmn.EvalOption
	if a.Explain {
		opts = append(opts, dmn.WithTrace())
	}
	res, err := dec.Evaluate(ctx, dmn.Input(a.Input), opts...)
	if err != nil {
		return toolError("evaluation failed: " + err.Error()), nil
	}
	return toolText(evaluateResponse{
		Outputs:     res.Outputs,
		Decisions:   res.Decisions,
		Diagnostics: toDiagnosticDTOs(res.Diags),
		Trace:       res.Trace,
	})
}

// --- model store (content-addressed, mirrors the HTTP service) ---

func (s *Server) compileAndStore(ctx context.Context, xml []byte) (*storedModel, error) {
	id := modelID(xml)

	s.mu.RLock()
	if sm, ok := s.models[id]; ok {
		s.mu.RUnlock()
		return sm, nil
	}
	s.mu.RUnlock()

	defs, diags, err := s.engine.Compile(ctx, xml)
	if err != nil {
		return nil, err
	}
	sm := &storedModel{id: id, defs: defs, index: defs.Index(), diags: diags}

	s.mu.Lock()
	s.models[id] = sm
	s.mu.Unlock()
	return sm, nil
}

func (s *Server) lookup(id string) (*storedModel, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sm, ok := s.models[id]
	return sm, ok
}

// modelID is the cache key for an XML document: a hex SHA-256 with a "sha256:"
// prefix so the scheme is explicit, matching the HTTP service's model ids.
func modelID(xml []byte) string {
	sum := sha256.Sum256(xml)
	return fmt.Sprintf("sha256:%x", sum)
}

// --- tool-result helpers ---

// toolText wraps a value as a successful MCP tool result: a single text block
// holding its JSON encoding.
func toolText(v any) (any, *rpcError) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, &rpcError{Code: codeInternalError, Message: err.Error()}
	}
	return map[string]any{
		"content": []any{map[string]any{"type": "text", "text": string(b)}},
	}, nil
}

// toolError wraps a message as a failed MCP tool result (isError set), so the
// agent sees the failure as tool output rather than a transport error.
func toolError(msg string) any {
	return map[string]any{
		"content": []any{map[string]any{"type": "text", "text": msg}},
		"isError": true,
	}
}

// --- response DTOs (shape mirrors the HTTP service for cross-surface parity) ---

type modelResponse struct {
	ModelID     string          `json:"modelId"`
	Decisions   []string        `json:"decisions"`
	Inputs      []string        `json:"inputs"`
	Diagnostics []diagnosticDTO `json:"diagnostics,omitempty"`
}

type modelSummary struct {
	ModelID   string   `json:"modelId"`
	Decisions []string `json:"decisions"`
	Inputs    []string `json:"inputs"`
}

type evaluateResponse struct {
	Outputs     map[string]any  `json:"outputs"`
	Decisions   map[string]any  `json:"decisions"`
	Diagnostics []diagnosticDTO `json:"diagnostics,omitempty"`
	Trace       *dmn.Trace      `json:"trace,omitempty"`
}

type diagnosticDTO struct {
	Severity   string `json:"severity"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	DecisionID string `json:"decisionId,omitempty"`
	Line       int    `json:"line,omitempty"`
	Col        int    `json:"col,omitempty"`
}

func toDiagnosticDTOs(diags dmn.Diagnostics) []diagnosticDTO {
	if len(diags) == 0 {
		return nil
	}
	out := make([]diagnosticDTO, len(diags))
	for i, d := range diags {
		out[i] = diagnosticDTO{
			Severity:   d.Severity.String(),
			Code:       d.Code,
			Message:    d.Message,
			DecisionID: d.DecisionID,
			Line:       d.Line,
			Col:        d.Col,
		}
	}
	return out
}
