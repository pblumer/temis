// Package mcp exposes the Temis DMN engine to AI agents over the Model Context
// Protocol (MCP). It is a thin adapter that reaches the engine only through the
// public dmn package, never through internal/ (architecture D5/ADR-0005), and
// realises the first pillar of the Agent-First surface (ADR-0013, WP-50): an
// agent can discover and call temis as a native tool to delegate a rule-based
// decision and get a deterministic, reproducible answer back.
//
// The protocol is JSON-RPC 2.0 over the stdio transport, implemented with the
// standard library alone (no MCP SDK dependency — see ADR-0014). The server
// offers four tools: list_models, load_model, describe_decision and evaluate.
// Compiled models are cached in memory keyed by the SHA-256 of their XML, so
// re-loading the same document is idempotent and returns the same model id.
package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/pblumer/temis/dmn"
	"github.com/pblumer/temis/flow"
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
	// store holds the compiled models the tools operate on. It defaults to an
	// in-process cache (memStore), but WithStore can replace it with a cache
	// shared with another surface (e.g. the HTTP service), so models loaded over
	// MCP and over HTTP are visible to both — one process, one address space.
	store   Store
	version string

	// flows holds decision-flow descriptors registered over the load_flow tool
	// (WP-92, ADR-0026), keyed by content hash. A flow resolves its model
	// references through store, so a model loaded over any surface is reachable.
	// It defaults to an in-process catalog, but WithFlowStore can replace it with
	// one shared with another surface (the HTTP service), so a flow registered over
	// MCP is visible in GET /v1/flows (and the modeler's flow list) and vice versa.
	flows FlowStore
	// token, when non-empty, is the deprecated single bearer token required on the
	// HTTP transport (HTTPHandler / WithHTTPToken). It grants every tool (admin).
	// It does not apply to the stdio transport, which is a trusted local subprocess.
	token string

	// auth, when set via WithAuth, is the host's scoped keystore gate (ADR-0028).
	// Each tool maps to a scope (toolScopes); the host verifies the caller's
	// kid.secret key against it. It supersedes token when both are set. Nil (and no
	// token) leaves the HTTP endpoint open.
	auth Auth

	// gitBaseURL overrides the GitHub REST API root for the git_* tools (default
	// https://api.github.com); set via WithGitHubBaseURL for GitHub Enterprise or
	// tests. The git-provider token is supplied per tool call (gitToken arg),
	// never stored on the server (WP-73, auth model A).
	gitBaseURL string
}

// ModelInfo summarises a cached model for list_models: its content-addressed id
// and the names of its evaluable decisions and input data.
type ModelInfo struct {
	ID        string
	Decisions []string
	Inputs    []string
}

// Store is the model cache the MCP tools operate on. Splitting it out lets the
// MCP server share one cache with another surface (the HTTP service) so a model
// loaded over either is visible to both. Implementations must be safe for
// concurrent use. Model ids are the "sha256:"-prefixed hash of the XML, so the
// same document always lands under the same id across surfaces.
type Store interface {
	// Compile compiles and caches xml, returning its content-addressed id, the
	// compiled definitions, its index and any compile diagnostics. Re-compiling
	// the same document returns the same id (idempotent).
	Compile(ctx context.Context, xml []byte) (id string, defs *dmn.Definitions, index dmn.ModelIndex, diags dmn.Diagnostics, err error)
	// Lookup returns the cached model for id, or ok=false when it is not cached.
	Lookup(id string) (defs *dmn.Definitions, index dmn.ModelIndex, ok bool)
	// List summarises every cached model, in any order (the caller sorts).
	List() []ModelInfo
}

