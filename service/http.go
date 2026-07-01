// Package service hosts the HTTP handlers that expose the engine as a network
// service. It accesses the engine only through the public dmn package, never
// through internal/ (architecture D5/ADR-0005).
//
// The server compiles DMN models on upload, caches them in memory keyed by the
// SHA-256 of their XML (so re-uploading the same document is idempotent and
// returns the same model id), and evaluates a named decision against a JSON
// input context. A stateless endpoint compiles and evaluates in one request.
//
// Errors are returned as RFC-7807 application/problem+json with a stable,
// machine-readable code. The routes follow docs/40-api-contract.md §2.
package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/pblumer/temis/dmn"
	"github.com/pblumer/temis/mcp"
	webui "github.com/pblumer/temis/web"
)

// maxBodyBytes caps request bodies to keep a single request from exhausting
// memory; oversized bodies are rejected. This is the transport-level guard,
// distinct from the engine's evaluation limits (WP-34).
const maxBodyBytes = 8 << 20 // 8 MiB

// Server is the HTTP front end over a dmn.Engine. It is safe for concurrent use:
// the engine is stateless and the model cache is guarded by a mutex. The zero
// value is not usable; construct one with NewServer.
type Server struct {
	engine *dmn.Engine

	// token, when non-empty, is the bearer token required on the /v1 data
	// endpoints. Empty means the API is open.
	token string

	// listModels enables the GET /v1/models listing endpoint. When false the
	// handler responds 404, so callers cannot enumerate the cached models (and
	// thereby the decisions in them). Defaults to true.
	listModels bool

	// loadExamplesOnInit preloads the bundled example models at construction
	// (set via WithExamples).
	loadExamplesOnInit bool

	// cacheSize is the model cache capacity applied at construction; 0 means
	// unbounded. NewServer builds cache from it after options run.
	cacheSize int
	cache     *modelCache

	// storeDir, when non-empty, is a filesystem directory that persists uploaded
	// and edited models so they survive a restart (ADR-0027); set via
	// WithModelStore. NewServer opens store from it after options run. Empty
	// leaves the server purely in-memory (the default).
	storeDir string
	store    *diskStore

	// mcpServer, when set via AttachMCP, co-locates the MCP endpoint (/mcp) in
	// this server's mux so it shares this server's model cache — one process, one
	// address space. Nil leaves /mcp unmounted.
	mcpServer *mcp.Server

	// assist, when set via WithAssist, enables the modeling assistant at
	// POST /v1/chat (ADR-0024). Nil leaves the endpoint dormant (503).
	assist *AssistConfig

	// sink, when set via WithClioSink, records each single-decision evaluation as
	// a tamper-evident event in a clio instance (ADR-0023). Nil disables audit
	// logging, leaving behaviour byte-identical to a server without it.
	sink *ClioSink

	// gitBaseURL overrides the GitHub REST API root for the /v1/git endpoints
	// (default https://api.github.com); set via WithGitHubBaseURL for GitHub
	// Enterprise or tests. The git-provider token is supplied per request
	// (X-Git-Token), never stored on the server (WP-72, auth model A).
	gitBaseURL string
}

// Option configures a Server at construction time.
type Option func(*Server)

// WithToken requires callers of the /v1 data endpoints to present
// "Authorization: Bearer <token>". An empty token leaves the API open. The
// docs, OpenAPI spec and health endpoints are never gated.
func WithToken(token string) Option {
	return func(s *Server) { s.token = token }
}

// WithModelListing toggles the GET /v1/models endpoint that enumerates every
// cached model with its decisions and inputs. Listing is enabled by default;
// pass WithModelListing(false) to keep the cached decisions private — the
// endpoint then responds 404 as if it did not exist.
func WithModelListing(enabled bool) Option {
	return func(s *Server) { s.listModels = enabled }
}

// WithClioSink attaches a clio audit sink so each single-decision evaluation
// (POST /v1/evaluate and POST /v1/models/{id}/evaluate) is recorded as a
// tamper-evident decision event in clio (ADR-0023). A nil sink is ignored,
// leaving the server's behaviour unchanged.
func WithClioSink(sink *ClioSink) Option {
	return func(s *Server) { s.sink = sink }
}

// WithCacheSize bounds how many compiled models the server keeps in memory.
// When the cache is full the least-recently-used model is evicted; a subsequent
// request for it recompiles on upload. A size <= 0 means unbounded (no
// eviction). The default is a bounded cache (WP-35).
func WithCacheSize(size int) Option {
	return func(s *Server) { s.cacheSize = size }
}

