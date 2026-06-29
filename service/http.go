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
	"net/http"
	"sort"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/pblumer/temis/dmn"
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

// WithCacheSize bounds how many compiled models the server keeps in memory.
// When the cache is full the least-recently-used model is evicted; a subsequent
// request for it recompiles on upload. A size <= 0 means unbounded (no
// eviction). The default is a bounded cache (WP-35).
func WithCacheSize(size int) Option {
	return func(s *Server) { s.cacheSize = size }
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
	if s.loadExamplesOnInit {
		s.loadExamples(context.Background())
	}
	return s
}

// Handler returns the HTTP handler exposing the service routes. It uses the
// standard library's method-and-pattern mux (Go 1.22+), so no external router is
// required.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	// Data endpoints: gated by the optional bearer token.
	mux.HandleFunc("POST /v1/models", s.requireToken(s.handleCreateModel))
	mux.HandleFunc("GET /v1/models", s.requireToken(s.handleListModels))
	mux.HandleFunc("GET /v1/models/{id}", s.requireToken(s.handleGetModel))
	mux.HandleFunc("GET /v1/models/{id}/xml", s.requireToken(s.handleGetModelXML))
	mux.HandleFunc("GET /v1/models/{id}/graph", s.requireToken(s.handleGetModelGraph))
	mux.HandleFunc("GET /v1/models/{id}/decisions/{decision}/table", s.requireToken(s.handleGetDecisionTable))
	mux.HandleFunc("POST /v1/models/{id}/save", s.requireToken(s.handleSaveModel))
	mux.HandleFunc("POST /v1/models/{id}/evaluate", s.requireToken(s.handleEvaluateModel))
	mux.HandleFunc("POST /v1/evaluate", s.requireToken(s.handleEvaluateStateless))
	// Discovery and probes: always public.
	mux.HandleFunc("GET /{$}", s.handleUI)
	mux.HandleFunc("GET /ui", s.handleUI)
	mux.HandleFunc("GET /og-image.png", s.handleOGImage)
	mux.HandleFunc("GET /docs", s.handleDocs)
	mux.HandleFunc("GET /openapi.yaml", s.handleOpenAPISpec)
	// Own DMN modeler frontend (ADR-0016), embedded — no CDN, offline. Served as
	// a subtree under /app/; /ui keeps the legacy dmn-js editor until WP-67.
	mux.Handle("GET /app/", http.StripPrefix("/app/", http.FileServerFS(webui.Assets())))
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /readyz", s.handleHealth)

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
}

type saveModelRequest struct {
	Nodes []dmn.NodeEdit `json:"nodes"`
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

// handleGetModelXML returns a cached model's raw DMN XML, so a client (the /ui
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
	s.evaluate(w, r.Context(), sm.defs, req.Decision, req.Input, req.Explain, req.Strict)
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
	s.evaluate(w, r.Context(), sm.defs, req.Decision, req.Input, req.Explain, req.Strict)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// evaluate runs a decision and writes the result or an appropriate problem. When
// explain is set the response carries the decision trace (which rules matched and
// why).
func (s *Server) evaluate(w http.ResponseWriter, ctx context.Context, defs *dmn.Definitions, decision string, input map[string]any, explain, strict bool) {
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
	return sm, nil
}

func (s *Server) lookup(id string) (*storedModel, bool) {
	return s.cache.get(id)
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
