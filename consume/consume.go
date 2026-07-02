package consume

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/pblumer/temis/audit"
	"github.com/pblumer/temis/dmn"
	"github.com/pblumer/temis/flow"
)

// CommandEventType is the CloudEvents `type` of a command that requests an
// evaluation (ADR-0033). It is versioned like the result events: a breaking
// change to the command's data shape bumps the `.v1` suffix.
const CommandEventType = "com.temis.decision.requested.v1"

// DecisionEventType and FlowEventType are the result-event types produced by a
// successful command. They mirror service.DecisionEventType / service.FlowEventType
// (the ADR-0023/WP-93 wire contract) so a command-driven result is byte-for-byte
// the same shape temisd's sink writes and is re-audited identically — they are
// re-exported from package audit, the read-side mirror, to keep the constant in
// one place across the clio-facing tools.
const (
	DecisionEventType = audit.DecisionEventType
	FlowEventType     = audit.FlowEventType
)

// CommandFailedType is the CloudEvents `type` written when a command cannot be
// evaluated (unresolved model/flow, compile or runtime error). It closes the
// loop — a command is always answered, with a result or a failure — so a
// requester can observe completion either way. Versioned via the `.v1` suffix.
const CommandFailedType = "com.temis.decision.failed.v1"

// Source resolves the artifacts a command references by content-addressed id:
// model XML by modelId and a raw flow descriptor by flowId. Both are hashed with
// the same "sha256:"+hex scheme as the server cache and package audit, so ids
// line up across temisd, temis-reaudit and this consumer.
type Source interface {
	// Model returns the DMN XML for modelID and whether it was found.
	Model(modelID string) ([]byte, bool)
	// Flow returns the raw flow descriptor for flowID and whether it was found.
	Flow(flowID string) ([]byte, bool)
}

// Command is a parsed command event: the clio envelope fields the consumer needs
// (event id and subject) plus the request payload. Exactly one of ModelID or
// FlowID selects what to evaluate; with ModelID, a non-empty Decision selects a
// single decision and an empty one the whole model graph.
type Command struct {
	// EventID is the clio event id of the command; it becomes the requestId that
	// correlates the result event(s) back to the command and is the idempotency
	// key the worker writes under.
	EventID string
	// Subject is the clio business entity the command was filed on; the result
	// events are filed under the same subject.
	Subject string

	ModelID  string
	FlowID   string
	Decision string
	Input    map[string]any
	// Explain requests the decision trace (data.trace) in the result event.
	Explain bool
}

// ResultEvent is a result CloudEvent produced from a command, ready for the
// worker to write back to clio. Type is DecisionEventType, FlowEventType or
// CommandFailedType; RequestID correlates it to the command. Decision is set for
// decision events (empty otherwise) and, together with RequestID, forms the
// idempotency key so a whole-graph command's N decision events each dedupe
// independently.
type ResultEvent struct {
	Subject   string
	Type      string
	RequestID string
	Decision  string
	// Data is the versioned data payload: *DecisionData, *FlowData or
	// *FailureData. It marshals to the same JSON `data` object temisd writes.
	Data any
}

// DecisionData is the versioned payload of a decision result event, mirroring
// service's decisionEventData plus the requestId correlation (ADR-0033).
type DecisionData struct {
	ModelID   string         `json:"modelId"`
	Decision  string         `json:"decision"`
	Input     map[string]any `json:"input"`
	Outputs   map[string]any `json:"outputs"`
	Trace     *dmn.Trace     `json:"trace,omitempty"`
	Engine    string         `json:"engine,omitempty"`
	InputHash string         `json:"inputHash"`
	RequestID string         `json:"requestId,omitempty"`
}

// FlowData is the versioned payload of a flow result event, mirroring service's
// flowEventData plus the requestId correlation (ADR-0033). The descriptor is
// carried verbatim so a re-audit can replay the flow.
type FlowData struct {
	FlowID     string          `json:"flowId"`
	Flow       string          `json:"flow,omitempty"`
	Version    string          `json:"version,omitempty"`
	Models     []string        `json:"models"`
	Descriptor json.RawMessage `json:"descriptor"`
	Input      map[string]any  `json:"input"`
	Outputs    map[string]any  `json:"outputs"`
	Engine     string          `json:"engine,omitempty"`
	InputHash  string          `json:"inputHash"`
	RequestID  string          `json:"requestId,omitempty"`
}