// WithModelStore persists uploaded and edited models to dir on disk and reloads
// them into the cache on the next start, so the server's models survive a restart
// (ADR-0027). Models are stored content-addressed as raw DMN XML; the bundled
// examples are not persisted (they re-embed on every start). An empty dir keeps
// the server purely in-memory — the default, byte-identical to before.
func WithModelStore(dir string) Option {
	return func(s *Server) { s.storeDir = dir }
}

// storedModel is a compiled model held in the cache together with the index and
// any diagnostics produced while compiling it.
type storedModel struct {
	id    string
	name  string // display name: the DMN definitions name, or a preset for examples
	xml   []byte // the raw DMN XML as uploaded, served back for the editor
	defs  *dmn.Definitions
	index dmn.ModelIndex
	diags dmn.Diagnostics
	// seq is a monotonic creation order assigned by the cache on first store, so a
	// client can present same-named revisions newest-first (a model's history).
	seq uint64
}

// NewServer returns a Server backed by engine. If engine is nil a default engine
// is used. Options such as WithToken tune the server's behaviour.
func NewServer(engine *dmn.Engine, opts ...Option) *Server {
	if engine == nil {
		engine = dmn.New()
	}
	s := &Server{engine: engine, listModels: true, cacheSize: defaultCacheSize}
	for _, opt := range opts {
		opt(s)
	}
	s.cache = newModelCache(s.cacheSize)
	// Examples load first, while store is still nil, so the bundled models are
	// never written to disk — they re-embed on every start (ADR-0027).
	if s.loadExamplesOnInit {
		s.loadExamples(context.Background())
	}
	// Then open the optional on-disk store and repopulate the cache from it, so a
	// user's own uploaded/edited models survive a restart. A store that cannot be
	// opened is logged and disabled rather than blocking startup.
	if s.storeDir != "" {
		store, err := newDiskStore(s.storeDir)
		if err != nil {
			log.Printf("temis: model store disabled: %v", err)
		} else {
			s.store = store
			s.loadPersisted(context.Background())
		}
	}
	return s
}

// loadPersisted repopulates the cache from the on-disk store (ADR-0027). It runs
// at construction, before the server serves, so it needs no locking beyond the
// cache's own. A model that no longer compiles is skipped — never blocking the
// server — and left on disk so a later fix can recover it.
func (s *Server) loadPersisted(ctx context.Context) {
	xmls, err := s.store.load()
	if err != nil {
		log.Printf("temis: model store: %v", err)
		return
	}
	for _, xml := range xmls {
		if _, err := s.compileAndStore(ctx, xml); err != nil {
			continue
		}
	}
}

// route is one token-gated /v1 data endpoint: an HTTP method, a Go 1.22 mux
// pattern and the handler that serves it. dataRoutes() is the single list of
// them, so registration (Handler) and the OpenAPI-sync test share one source.
type route struct {
	method  string
	pattern string
	handler http.HandlerFunc
}

