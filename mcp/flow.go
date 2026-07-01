package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/pblumer/temis/dmn"
	"github.com/pblumer/temis/flow"
)

// flowTools is the decision-flow slice of the tool catalogue, registered into the
// shared tools list by init so it is advertised alongside the model tools
// (WP-92, ADR-0026).
var flowTools = []toolSpec{
	{
		Name: "load_flow",
		Description: "Register a decision-flow descriptor (JSON, ADR-0026) that composes " +
			"several already-loaded models into one stateless evaluation, returning a stable " +
			"content-addressed flowId. Diagnostics from validating the flow against the loaded " +
			"models are returned; load the referenced models first with load_model.",
		InputSchema: obj(map[string]any{
			"flow": map[string]any{
				"type":        "object",
				"description": "The flow descriptor: flow name, optional inputs, steps (id, model=modelId, decision, in-wiring) and output.",
			},
		}, "flow"),
	},
	{
		Name: "describe_flow",
		Description: "Describe a registered flow: its name, declared inputs and the steps it " +
			"composes (each step's id, model and decision). Use this to learn what to pass to evaluate_flow.",
		InputSchema: obj(map[string]any{
			"flowId": str("The flowId returned by load_flow."),
		}, "flowId"),
	},
	{
		Name: "evaluate_flow",
		Description: "Evaluate a decision flow and return its outputs deterministically. Supply " +
			"either flowId (for a registered flow) or flow (an inline descriptor). Models are resolved " +
			"from this server's cache. Set explain=true to also get the aggregated decision trace.",
		InputSchema: obj(map[string]any{
			"flowId":  str("A flowId from load_flow. Provide this or flow."),
			"flow":    map[string]any{"type": "object", "description": "An inline flow descriptor to evaluate in one call. Provide this or flowId."},
			"input":   map[string]any{"type": "object", "description": "The flow's inputs: name → value."},
			"explain": map[string]any{"type": "boolean", "description": "When true, include the aggregated decision trace of every decision step."},
		}),
	},
}

func init() { tools = append(tools, flowTools...) }

// storedFlow is a registered flow together with the descriptor it was parsed
// from (kept so describe_flow can report its steps and inputs).
type storedFlow struct {
	id   string
	flow *flow.Flow
	desc flow.Descriptor
}

// flowStore holds registered flows keyed by content hash. Flows are small and
// few, so a plain guarded map suffices.
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

func flowID(desc []byte) string {
	sum := sha256.Sum256(desc)
	return fmt.Sprintf("sha256:%x", sum)
}

// storeResolver resolves a flow's model references through the MCP server's model
// store, so a model loaded over any surface (MCP or the co-located HTTP service,
// ADR-0021) is reachable by a flow.
type storeResolver struct{ store Store }

func (r storeResolver) Resolve(_ context.Context, modelID string) (*dmn.Definitions, error) {
	if defs, _, ok := r.store.Lookup(modelID); ok {
		return defs, nil
	}
	return nil, fmt.Errorf("model %q not loaded", modelID)
}

func (s *Server) toolLoadFlow(ctx context.Context, raw json.RawMessage) (any, *rpcError) {
	var a struct {
		Flow json.RawMessage `json:"flow"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return toolError("invalid arguments: " + err.Error()), nil
	}
	if len(a.Flow) == 0 {
		return toolError("missing required argument: flow"), nil
	}
	f, _, err := flow.Compile(a.Flow)
	if err != nil {
		return toolError("could not compile flow: " + err.Error()), nil
	}
	var desc flow.Descriptor
	_ = json.Unmarshal(a.Flow, &desc)
	id := flowID(a.Flow)
	s.flows.put(&storedFlow{id: id, flow: f, desc: desc})
	diags := f.Validate(ctx, storeResolver{s.store})
	return toolText(flowResponse{FlowID: id, Name: f.Name(), Diagnostics: toFlowDiagnosticDTOs(diags)})
}

func (s *Server) toolDescribeFlow(raw json.RawMessage) (any, *rpcError) {
	var a struct {
		FlowID string `json:"flowId"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return toolError("invalid arguments: " + err.Error()), nil
	}
	if a.FlowID == "" {
		return toolError("missing required argument: flowId"), nil
	}
	sf, ok := s.flows.get(a.FlowID)
	if !ok {
		return toolError("no flow with id " + a.FlowID + "; load it first with load_flow"), nil
	}
	return toolText(flowDescription{
		FlowID: sf.id,
		Name:   sf.desc.Flow,
		Inputs: sf.desc.Inputs,
		Steps:  sf.desc.Steps,
	})
}

func (s *Server) toolEvaluateFlow(ctx context.Context, raw json.RawMessage) (any, *rpcError) {
	var a struct {
		FlowID  string          `json:"flowId"`
		Flow    json.RawMessage `json:"flow"`
		Input   map[string]any  `json:"input"`
		Explain bool            `json:"explain"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return toolError("invalid arguments: " + err.Error()), nil
	}

	var f *flow.Flow
	switch {
	case a.FlowID != "":
		sf, ok := s.flows.get(a.FlowID)
		if !ok {
			return toolError("no flow with id " + a.FlowID + "; load it first with load_flow"), nil
		}
		f = sf.flow
	case len(a.Flow) > 0:
		cf, _, err := flow.Compile(a.Flow)
		if err != nil {
			return toolError("could not compile flow: " + err.Error()), nil
		}
		f = cf
	default:
		return toolError("provide either flowId or flow"), nil
	}

	var opts []flow.Option
	if a.Explain {
		opts = append(opts, flow.WithTrace())
	}
	res, err := f.Evaluate(ctx, dmn.Input(a.Input), storeResolver{s.store}, opts...)
	if err != nil {
		var fe *flow.Error
		if errors.As(err, &fe) {
			b, _ := json.MarshalIndent(map[string]any{
				"error":    "flow is not valid or could not be evaluated",
				"problems": toFlowDiagnosticDTOs(fe.Diagnostics),
			}, "", "  ")
			return map[string]any{
				"content": []any{map[string]any{"type": "text", "text": string(b)}},
				"isError": true,
			}, nil
		}
		return toolError("flow evaluation failed: " + err.Error()), nil
	}
	return toolText(evaluateResponse{Outputs: res.Outputs, Decisions: res.Decisions, Trace: res.Trace})
}

// --- DTOs ---

type flowResponse struct {
	FlowID      string              `json:"flowId"`
	Name        string              `json:"name,omitempty"`
	Diagnostics []flowDiagnosticDTO `json:"diagnostics,omitempty"`
}

type flowDescription struct {
	FlowID string           `json:"flowId"`
	Name   string           `json:"name,omitempty"`
	Inputs []flow.InputDecl `json:"inputs,omitempty"`
	Steps  []flow.Step      `json:"steps"`
}

type flowDiagnosticDTO struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Step    string `json:"step,omitempty"`
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
