package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/pblumer/temis/dmn"
	"github.com/pblumer/temis/flow"
)

// storedFlow is a registered decision flow together with the descriptor bytes it
// was compiled from and its structural diagnostics.
type storedFlow struct {
	id    string
	flow  *flow.Flow
	desc  []byte
	diags flow.Diagnostics
}

// flowStore holds registered flows keyed by their content hash. Flows are small
// metadata artifacts and few in number, so a plain guarded map suffices (no LRU).
type flowStore struct {
	mu sync.Mutex
	m  map[string]*storedFlow
}

func newFlowStore() *flowStore { return &flowStore{m: map[string]*storedFlow{}} }

func (f *flowStore) put(sf *storedFlow) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.m[sf.id] = sf
}

func (f *flowStore) get(id string) (*storedFlow, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sf, ok := f.m[id]
	return sf, ok
}

// snapshot returns every registered flow, for the catalog listing.
func (f *flowStore) snapshot() []*storedFlow {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*storedFlow, 0, len(f.m))
	for _, sf := range f.m {
		out = append(out, sf)
	}
	return out
}

// flowID is the content hash of a flow descriptor, mirroring the model cache's
// scheme so registration is idempotent.
func flowID(desc []byte) string {
	sum := sha256.Sum256(desc)
	return fmt.Sprintf("sha256:%x", sum)
}

// loadFlows reads *.flow.json descriptors from s.flowsDir and registers each in
// the catalog (ADR-0032). It runs at construction, before the server serves, so
// it needs no locking beyond the store's own. A descriptor that does not compile
// is logged and skipped — never blocking startup — and left on disk so a later
// fix recovers it. Flows validate against the already-loaded models; a flow that
// references an unloaded model still registers, carrying diagnostics.
func (s *Server) loadFlows(ctx context.Context) {
	entries, err := os.ReadDir(s.flowsDir)
	if err != nil {
		log.Printf("temis: flow store disabled: %v", err)
		return
	}
	var loaded, skipped int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".flow.json") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(s.flowsDir, e.Name()))
		if err != nil {
			log.Printf("temis: flow %q: %v", e.Name(), err)
			skipped++
			continue
		}
		f, _, err := flow.Compile(body)
		if err != nil {
			log.Printf("temis: flow %q: %v", e.Name(), err)
			skipped++
			continue
		}
		s.flows.put(&storedFlow{id: flowID(body), flow: f, desc: body, diags: f.Validate(ctx, cacheResolver{s})})
		loaded++
	}
	msg := fmt.Sprintf("temis: flow store at %s (%d flows loaded", s.flowsDir, loaded)
	if skipped > 0 {
		msg += fmt.Sprintf(", %d skipped", skipped)
	}
	log.Print(msg + ")")
}

// cacheResolver resolves a flow's model references through the server's model
// cache (and, when configured, its on-disk store). A model must be loaded (via
// POST /v1/models or a git load) before a flow that references it can evaluate.
type cacheResolver struct{ s *Server }

func (c cacheResolver) Resolve(_ context.Context, modelID string) (*dmn.Definitions, error) {
	if sm, ok := c.s.lookup(modelID); ok {
		return sm.defs, nil
	}
	return nil, fmt.Errorf("model %q not loaded", modelID)
}

// --- DTOs ---

type flowResponse struct {
	FlowID      string              `json:"flowId"`
	Name        string              `json:"name,omitempty"`
	Diagnostics []flowDiagnosticDTO `json:"diagnostics,omitempty"`
}

type flowDiagnosticDTO struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Step    string `json:"step,omitempty"`
}

// flowSummary is one entry in the flow catalog (GET /v1/flows).
type flowSummary struct {
	FlowID string   `json:"flowId"`
	Name   string   `json:"name,omitempty"`
	Inputs []string `json:"inputs"`
	Steps  int      `json:"steps"`
}

type flowListResponse struct {
	Flows []flowSummary `json:"flows"`
	Count int           `json:"count"`
}

// flowDetail is a registered flow's drawing data (GET /v1/flows/{id}): its
// declared inputs, steps and output wiring, plus fresh validation diagnostics
// against the currently loaded models. It is what a UI renders as a graph.
type flowDetail struct {
	FlowID      string              `json:"flowId"`
	Name        string              `json:"name,omitempty"`
	Version     string              `json:"version,omitempty"`
	Inputs      []flow.InputDecl    `json:"inputs,omitempty"`
	Steps       []flow.Step         `json:"steps"`
	Output      map[string]string   `json:"output,omitempty"`
	Diagnostics []flowDiagnosticDTO `json:"diagnostics,omitempty"`
}

type evaluateFlowRequest struct {
	Input   map[string]any `json:"input"`
	Explain bool           `json:"explain,omitempty"`
}

type evaluateFlowStatelessRequest struct {
	Flow    json.RawMessage `json:"flow"`
	Input   map[string]any  `json:"input"`
	Explain bool            `json:"explain,omitempty"`
}

// --- handlers ---

// handleCreateFlow registers a JSON flow descriptor (the raw request body) and
// returns its content-addressed flowId. It responds 201 with any diagnostics
// from validating the flow against the currently loaded models (unresolved
// models are reported but do not block registration — they may be loaded later).
// Malformed JSON is a 400.
func (s *Server) handleCreateFlow(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(w, r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	f, _, err := flow.Compile(body)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "FLOW_MALFORMED", err.Error())
		return
	}
	diags := f.Validate(r.Context(), cacheResolver{s})
	id := flowID(body)
	s.flows.put(&storedFlow{id: id, flow: f, desc: body, diags: diags})
	writeJSON(w, http.StatusCreated, flowResponse{
		FlowID:      id,
		Name:        f.Name(),
		Diagnostics: toFlowDiagnosticDTOs(diags),
	})
}