// dataRoutes is the canonical list of token-gated /v1 endpoints. Every entry
// must have a matching path+method in service/openapi.yaml (enforced by
// TestOpenAPICoversDataRoutes); adding a route here without documenting it — or
// vice versa — breaks that test on purpose.
func (s *Server) dataRoutes() []route {
	return []route{
		{"POST", "/v1/models", s.handleCreateModel},
		{"GET", "/v1/models", s.handleListModels},
		{"GET", "/v1/models/{id}", s.handleGetModel},
		{"DELETE", "/v1/models/{id}", s.handleDeleteModel},
		{"GET", "/v1/models/{id}/xml", s.handleGetModelXML},
		{"POST", "/v1/models/{id}/rename", s.handleRenameModel},
		// Modeler (ADR-0016): structure, types and per-decision logic editing that
		// backs the built-in DMN modeler frontend; all on the same token-gated /v1
		// surface. The mutating ones recompile and return the saved model (201).
		{"GET", "/v1/models/{id}/graph", s.handleGetModelGraph},
		{"POST", "/v1/models/{id}/graph", s.handleSaveGraph},
		{"GET", "/v1/models/{id}/types", s.handleGetTypes},
		{"POST", "/v1/models/{id}/types", s.handleSaveType},
		{"DELETE", "/v1/models/{id}/types/{name}", s.handleDeleteType},
		{"GET", "/v1/models/{id}/decisions/{decision}/table", s.handleGetDecisionTable},
		{"POST", "/v1/models/{id}/decisions/{decision}/table", s.handleSaveDecisionTable},
		{"POST", "/v1/models/{id}/decisions/{decision}/create-table", s.handleCreateDecisionTable},
		{"GET", "/v1/models/{id}/decisions/{decision}/literal", s.handleGetLiteral},
		{"POST", "/v1/models/{id}/decisions/{decision}/literal", s.handleSaveLiteral},
		{"GET", "/v1/models/{id}/decisions/{decision}/context", s.handleGetContext},
		{"POST", "/v1/models/{id}/decisions/{decision}/context", s.handleSaveContext},
		{"POST", "/v1/models/{id}/decisions/{decision}/create-context", s.handleCreateContext},
		{"GET", "/v1/models/{id}/decisions/{decision}/conditional", s.handleGetConditional},
		{"POST", "/v1/models/{id}/decisions/{decision}/conditional", s.handleSaveConditional},
		{"POST", "/v1/models/{id}/decisions/{decision}/create-conditional", s.handleCreateConditional},
		{"GET", "/v1/models/{id}/decisions/{decision}/list", s.handleGetList},
		{"POST", "/v1/models/{id}/decisions/{decision}/list", s.handleSaveList},
		{"POST", "/v1/models/{id}/decisions/{decision}/create-list", s.handleCreateList},
		{"GET", "/v1/models/{id}/decisions/{decision}/relation", s.handleGetRelation},
		{"POST", "/v1/models/{id}/decisions/{decision}/relation", s.handleSaveRelation},
		{"POST", "/v1/models/{id}/decisions/{decision}/create-relation", s.handleCreateRelation},
		{"GET", "/v1/models/{id}/decisions/{decision}/filter", s.handleGetFilter},
		{"POST", "/v1/models/{id}/decisions/{decision}/filter", s.handleSaveFilter},
		{"POST", "/v1/models/{id}/decisions/{decision}/create-filter", s.handleCreateFilter},
		{"GET", "/v1/models/{id}/bkm/{bkm}", s.handleGetBKM},
		{"POST", "/v1/models/{id}/bkm/{bkm}", s.handleSaveBKM},
		{"POST", "/v1/models/{id}/save", s.handleSaveModel},
		// Evaluation.
		{"POST", "/v1/models/{id}/evaluate", s.handleEvaluateModel},
		{"POST", "/v1/models/{id}/evaluate-graph", s.handleEvaluateGraph},
		{"POST", "/v1/evaluate", s.handleEvaluateStateless},
		// Modeling assistant (ADR-0024): an LLM drives temis's tools to help build
		// decisions. Dormant (503) until enabled with WithAssist.
		{"POST", "/v1/chat", s.handleChat},
	}
}

// Handler returns the HTTP handler exposing the service routes. It uses the
// standard library's method-and-pattern mux (Go 1.22+), so no external router is
// required.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	// Data endpoints: gated by the optional bearer token. Registered from the
	// single dataRoutes() table so the route set has one source of truth — the
	// OpenAPI-sync test reads the same table (see http_test.go).
	for _, rt := range s.dataRoutes() {
		mux.HandleFunc(rt.method+" "+rt.pattern, s.requireToken(rt.handler))
	}
	// Git-backed models: browse, load, save and propose against a repository
	// (WP-72). Registered outside the dataRoutes() table (and thus the
	// OpenAPI-sync test) for now — the git endpoints are not in openapi.yaml yet.
	// The git-provider token is per request (X-Git-Token); these endpoints share
	// the same optional API token gate as the others.
	s.registerGitRoutes(mux)
	// Discovery and probes: always public.
	mux.HandleFunc("GET /docs", s.handleDocs)
	mux.HandleFunc("GET /openapi.yaml", s.handleOpenAPISpec)
	// Own DMN modeler frontend (ADR-0016, WP-67 cutover): the embedded SPA is now
	// THE editor, served at the site root — no dmn-js, no CDN, offline. The legacy
	// /ui and /app/ paths redirect here so old links keep working. This catch-all
	// also serves the SPA's assets (assets/, feel.wasm, wasm_exec.js). It is
	// registered method-agnostically so it does not overlap the gRPC handler's own
	// path prefix below (a method-specific "GET /" would tie with it and panic);
	// more specific routes still take precedence.
	mux.Handle("/", http.FileServerFS(webui.Assets()))
	mux.HandleFunc("GET /ui", redirectTo("/"))
	mux.HandleFunc("GET /app/", redirectTo("/"))
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /readyz", s.handleHealth)

	// MCP endpoint, co-located when attached: POST/GET /mcp share this server's
	// model cache (and its preloaded examples), so a model is visible whether it
	// was loaded over the API, the modeler or MCP (one address space).
	if s.mcpServer != nil {
		s.mcpServer.RegisterRoutes(mux)
	}

	// gRPC: the Connect handler serves the dmn.v1.DmnEngine service (gRPC,
	// gRPC-Web and Connect) under its own path prefix on the same mux (WP-33).
	grpcPath, grpcHandler := s.grpcHandler()
	mux.Handle(grpcPath, grpcHandler)

	// h2c lets full gRPC and the bidi EvaluateBatch stream work over cleartext
	// HTTP/2 (no TLS); HTTP/1.1 requests are still served normally.
	return h2c.NewHandler(mux, &http2.Server{})
}