// FailureData is the payload of a CommandFailedType event: what was asked and why
// it could not be evaluated. It carries no outputs.
type FailureData struct {
	ModelID   string         `json:"modelId,omitempty"`
	FlowID    string         `json:"flowId,omitempty"`
	Decision  string         `json:"decision,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
	Error     string         `json:"error"`
	Engine    string         `json:"engine,omitempty"`
	RequestID string         `json:"requestId,omitempty"`
}

// rawEvent is the slice of a clio CloudEvent the consumer reads: the envelope id
// and subject plus the command data payload.
type rawEvent struct {
	ID      string      `json:"id"`
	Subject string      `json:"subject"`
	Type    string      `json:"type"`
	Data    commandData `json:"data"`
}

type commandData struct {
	ModelID  string         `json:"modelId"`
	FlowID   string         `json:"flowId"`
	Decision string         `json:"decision"`
	Input    map[string]any `json:"input"`
	Explain  bool           `json:"explain"`
}

// ParseCommand decodes one clio event and reports whether it is a command. A
// non-command event (any other type) returns ok=false with no error, so a caller
// may point ParseCommand at a broader stream and ignore the rest — the same
// tolerance package audit's ReAudit uses. A malformed JSON envelope is an error.
func ParseCommand(raw []byte) (cmd Command, ok bool, err error) {
	var ev rawEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		return Command{}, false, fmt.Errorf("decode command event: %w", err)
	}
	if ev.Type != CommandEventType {
		return Command{}, false, nil
	}
	return Command{
		EventID:  ev.ID,
		Subject:  ev.Subject,
		ModelID:  ev.Data.ModelID,
		FlowID:   ev.Data.FlowID,
		Decision: ev.Data.Decision,
		Input:    ev.Data.Input,
		Explain:  ev.Data.Explain,
	}, true, nil
}

// Handle evaluates cmd against the artifacts resolved from src and returns the
// result event(s): one flow event for a flow command, one decision event per
// decision for a whole-graph command, or a single decision event for a
// single-decision command. engine is stamped on data.engine (may be empty).
//
// A resolution, compile or runtime failure is returned as an error and no events;
// the caller turns that into a FailureEvent so the command is still answered. A
// nil eng uses a fresh dmn.New().
func Handle(ctx context.Context, eng *dmn.Engine, cmd Command, src Source, engine string) ([]ResultEvent, error) {
	if eng == nil {
		eng = dmn.New()
	}
	switch {
	case cmd.FlowID != "":
		return handleFlow(ctx, eng, cmd, src, engine)
	case cmd.ModelID != "" && cmd.Decision != "":
		return handleDecision(ctx, eng, cmd, src, engine)
	case cmd.ModelID != "":
		return handleGraph(ctx, eng, cmd, src, engine)
	default:
		return nil, errors.New("command names neither a modelId nor a flowId")
	}
}

// FailureEvent builds a CommandFailedType result event from a command and the
// error that stopped it, so the worker can record why a command was not answered
// with a result.
func FailureEvent(cmd Command, evalErr error, engine string) ResultEvent {
	return ResultEvent{
		Subject:   cmd.Subject,
		Type:      CommandFailedType,
		RequestID: cmd.EventID,
		Data: &FailureData{
			ModelID:   cmd.ModelID,
			FlowID:    cmd.FlowID,
			Decision:  cmd.Decision,
			Input:     cmd.Input,
			Error:     evalErr.Error(),
			Engine:    engine,
			RequestID: cmd.EventID,
		},
	}
}

func handleDecision(ctx context.Context, eng *dmn.Engine, cmd Command, src Source, engine string) ([]ResultEvent, error) {
	defs, err := compile(ctx, eng, src, cmd.ModelID)
	if err != nil {
		return nil, err
	}
	dc, err := defs.Decision(cmd.Decision)
	if err != nil {
		return nil, fmt.Errorf("decision %q: %w", cmd.Decision, err)
	}
	res, err := dc.Evaluate(ctx, dmn.Input(cmd.Input), evalOpts(cmd)...)
	if err != nil {
		return nil, fmt.Errorf("evaluate: %w", err)
	}
	return []ResultEvent{{
		Subject:   cmd.Subject,
		Type:      DecisionEventType,
		RequestID: cmd.EventID,
		Decision:  cmd.Decision,
		Data: &DecisionData{
			ModelID:   cmd.ModelID,
			Decision:  cmd.Decision,
			Input:     cmd.Input,
			Outputs:   res.Outputs,
			Trace:     res.Trace,
			Engine:    engine,
			InputHash: inputHash(cmd.ModelID, cmd.Decision, cmd.Input),
			RequestID: cmd.EventID,
		},
	}}, nil
}

func handleGraph(ctx context.Context, eng *dmn.Engine, cmd Command, src Source, engine string) ([]ResultEvent, error) {
	defs, err := compile(ctx, eng, src, cmd.ModelID)
	if err != nil {
		return nil, err
	}
	res, err := defs.EvaluateGraph(ctx, dmn.Input(cmd.Input), evalOpts(cmd)...)
	if err != nil {
		return nil, fmt.Errorf("evaluate graph: %w", err)
	}
	// One decision event per evaluated decision, exactly like the server's
	// evaluate-graph sink path: each decision dedupes on (requestId, decision).
	out := make([]ResultEvent, 0, len(res.Values))
	for name, val := range res.Values {
		dd := &DecisionData{
			ModelID:   cmd.ModelID,
			Decision:  name,
			Input:     cmd.Input,
			Outputs:   map[string]any{name: val},
			Engine:    engine,
			InputHash: inputHash(cmd.ModelID, name, cmd.Input),
			RequestID: cmd.EventID,
		}
		if res.Traces != nil {
			dd.Trace = res.Traces[name]
		}
		out = append(out, ResultEvent{
			Subject:   cmd.Subject,
			Type:      DecisionEventType,
			RequestID: cmd.EventID,
			Decision:  name,
			Data:      dd,
		})
	}
	return out, nil
}

func handleFlow(ctx context.Context, eng *dmn.Engine, cmd Command, src Source, engine string) ([]ResultEvent, error) {
	desc, ok := src.Flow(cmd.FlowID)
	if !ok {
		return nil, fmt.Errorf("flow %q not found in source", cmd.FlowID)
	}
	f, _, err := flow.Compile(desc)
	if err != nil {
		return nil, fmt.Errorf("compile flow: %w", err)
	}
	var opts []flow.Option
	if cmd.Explain {
		opts = append(opts, flow.WithTrace())
	}
	res, err := f.Evaluate(ctx, dmn.Input(cmd.Input), &sourceResolver{eng: eng, src: src, compiled: map[string]*dmn.Definitions{}}, opts...)
	if err != nil {
		return nil, fmt.Errorf("evaluate flow: %w", err)
	}
	var d flow.Descriptor
	_ = json.Unmarshal(desc, &d)
	models := make([]string, 0, len(d.Steps))
	for _, st := range d.Steps {
		models = append(models, st.Model)
	}
	return []ResultEvent{{
		Subject:   cmd.Subject,
		Type:      FlowEventType,
		RequestID: cmd.EventID,
		Data: &FlowData{
			FlowID:     cmd.FlowID,
			Flow:       d.Flow,
			Version:    d.Version,
			Models:     models,
			Descriptor: desc,
			Input:      cmd.Input,
			Outputs:    res.Outputs,
			Engine:     engine,
			InputHash:  flowInputHash(cmd.FlowID, cmd.Input),
			RequestID:  cmd.EventID,
		},
	}}, nil
}

// evalOpts turns a command's flags into dmn evaluation options.
func evalOpts(cmd Command) []dmn.EvalOption {
	if cmd.Explain {
		return []dmn.EvalOption{dmn.WithTrace()}
	}
	return nil
}

// compile resolves and compiles a model by id.
func compile(ctx context.Context, eng *dmn.Engine, src Source, modelID string) (*dmn.Definitions, error) {
	xml, ok := src.Model(modelID)
	if !ok {
		return nil, fmt.Errorf("model %q not found in source", modelID)
	}
	defs, _, err := eng.Compile(ctx, xml)
	if err != nil {
		return nil, fmt.Errorf("compile: %w", err)
	}
	return defs, nil
}

// sourceResolver is a flow.Resolver backed by a Source, compiling each referenced
// model once per flow evaluation.
type sourceResolver struct {
	eng      *dmn.Engine
	src      Source
	compiled map[string]*dmn.Definitions
}

func (r *sourceResolver) Resolve(ctx context.Context, modelID string) (*dmn.Definitions, error) {
	if d, ok := r.compiled[modelID]; ok {
		return d, nil
	}
	d, err := compile(ctx, r.eng, r.src, modelID)
	if err != nil {
		return nil, err
	}
	r.compiled[modelID] = d
	return d, nil
}

// inputHash mirrors service.inputHash: the idempotency digest over model,
// decision and input. It keeps command-driven result events uniform with the
// sink's, though the worker dedupes on requestId, not this hash.
func inputHash(modelID, decision string, input map[string]any) string {
	payload := struct {
		ModelID  string         `json:"modelId"`
		Decision string         `json:"decision"`
		Input    map[string]any `json:"input"`
	}{modelID, decision, input}
	buf, _ := json.Marshal(payload)
	sum := sha256.Sum256(buf)
	return hex.EncodeToString(sum[:])
}

// flowInputHash mirrors service.flowInputHash.
func flowInputHash(flowID string, input map[string]any) string {
	payload := struct {
		FlowID string         `json:"flowId"`
		Input  map[string]any `json:"input"`
	}{flowID, input}
	buf, _ := json.Marshal(payload)
	sum := sha256.Sum256(buf)
	return hex.EncodeToString(sum[:])
}