// FlowStore is the decision-flow catalog the MCP flow tools operate on. Like
// Store for models, splitting it out lets the MCP server share one catalog with
// another surface (the HTTP service) so a flow registered over either — MCP
// load_flow or the /v1 API — is visible to both, including the modeler's flow
// list. Flow ids are the "sha256:"-prefixed hash of the descriptor bytes, so the
// same descriptor always lands under the same id across surfaces. Implementations
// must be safe for concurrent use.
type FlowStore interface {
	// Put registers a compiled flow under id together with the descriptor bytes it
	// was compiled from. Re-registering the same id overwrites (idempotent for a
	// content-addressed id).
	Put(ctx context.Context, id string, f *flow.Flow, desc []byte)
	// Get returns the compiled flow and the descriptor bytes for id, or ok=false
	// when no flow is registered under it.
	Get(id string) (f *flow.Flow, desc []byte, ok bool)
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

// WithHTTPToken requires callers of the HTTP transport to present
// "Authorization: Bearer <token>". An empty token leaves the HTTP endpoint open.
// It has no effect on the stdio transport. Deprecated in favour of WithAuth for
// scoped keys; a matching token grants every tool (admin).
func WithHTTPToken(token string) Option {
	return func(s *Server) { s.token = token }
}

// WithAuth gates the HTTP transport with the host's scoped keystore (ADR-0028).
// The mcp package maps each tool to a scope (toolScopes) and defers the verdict
// to auth. A nil Auth is ignored, keeping the endpoint open (or token-gated). It
// supersedes WithHTTPToken. It has no effect on the stdio transport.
func WithAuth(auth Auth) Option {
	return func(s *Server) {
		if auth != nil {
			s.auth = auth
		}
	}
}

// Auth authorizes MCP tool calls by scope. The host (temisd) implements it over
// its keystore; the mcp package supplies the scope string for each tool and the
// bearer credential from the request, and defers the verdict. Keeping the scope
// vocabulary on the host side avoids the mcp package importing the service.
type Auth interface {
	// Authorize verifies bearer for the given scope. An empty scope means "any
	// authenticated key" (discovery messages when enforcement is on).
	Authorize(bearer, scope string) AuthResult
}

// AuthResult is the verdict of an Auth check.
type AuthResult int

// The possible Auth verdicts.
const (
	AuthAllowed         AuthResult = iota // proceed
	AuthUnauthenticated                   // 401: missing/invalid/expired/revoked key
	AuthForbidden                         // 403: valid key, missing scope
)

// toolScopes maps each tool to its required scope (ADR-0028 §2). Strings match
// the service's Scope constants. A tool absent here (e.g. an unknown name) needs
// only authentication, then dispatch reports the unknown tool.
var toolScopes = map[string]string{
	"list_models":       "models:read",
	"load_model":        "models:read",
	"describe_decision": "models:read",
	"evaluate":          "evaluate",
	"load_flow":         "flow",
	"describe_flow":     "flow",
	"evaluate_flow":     "flow",
	"git_list_models":   "git",
	"git_load_model":    "git",
	"git_propose":       "git",
	"git_list_flows":    "git",
	"git_load_flow":     "git",
}

// WithStore backs the server with store instead of its default in-process cache,
// so the MCP tools share that store. Used to co-locate the MCP endpoint in the
// HTTP service process on one shared cache (same address space). A nil store is
// ignored, keeping the default. When set, the engine passed to NewServer is
// unused — the store owns compilation.
func WithStore(store Store) Option {
	return func(s *Server) {
		if store != nil {
			s.store = store
		}
	}
}

// WithFlowStore backs the server's flow catalog with store instead of its default
// in-process one, so MCP load_flow shares that catalog with another surface. Used
// to co-locate the MCP endpoint in the HTTP service process on one shared catalog:
// a flow registered over MCP then appears in GET /v1/flows (and the modeler's flow
// list) and vice versa. A nil store is ignored, keeping the default.
func WithFlowStore(store FlowStore) Option {
	return func(s *Server) {
		if store != nil {
			s.flows = store
		}
	}
}

// NewServer returns a Server backed by engine. If engine is nil a default engine
// is used. Unless WithStore overrides it, the server keeps its own in-process
// model cache built on engine.
func NewServer(engine *dmn.Engine, opts ...Option) *Server {
	if engine == nil {
		engine = dmn.New()
	}
	s := &Server{store: newMemStore(engine), version: "dev", flows: newFlowStore()}
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
		Description: "Describe a decision of a cached model: its name and the typed inputs " +
			"it expects (name, FEEL type, required). Use this to learn exactly what to " +
			"pass to evaluate.",
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
			"strict": map[string]any{
				"type":        "boolean",
				"description": "When true, validate the input against the decision's typed schema first and fail with the precise problems (wrong type, unknown or missing input) instead of silently coercing it.",
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
	case "load_flow":
		return s.toolLoadFlow(ctx, p.Arguments)
	case "describe_flow":
		return s.toolDescribeFlow(p.Arguments)
	case "evaluate_flow":
		return s.toolEvaluateFlow(ctx, p.Arguments)
	case "git_list_flows":
		return s.toolGitListFlows(ctx, p.Arguments)
	case "git_load_flow":
		return s.toolGitLoadFlow(ctx, p.Arguments)
	case "git_list_models":
		return s.toolGitListModels(ctx, p.Arguments)
	case "git_load_model":
		return s.toolGitLoadModel(ctx, p.Arguments)
	case "git_propose":
		return s.toolGitPropose(ctx, p.Arguments)
	default:
		return toolError("unknown tool: " + p.Name), nil
	}
}

func (s *Server) toolListModels() (any, *rpcError) {
	infos := s.store.List()
	summaries := make([]modelSummary, 0, len(infos))
	for _, mi := range infos {
		summaries = append(summaries, modelSummary{ModelID: mi.ID, Decisions: mi.Decisions, Inputs: mi.Inputs})
	}
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
	id, _, index, diags, err := s.store.Compile(ctx, []byte(a.XML))
	if err != nil {
		return toolError("could not compile model: " + err.Error()), nil
	}
	return toolText(modelResponse{
		ModelID:     id,
		Decisions:   index.Decisions,
		Inputs:      index.Inputs,
		Diagnostics: toDiagnosticDTOs(diags),
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
	defs, _, ok := s.store.Lookup(a.ModelID)
	if !ok {
		return toolError("no model with id " + a.ModelID + "; load it first with load_model"), nil
	}
	if a.Decision == "" {
		return toolError("missing required argument: decision"), nil
	}
	dec, err := defs.Decision(a.Decision)
	if err != nil {
		return toolError(err.Error()), nil
	}
	// reachableInputs is additive alongside inputs: the direct inputs plus those
	// reached transitively through required decisions — the full set a flow step
	// targeting this decision may wire (ADR-0026). inputs (direct declaration) is
	// unchanged.
	reachable, _ := defs.ReachableInputSchema(a.Decision)
	return toolText(map[string]any{
		"modelId":         a.ModelID,
		"decision":        dec.Name(),
		"decisionId":      dec.ID(),
		"inputs":          dec.InputSchema(),
		"reachableInputs": reachable,
	})
}

func (s *Server) toolEvaluate(ctx context.Context, raw json.RawMessage) (any, *rpcError) {
	var a struct {
		ModelID  string         `json:"modelId"`
		XML      string         `json:"xml"`
		Decision string         `json:"decision"`
		Input    map[string]any `json:"input"`
		Explain  bool           `json:"explain"`
		Strict   bool           `json:"strict"`
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
		d, _, ok := s.store.Lookup(a.ModelID)
		if !ok {
			return toolError("no model with id " + a.ModelID + "; load it first with load_model"), nil
		}
		defs = d
	case a.XML != "":
		_, d, _, _, err := s.store.Compile(ctx, []byte(a.XML))
		if err != nil {
			return toolError("could not compile model: " + err.Error()), nil
		}
		defs = d
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
	if a.Strict {
		opts = append(opts, dmn.WithStrictInput())
	}
	res, err := dec.Evaluate(ctx, dmn.Input(a.Input), opts...)
	if err != nil {
		var ie *dmn.InputError
		if errors.As(err, &ie) {
			b, _ := json.MarshalIndent(map[string]any{
				"error":    "input validation failed",
				"problems": ie.Problems,
			}, "", "  ")
			return map[string]any{
				"content": []any{map[string]any{"type": "text", "text": string(b)}},
				"isError": true,
			}, nil
		}
		return toolError("evaluation failed: " + err.Error()), nil
	}
	return toolText(evaluateResponse{
		Outputs:     res.Outputs,
		Decisions:   res.Decisions,
		Diagnostics: toDiagnosticDTOs(res.Diags),
		Trace:       res.Trace,
	})
}

// --- default in-process model store (content-addressed, mirrors the HTTP service) ---

// memStore is the Server's default cache: content-addressed, mutex-guarded and
// unbounded. WithStore replaces it when the cache must be shared with another
// surface. It reaches the engine only through the public dmn package (ADR-0005).
type memStore struct {
	engine *dmn.Engine
	mu     sync.RWMutex
	models map[string]*storedModel
}

func newMemStore(engine *dmn.Engine) *memStore {
	return &memStore{engine: engine, models: map[string]*storedModel{}}
}

func (m *memStore) Compile(ctx context.Context, xml []byte) (string, *dmn.Definitions, dmn.ModelIndex, dmn.Diagnostics, error) {
	id := modelID(xml)

	m.mu.RLock()
	if sm, ok := m.models[id]; ok {
		m.mu.RUnlock()
		return sm.id, sm.defs, sm.index, sm.diags, nil
	}
	m.mu.RUnlock()

	defs, diags, err := m.engine.Compile(ctx, xml)
	if err != nil {
		return "", nil, dmn.ModelIndex{}, nil, err
	}
	sm := &storedModel{id: id, defs: defs, index: defs.Index(), diags: diags}

	m.mu.Lock()
	m.models[id] = sm
	m.mu.Unlock()
	return sm.id, sm.defs, sm.index, sm.diags, nil
}

func (m *memStore) Lookup(id string) (*dmn.Definitions, dmn.ModelIndex, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sm, ok := m.models[id]
	if !ok {
		return nil, dmn.ModelIndex{}, false
	}
	return sm.defs, sm.index, true
}

func (m *memStore) List() []ModelInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ModelInfo, 0, len(m.models))
	for _, sm := range m.models {
		out = append(out, ModelInfo{ID: sm.id, Decisions: sm.index.Decisions, Inputs: sm.index.Inputs})
	}
	return out
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