// --- request/response DTOs ---

type modelResponse struct {
	ModelID     string                      `json:"modelId"`
	Name        string                      `json:"name,omitempty"`
	Decisions   []string                    `json:"decisions"`
	Inputs      []string                    `json:"inputs"`
	Schema      map[string][]dmn.InputField `json:"schema,omitempty"`
	Diagnostics []diagnosticDTO             `json:"diagnostics,omitempty"`
}

// schemaOf returns each executable decision's typed input schema, keyed by
// decision name, for self-description.
func schemaOf(defs *dmn.Definitions, decisions []string) map[string][]dmn.InputField {
	if len(decisions) == 0 {
		return nil
	}
	out := make(map[string][]dmn.InputField, len(decisions))
	for _, name := range decisions {
		if fields, err := defs.InputSchema(name); err == nil {
			out[name] = fields
		}
	}
	return out
}

type modelListResponse struct {
	Models []modelSummary `json:"models"`
	Count  int            `json:"count"`
}

type modelSummary struct {
	ModelID   string   `json:"modelId"`
	Name      string   `json:"name,omitempty"`
	Decisions []string `json:"decisions"`
	Inputs    []string `json:"inputs"`
	// Seq is the model's creation order (higher = newer), so the client can show a
	// model's same-named revisions newest-first as a history.
	Seq uint64 `json:"seq"`
}

type saveModelRequest struct {
	Nodes []dmn.NodeEdit `json:"nodes"`
}

type renameModelRequest struct {
	Name string `json:"name"`
}

type evaluateModelRequest struct {
	Decision string         `json:"decision"`
	Input    map[string]any `json:"input"`
	Explain  bool           `json:"explain,omitempty"`
	Strict   bool           `json:"strict,omitempty"`
}

type evaluateStatelessRequest struct {
	XML      string         `json:"xml"`
	Decision string         `json:"decision"`
	Input    map[string]any `json:"input"`
	Explain  bool           `json:"explain,omitempty"`
	Strict   bool           `json:"strict,omitempty"`
}

type evaluateResponse struct {
	Outputs     map[string]any  `json:"outputs"`
	Decisions   map[string]any  `json:"decisions"`
	Diagnostics []diagnosticDTO `json:"diagnostics,omitempty"`
	Trace       *dmn.Trace      `json:"trace,omitempty"`
}

type evaluateGraphRequest struct {
	Input   map[string]any `json:"input"`
	Explain bool           `json:"explain,omitempty"`
	Strict  bool           `json:"strict,omitempty"`
}

// evaluateGraphResponse carries the whole model's result: every decision's value
// (and trace with explain), the inputs the graph consumes (so a client can build
// the form from one source of truth), and any per-decision evaluation errors.
type evaluateGraphResponse struct {
	Values      map[string]any        `json:"values"`
	Traces      map[string]*dmn.Trace `json:"traces,omitempty"`
	Errors      map[string]string     `json:"errors,omitempty"`
	InputSchema []dmn.InputField      `json:"inputSchema"`
	Diagnostics []diagnosticDTO       `json:"diagnostics,omitempty"`
}