// handleListFlows returns the catalog of registered flows (WP-96): each flow's
// id, name, declared input names and step count. It is the entry point a UI uses
// to list what can be run.
func (s *Server) handleListFlows(w http.ResponseWriter, _ *http.Request) {
	sfs := s.flows.snapshot()
	out := make([]flowSummary, 0, len(sfs))
	for _, sf := range sfs {
		var d flow.Descriptor
		_ = json.Unmarshal(sf.desc, &d)
		names := make([]string, 0, len(d.Inputs))
		for _, in := range d.Inputs {
			names = append(names, in.Name)
		}
		out = append(out, flowSummary{FlowID: sf.id, Name: sf.flow.Name(), Inputs: names, Steps: len(d.Steps)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FlowID < out[j].FlowID })
	writeJSON(w, http.StatusOK, flowListResponse{Flows: out, Count: len(out)})
}

// handleGetFlow returns a registered flow's steps, inputs and output wiring (the
// graph a UI draws), plus fresh validation diagnostics against the loaded models.
func (s *Server) handleGetFlow(w http.ResponseWriter, r *http.Request) {
	sf, ok := s.flows.get(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "FLOW_NOT_FOUND", "no flow with that id")
		return
	}
	var d flow.Descriptor
	_ = json.Unmarshal(sf.desc, &d)
	diags := sf.flow.Validate(r.Context(), cacheResolver{s})
	writeJSON(w, http.StatusOK, flowDetail{
		FlowID:      sf.id,
		Name:        d.Flow,
		Version:     d.Version,
		Inputs:      d.Inputs,
		Steps:       d.Steps,
		Output:      d.Output,
		Diagnostics: toFlowDiagnosticDTOs(diags),
	})
}

// handleEvaluateFlow evaluates a previously registered flow by id.
func (s *Server) handleEvaluateFlow(w http.ResponseWriter, r *http.Request) {
	sf, ok := s.flows.get(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "FLOW_NOT_FOUND", "no flow with that id")
		return
	}
	var req evaluateFlowRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	s.evaluateFlow(w, r.Context(), sf.flow, sf.desc, req.Input, req.Explain)
}

// handleEvaluateFlowStateless compiles and evaluates an inline flow descriptor in
// one request, without registering it.
func (s *Server) handleEvaluateFlowStateless(w http.ResponseWriter, r *http.Request) {
	var req evaluateFlowStatelessRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if len(req.Flow) == 0 {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", "missing flow")
		return
	}
	f, _, err := flow.Compile(req.Flow)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "FLOW_MALFORMED", err.Error())
		return
	}
	s.evaluateFlow(w, r.Context(), f, req.Flow, req.Input, req.Explain)
}

// evaluateFlow runs a compiled flow against the server's models and writes the
// result or an appropriate problem. A flow that is not sound (bad wiring,
// unresolved model, unknown decision) is a 422 with the structured diagnostics.
func (s *Server) evaluateFlow(w http.ResponseWriter, ctx context.Context, f *flow.Flow, desc []byte, input map[string]any, explain bool) {
	var opts []flow.Option
	if explain {
		opts = append(opts, flow.WithTrace())
	}
	res, err := f.Evaluate(ctx, dmn.Input(input), cacheResolver{s}, opts...)
	if err != nil {
		var fe *flow.Error
		if errors.As(err, &fe) {
			writeProblemDetail(w, problem{
				Title:        http.StatusText(http.StatusUnprocessableEntity),
				Status:       http.StatusUnprocessableEntity,
				Detail:       "flow is not valid or could not be evaluated",
				Code:         "FLOW_INVALID",
				FlowProblems: toFlowDiagnosticDTOs(fe.Diagnostics),
			})
			return
		}
		writeProblem(w, http.StatusUnprocessableEntity, "FLOW_EVALUATION_FAILED", err.Error())
		return
	}
	// Audit the flow before answering (WP-93). Fail-closed aborts the request; in
	// best-effort mode RecordFlow logs and returns nil so the result still flows.
	if s.sink != nil {
		if err := s.sink.RecordFlow(ctx, flowRecordFrom(desc, input, res.Outputs, authKidFromContext(ctx))); err != nil {
			writeProblem(w, http.StatusBadGateway, "AUDIT_WRITE_FAILED", err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, evaluateResponse{
		Outputs:   res.Outputs,
		Decisions: res.Decisions,
		Trace:     res.Trace,
	})
}

// flowRecordFrom builds a clio FlowRecord from the descriptor bytes and the
// evaluation's input/outputs. The descriptor is parsed for the flow name, version
// and ordered step modelIds; it is also carried verbatim so a re-audit can replay.
func flowRecordFrom(desc []byte, input, outputs map[string]any, authKid string) FlowRecord {
	var d flow.Descriptor
	_ = json.Unmarshal(desc, &d)
	models := make([]string, 0, len(d.Steps))
	for _, st := range d.Steps {
		models = append(models, st.Model)
	}
	return FlowRecord{
		FlowID:     flowID(desc),
		Name:       d.Flow,
		Version:    d.Version,
		Models:     models,
		Descriptor: desc,
		Input:      input,
		Outputs:    outputs,
		AuthKid:    authKid,
	}
}

func toFlowDiagnosticDTOs(diags flow.Diagnostics) []flowDiagnosticDTO {
	if len(diags) == 0 {
		return nil
	}
	out := make([]flowDiagnosticDTO, len(diags))
	for i, d := range diags {
		out[i] = flowDiagnosticDTO{Code: d.Code, Message: d.Message, Step: d.Step}
	}
	return out
}
