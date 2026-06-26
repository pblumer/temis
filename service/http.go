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
	"sync"

	"github.com/pblumer/temis/dmn"
)

// maxBodyBytes caps request bodies to keep a single request from exhausting
// memory; oversized bodies are rejected. Tunable once configurable limits land
// (WP-34).
const maxBodyBytes = 8 << 20 // 8 MiB

// Server is the HTTP front end over a dmn.Engine. It is safe for concurrent use:
// the engine is stateless and the model cache is guarded by a mutex. The zero
// value is not usable; construct one with NewServer.
type Server struct {
	engine *dmn.Engine

	mu     sync.RWMutex
	models map[string]*storedModel
}

// storedModel is a compiled model held in the cache together with the index and
// any diagnostics produced while compiling it.
type storedModel struct {
	id    string
	defs  *dmn.Definitions
	index dmn.ModelIndex
	diags dmn.Diagnostics
}

// NewServer returns a Server backed by engine. If engine is nil a default engine
// is used.
func NewServer(engine *dmn.Engine) *Server {
	if engine == nil {
		engine = dmn.New()
	}
	return &Server{engine: engine, models: map[string]*storedModel{}}
}

// Handler returns the HTTP handler exposing the service routes. It uses the
// standard library's method-and-pattern mux (Go 1.22+), so no external router is
// required.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/models", s.handleCreateModel)
	mux.HandleFunc("GET /v1/models/{id}", s.handleGetModel)
	mux.HandleFunc("POST /v1/models/{id}/evaluate", s.handleEvaluateModel)
	mux.HandleFunc("POST /v1/evaluate", s.handleEvaluateStateless)
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /readyz", s.handleHealth)
	return mux
}

// --- request/response DTOs ---

type modelResponse struct {
	ModelID     string          `json:"modelId"`
	Decisions   []string        `json:"decisions"`
	Inputs      []string        `json:"inputs"`
	Diagnostics []diagnosticDTO `json:"diagnostics,omitempty"`
}

type evaluateModelRequest struct {
	Decision string         `json:"decision"`
	Input    map[string]any `json:"input"`
}

type evaluateStatelessRequest struct {
	XML      string         `json:"xml"`
	Decision string         `json:"decision"`
	Input    map[string]any `json:"input"`
}

type evaluateResponse struct {
	Outputs     map[string]any  `json:"outputs"`
	Decisions   map[string]any  `json:"decisions"`
	Diagnostics []diagnosticDTO `json:"diagnostics,omitempty"`
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
		Decisions:   sm.index.Decisions,
		Inputs:      sm.index.Inputs,
		Diagnostics: toDiagnosticDTOs(sm.diags),
	})
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
		Decisions:   sm.index.Decisions,
		Inputs:      sm.index.Inputs,
		Diagnostics: toDiagnosticDTOs(sm.diags),
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
	s.evaluate(w, r.Context(), sm.defs, req.Decision, req.Input)
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
	s.evaluate(w, r.Context(), sm.defs, req.Decision, req.Input)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// evaluate runs a decision and writes the result or an appropriate problem.
func (s *Server) evaluate(w http.ResponseWriter, ctx context.Context, defs *dmn.Definitions, decision string, input map[string]any) {
	if decision == "" {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", "missing decision")
		return
	}
	dec, err := defs.Decision(decision)
	if err != nil {
		writeProblem(w, http.StatusNotFound, "DECISION_NOT_FOUND", err.Error())
		return
	}
	res, err := dec.Evaluate(ctx, dmn.Input(input))
	if err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "EVALUATION_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, evaluateResponse{
		Outputs:     res.Outputs,
		Decisions:   res.Decisions,
		Diagnostics: toDiagnosticDTOs(res.Diags),
	})
}

// --- model store ---

// compileAndStore compiles xml and caches the result under its content hash,
// reusing an existing entry when the same document was already compiled.
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

// problem is an RFC-7807 problem detail with a stable engine-specific code.
type problem struct {
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
	Code   string `json:"code"`
}

func writeProblem(w http.ResponseWriter, status int, code, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problem{
		Title:  http.StatusText(status),
		Status: status,
		Detail: detail,
		Code:   code,
	})
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