type diagnosticDTO struct {
	Severity   string `json:"severity"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	DecisionID string `json:"decisionId,omitempty"`
	Line       int    `json:"line,omitempty"`
	Col        int    `json:"col,omitempty"`
}

// --- handlers ---

// handleCreateModel compiles an uploaded DMN XML document (the raw request body)
// and caches it. It responds 201 with the model id, index and any per-decision
// compile diagnostics. Malformed XML is a 400.
func (s *Server) handleCreateModel(w http.ResponseWriter, r *http.Request) {
	xml, err := readBody(w, r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	sm, err := s.compileAndStore(r.Context(), xml)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "MALFORMED_XML", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, modelResponse{
		ModelID:     sm.id,
		Name:        sm.name,
		Decisions:   sm.index.Decisions,
		Inputs:      sm.index.Inputs,
		Schema:      schemaOf(sm.defs, sm.index.Decisions),
		Diagnostics: toDiagnosticDTOs(sm.diags),
	})
}

// handleListModels returns a summary of every model currently held in the
// cache, sorted by id for a stable order. When listing is disabled
// (WithModelListing(false)) it responds 404 so the cached decisions stay
// private and the endpoint looks absent.
func (s *Server) handleListModels(w http.ResponseWriter, _ *http.Request) {
	if !s.listModels {
		writeProblem(w, http.StatusNotFound, "NOT_FOUND", "model listing is disabled")
		return
	}

	models := s.cache.snapshot()
	summaries := make([]modelSummary, 0, len(models))
	for _, sm := range models {
		summaries = append(summaries, modelSummary{
			ModelID:   sm.id,
			Name:      sm.name,
			Decisions: sm.index.Decisions,
			Inputs:    sm.index.Inputs,
			Seq:       sm.seq,
		})
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].ModelID < summaries[j].ModelID
	})
	writeJSON(w, http.StatusOK, modelListResponse{Models: summaries, Count: len(summaries)})
}

// handleGetModel returns a cached model's index.
func (s *Server) handleGetModel(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	writeJSON(w, http.StatusOK, modelResponse{
		ModelID:     sm.id,
		Name:        sm.name,
		Decisions:   sm.index.Decisions,
		Inputs:      sm.index.Inputs,
		Schema:      schemaOf(sm.defs, sm.index.Decisions),
		Diagnostics: toDiagnosticDTOs(sm.diags),
	})
}

// handleDeleteModel drops a cached model (one revision) from the cache. The
// modeler uses it to remove a model the user no longer wants — deleting a whole
// named group is done by the client calling this once per revision. It responds
// 204 on success and 404 when no model has that id.
func (s *Server) handleDeleteModel(w http.ResponseWriter, r *http.Request) {
	if !s.cache.delete(r.PathValue("id")) {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleRenameModel sets a cached model's display name (the DMN definitions name)
// and caches the recompiled document under its new content hash, responding 201
// with the saved model's id so the client can switch to it — the model-level
// counterpart to renaming an element via /save (ADR-0016). The original revision
// stays cached; the modeler removes it (and renames the rest of the group) via
// DELETE when it renames a whole named model.
func (s *Server) handleRenameModel(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	var req renameModelRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", "name must not be empty")
		return
	}
	patched, err := dmn.SetModelName(sm.xml, name)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "MALFORMED_XML", err.Error())
		return
	}
	saved, err := s.compileAndStore(r.Context(), patched)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "MALFORMED_XML", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, modelResponse{
		ModelID:     saved.id,
		Name:        saved.name,
		Decisions:   saved.index.Decisions,
		Inputs:      saved.index.Inputs,
		Schema:      schemaOf(saved.defs, saved.index.Decisions),
		Diagnostics: toDiagnosticDTOs(saved.diags),
	})
}

// handleGetModelXML returns a cached model's raw DMN XML, so a client (the
// editor) can reopen a model that was previously deployed to the server.
func (s *Server) handleGetModelXML(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(sm.xml)
}

// handleGetModelGraph returns a cached model's decision requirements graph
// (nodes + requirement edges), so the own modeler frontend can draw it without
// parsing DMN XML in the browser (ADR-0016).
func (s *Server) handleGetModelGraph(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	writeJSON(w, http.StatusOK, sm.defs.Graph())
}

// handleGetTypes returns the model's named item definitions, for the modeler's
// type manager and type pickers (ADR-0016).
func (s *Server) handleGetTypes(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"types": sm.defs.ItemDefinitions()})
}

// handleSaveType creates or updates a simple item definition, recompiles and
// caches the model, and returns the new id. A structured (component) type or an
// empty name is a 400.
func (s *Server) handleSaveType(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	var t dmn.ItemType
	if err := decodeJSON(w, r, &t); err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	patched, err := dmn.SetItemDefinition(sm.xml, t)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "TYPE_SAVE_FAILED", err.Error())
		return
	}
	s.respondSaved(w, r, patched)
}

// handleDeleteType removes a named item definition and returns the recompiled
// model's new id.
func (s *Server) handleDeleteType(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	patched, err := dmn.RemoveItemDefinition(sm.xml, r.PathValue("name"))
	if err != nil {
		writeProblem(w, http.StatusNotFound, "TYPE_NOT_FOUND", err.Error())
		return
	}
	s.respondSaved(w, r, patched)
}

// respondSaved compiles and caches patched XML and writes the saved model
// response, the common tail of the modeler's mutating endpoints.
func (s *Server) respondSaved(w http.ResponseWriter, r *http.Request, patched []byte) {
	saved, err := s.compileAndStore(r.Context(), patched)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "MALFORMED_XML", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, modelResponse{
		ModelID:     saved.id,
		Name:        saved.name,
		Decisions:   saved.index.Decisions,
		Inputs:      saved.index.Inputs,
		Schema:      schemaOf(saved.defs, saved.index.Decisions),
		Diagnostics: toDiagnosticDTOs(saved.diags),
	})
}

// handleGetDecisionTable returns a decision's static decision-table view (hit
// policy, columns and rule rows), so the modeler can open it on double-click
// without parsing DMN XML in the browser (ADR-0016). It is a 404 when the model,
// the decision, or its decision-table logic is absent.
func (s *Server) handleGetDecisionTable(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	table, ok := sm.defs.DecisionTable(r.PathValue("decision"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "TABLE_NOT_FOUND", "no decision table for that decision")
		return
	}
	writeJSON(w, http.StatusOK, table)
}

// handleGetBKM returns a business knowledge model's encapsulated-logic view, or
// 404 when there is no such BKM.
func (s *Server) handleGetBKM(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	bkm, ok := sm.defs.BKMFunction(r.PathValue("bkm"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "BKM_NOT_FOUND", "no business knowledge model with that id")
		return
	}
	writeJSON(w, http.StatusOK, bkm)
}

// handleSaveBKM sets a business knowledge model's function (parameters + literal
// body), recompiles and caches the model, and returns the new id with any compile
// diagnostics.
func (s *Server) handleSaveBKM(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	var edit dmn.BKMFunctionEdit
	if err := decodeJSON(w, r, &edit); err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	patched, err := dmn.SetBKMFunction(sm.xml, r.PathValue("bkm"), edit)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "BKM_SAVE_FAILED", err.Error())
		return
	}
	s.respondSaved(w, r, patched)
}

// handleGetLiteral returns a decision's literal-expression view, or 404 when the
// decision's logic is not a literal expression.
func (s *Server) handleGetLiteral(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	lit, ok := sm.defs.LiteralExpression(r.PathValue("decision"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "LITERAL_NOT_FOUND", "no literal expression for that decision")
		return
	}
	writeJSON(w, http.StatusOK, lit)
}

type saveLiteralRequest struct {
	Text    string `json:"text"`
	TypeRef string `json:"typeRef"`
}

// handleSaveLiteral sets (or creates) a decision's literal-expression logic,
// recompiles and caches the model, and returns the new id with any compile
// diagnostics (so the client can surface a FEEL error). It is a 404/400 when the
// decision is unknown or already has non-literal logic (ADR-0016).
func (s *Server) handleSaveLiteral(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	var req saveLiteralRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	patched, err := dmn.SetLiteralExpression(sm.xml, r.PathValue("decision"), req.Text, req.TypeRef)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "LITERAL_SAVE_FAILED", err.Error())
		return
	}
	saved, err := s.compileAndStore(r.Context(), patched)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "MALFORMED_XML", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, modelResponse{
		ModelID:     saved.id,
		Name:        saved.name,
		Decisions:   saved.index.Decisions,
		Inputs:      saved.index.Inputs,
		Schema:      schemaOf(saved.defs, saved.index.Decisions),
		Diagnostics: toDiagnosticDTOs(saved.diags),
	})
}

// handleCreateDecisionTable gives an undecided decision a fresh decision table
// (columns derived from its requirements), recompiles and caches the model, and
// returns the new id. It is a 404/400 when the decision is unknown or already has
// logic (ADR-0016).
func (s *Server) handleCreateDecisionTable(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	patched, err := dmn.CreateDecisionTable(sm.xml, r.PathValue("decision"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "TABLE_CREATE_FAILED", err.Error())
		return
	}
	saved, err := s.compileAndStore(r.Context(), patched)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "MALFORMED_XML", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, modelResponse{
		ModelID:     saved.id,
		Name:        saved.name,
		Decisions:   saved.index.Decisions,
		Inputs:      saved.index.Inputs,
		Schema:      schemaOf(saved.defs, saved.index.Decisions),
		Diagnostics: toDiagnosticDTOs(saved.diags),
	})
}

// handleSaveDecisionTable rewrites a decision's decision-table rules and caches
// the recompiled model under its new content hash, returning the saved model's
// id and any compile diagnostics (so the client can surface a cell the engine
// rejects). The table's columns and hit policy are preserved (ADR-0016). It is a
// 404 when the model or the decision's table is absent.
func (s *Server) handleSaveDecisionTable(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	var edit dmn.TableEdit
	if err := decodeJSON(w, r, &edit); err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	patched, err := dmn.ApplyTableEdit(sm.xml, r.PathValue("decision"), edit)
	if err != nil {
		writeProblem(w, http.StatusNotFound, "TABLE_NOT_FOUND", err.Error())
		return
	}
	saved, err := s.compileAndStore(r.Context(), patched)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "MALFORMED_XML", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, modelResponse{
		ModelID:     saved.id,
		Name:        saved.name,
		Decisions:   saved.index.Decisions,
		Inputs:      saved.index.Inputs,
		Schema:      schemaOf(saved.defs, saved.index.Decisions),
		Diagnostics: toDiagnosticDTOs(saved.diags),
	})
}

// handleSaveGraph reconciles a cached model to a desired decision requirements
// graph — persisting added and removed nodes/edges as well as moved/renamed/
// retyped ones — then recompiles and caches the result, returning the new model
// id and any compile diagnostics. Surviving decisions keep their logic; new
// decisions are created undecided. It is the modeler's structural save (ADR-0016).
func (s *Server) handleSaveGraph(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	var edit dmn.GraphEdit
	if err := decodeJSON(w, r, &edit); err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	patched, err := dmn.ApplyGraph(sm.xml, edit)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "MALFORMED_XML", err.Error())
		return
	}
	saved, err := s.compileAndStore(r.Context(), patched)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "MALFORMED_XML", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, modelResponse{
		ModelID:     saved.id,
		Name:        saved.name,
		Decisions:   saved.index.Decisions,
		Inputs:      saved.index.Inputs,
		Schema:      schemaOf(saved.defs, saved.index.Decisions),
		Diagnostics: toDiagnosticDTOs(saved.diags),
	})
}

// handleSaveModel applies modeler edits (positions, names, types) to a cached
// model's XML, recompiles the patched document and caches it under its new
// content hash. It responds 201 with the saved model's id and index, so the
// client can switch to the persisted revision. The original model stays cached.
// Because edits patch the existing XML, all decision logic and the untouched
// DMNDI are preserved (ADR-0016, Edit→Save).
func (s *Server) handleSaveModel(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	var req saveModelRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	patched, err := dmn.ApplyEdits(sm.xml, req.Nodes)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "MALFORMED_XML", err.Error())
		return
	}
	saved, err := s.compileAndStore(r.Context(), patched)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "MALFORMED_XML", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, modelResponse{
		ModelID:     saved.id,
		Name:        saved.name,
		Decisions:   saved.index.Decisions,
		Inputs:      saved.index.Inputs,
		Schema:      schemaOf(saved.defs, saved.index.Decisions),
		Diagnostics: toDiagnosticDTOs(saved.diags),
	})
}

// handleEvaluateModel evaluates a decision of a cached model.
func (s *Server) handleEvaluateModel(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	var req evaluateModelRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	s.evaluate(w, r.Context(), sm.id, sm.defs, req.Decision, req.Input, req.Explain, req.Strict)
}

// handleEvaluateStateless compiles and evaluates in a single request, caching the
// model as a side effect so a follow-up by id is cheap.
func (s *Server) handleEvaluateStateless(w http.ResponseWriter, r *http.Request) {
	var req evaluateStatelessRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if req.XML == "" {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", "missing xml")
		return
	}
	sm, err := s.compileAndStore(r.Context(), []byte(req.XML))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "MALFORMED_XML", err.Error())
		return
	}
	s.evaluate(w, r.Context(), sm.id, sm.defs, req.Decision, req.Input, req.Explain, req.Strict)
}

// handleEvaluateGraph evaluates the whole model: it fills the supplied leaf
// inputs once and returns every decision's value (and trace with explain), so the
// modeler can show the entire DRG with its results rather than one decision at a
// time. Inputs are validated against the model's whole-graph schema when strict.
func (s *Server) handleEvaluateGraph(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	var req evaluateGraphRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	var opts []dmn.EvalOption
	if req.Explain {
		opts = append(opts, dmn.WithTrace())
	}
	if req.Strict {
		opts = append(opts, dmn.WithStrictInput())
	}
	res, err := sm.defs.EvaluateGraph(r.Context(), dmn.Input(req.Input), opts...)
	if err != nil {
		var ie *dmn.InputError
		if errors.As(err, &ie) {
			writeProblemDetail(w, problem{
				Title:    http.StatusText(http.StatusUnprocessableEntity),
				Status:   http.StatusUnprocessableEntity,
				Detail:   "input does not satisfy the model's schema",
				Code:     "INVALID_INPUT",
				Problems: ie.Problems,
			})
			return
		}
		writeProblem(w, http.StatusUnprocessableEntity, "EVALUATION_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, evaluateGraphResponse{
		Values:      res.Values,
		Traces:      res.Traces,
		Errors:      res.Errors,
		InputSchema: sm.defs.ModelInputSchema(),
		Diagnostics: toDiagnosticDTOs(res.Diags),
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// redirectTo permanently redirects to target. It keeps the retired /ui and /app/
// paths pointing at the modeler's new home at the site root (ADR-0016 WP-67).
func redirectTo(target string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	}
}

// evaluate runs a decision and writes the result or an appropriate problem. When
// explain is set the response carries the decision trace (which rules matched and
// why).
func (s *Server) evaluate(w http.ResponseWriter, ctx context.Context, modelID string, defs *dmn.Definitions, decision string, input map[string]any, explain, strict bool) {
	if decision == "" {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", "missing decision")
		return
	}
	dec, err := defs.Decision(decision)
	if err != nil {
		writeProblem(w, http.StatusNotFound, "DECISION_NOT_FOUND", err.Error())
		return
	}
	var opts []dmn.EvalOption
	if explain {
		opts = append(opts, dmn.WithTrace())
	}
	if strict {
		opts = append(opts, dmn.WithStrictInput())
	}
	res, err := dec.Evaluate(ctx, dmn.Input(input), opts...)
	if err != nil {
		var ie *dmn.InputError
		if errors.As(err, &ie) {
			writeProblemDetail(w, problem{
				Title:    http.StatusText(http.StatusUnprocessableEntity),
				Status:   http.StatusUnprocessableEntity,
				Detail:   "input does not satisfy the decision's schema",
				Code:     "INVALID_INPUT",
				Problems: ie.Problems,
			})
			return
		}
		writeProblem(w, http.StatusUnprocessableEntity, "EVALUATION_FAILED", err.Error())
		return
	}
	// Audit the decision before answering. In fail-closed mode a failed write
	// aborts the request (the decision must be recorded to count as made); in
	// best-effort mode Record logs and returns nil so the result still flows.
	if s.sink != nil {
		if err := s.sink.Record(ctx, DecisionRecord{
			ModelID:  modelID,
			Decision: decision,
			Input:    input,
			Outputs:  res.Outputs,
			Trace:    res.Trace,
			Strict:   strict,
		}); err != nil {
			writeProblem(w, http.StatusBadGateway, "AUDIT_WRITE_FAILED", err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, evaluateResponse{
		Outputs:     res.Outputs,
		Decisions:   res.Decisions,
		Diagnostics: toDiagnosticDTOs(res.Diags),
		Trace:       res.Trace,
	})
}

// --- model store ---

// compileAndStore compiles xml and caches the result under its content hash,
// reusing an existing entry when the same document was already compiled.
func (s *Server) compileAndStore(ctx context.Context, xml []byte) (*storedModel, error) {
	id := modelID(xml)

	if sm, ok := s.cache.get(id); ok {
		return sm, nil
	}

	defs, diags, err := s.engine.Compile(ctx, xml)
	if err != nil {
		return nil, err
	}
	sm := &storedModel{id: id, name: defs.ModelName(), xml: xml, defs: defs, index: defs.Index(), diags: diags}
	s.cache.add(sm)
	// Persist the raw XML so this model survives a restart (ADR-0027). Idempotent:
	// a content-addressed file that already exists is left untouched. A failed
	// write is logged but never fails the request — the model is already cached
	// and serving.
	if s.store != nil {
		if err := s.store.put(id, xml); err != nil {
			log.Printf("temis: model store: persisting %s: %v", id, err)
		}
	}
	return sm, nil
}

func (s *Server) lookup(id string) (*storedModel, bool) {
	if sm, ok := s.cache.get(id); ok {
		return sm, true
	}
	// Fall back to the on-disk store: a persisted model that was evicted from the
	// bounded in-memory cache is still durably available and recompiles on demand
	// (ADR-0027), re-entering the cache as most-recently-used.
	if s.store != nil {
		if xml, ok := s.store.get(id); ok {
			if sm, err := s.compileAndStore(context.Background(), xml); err == nil {
				return sm, true
			}
		}
	}
	return nil, false
}

// modelID is the cache key for an XML document: a hex SHA-256 with a "sha256:"
// prefix so the scheme is explicit in the API.
func modelID(xml []byte) string {
	sum := sha256.Sum256(xml)
	return fmt.Sprintf("sha256:%x", sum)
}

// --- helpers ---

func readBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if len(body) == 0 {
		return nil, errors.New("empty body")
	}
	return body, nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// problem is an RFC-7807 problem detail with a stable engine-specific code. The
// optional Problems extension carries structured input-validation failures
// (code INVALID_INPUT).
type problem struct {
	Title    string             `json:"title"`
	Status   int                `json:"status"`
	Detail   string             `json:"detail,omitempty"`
	Code     string             `json:"code"`
	Problems []dmn.InputProblem `json:"problems,omitempty"`
}

func writeProblem(w http.ResponseWriter, status int, code, detail string) {
	writeProblemDetail(w, problem{Title: http.StatusText(status), Status: status, Detail: detail, Code: code})
}

func writeProblemDetail(w http.ResponseWriter, p problem) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
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
