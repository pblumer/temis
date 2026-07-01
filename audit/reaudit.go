// Package audit re-evaluates the decision events temis writes to clio (ADR-0023,
// WP-54) and verifies that each one still reproduces — the determinism proof a
// hash chain alone cannot give. clio's chain proves a record was not altered;
// re-running the recorded input against the recorded model proves the decision
// was rule-conformant.
//
// It is a read-only consumer over the public dmn package (ADR-0011): it imports
// no internal/ package and never writes back to clio. The event stream and the
// model bytes are supplied by the caller (the temis-reaudit command wires clio
// and a model directory), so the core is trivially testable.
package audit

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pblumer/temis/dmn"
	"github.com/pblumer/temis/flow"
)

// DecisionEventType is the CloudEvents type of the decision events this tool
// re-audits. It mirrors service.DecisionEventType — the wire contract from
// ADR-0023 — kept here so the package does not import the HTTP server.
const DecisionEventType = "com.temis.decision.evaluated.v1"

// FlowEventType is the CloudEvents type of the decision-flow events this tool
// re-audits (WP-93, ADR-0026). It mirrors service.FlowEventType. A flow event
// carries the flow descriptor, so replay recompiles it and resolves each step's
// model from the same ModelSource used for decision events.
const FlowEventType = "com.temis.flow.evaluated.v1"

// Status is the verdict for one re-audited event.
type Status string

const (
	// Reproduced means re-evaluating the recorded input against the recorded
	// model yielded exactly the recorded outputs.
	Reproduced Status = "reproduced"
	// Discrepancy means the re-evaluation succeeded but produced different
	// outputs than were recorded — the decision is not reproducible.
	Discrepancy Status = "discrepancy"
	// ModelUnavailable means the recorded modelId was not found in the model
	// source, so the decision could not be re-evaluated (inconclusive).
	ModelUnavailable Status = "model_unavailable"
	// EvalError means compiling or evaluating the recorded model failed.
	EvalError Status = "eval_error"
)

// Outcome is the result of re-auditing a single decision event.
type Outcome struct {
	EventID  string `json:"eventId,omitempty"`
	Subject  string `json:"subject,omitempty"`
	Decision string `json:"decision"`
	ModelID  string `json:"modelId"`
	Status   Status `json:"status"`
	// Detail explains a non-reproduced outcome (the mismatch or the error); it
	// is empty for Reproduced.
	Detail string `json:"detail,omitempty"`
}

// Reproduced reports whether every audited event reproduced exactly.
func (r Report) Reproduced() bool {
	return r.Discrepancies == 0 && r.EvalErrors == 0 && r.Unavailable == 0
}

// Report is the summary of a re-audit run.
type Report struct {
	Total         int `json:"total"`
	OK            int `json:"reproduced"`
	Discrepancies int `json:"discrepancies"`
	Unavailable   int `json:"modelUnavailable"`
	EvalErrors    int `json:"evalErrors"`
	// Outcomes holds the non-reproduced outcomes (discrepancies, errors, missing
	// models). Reproduced events are counted but not retained, so a clean audit
	// of a large history stays small.
	Outcomes []Outcome `json:"outcomes,omitempty"`
}

// ModelSource resolves a content-addressed modelId to the DMN XML that produced
// it. A re-audit can only verify decisions whose model it can resolve.
type ModelSource interface {
	// Model returns the DMN XML for modelID and whether it was found.
	Model(modelID string) ([]byte, bool)
}

// cloudEvent is the slice of a clio CloudEvent this tool reads: the envelope
// fields plus the decision data payload (ADR-0023).
type cloudEvent struct {
	ID      string       `json:"id"`
	Subject string       `json:"subject"`
	Type    string       `json:"type"`
	Data    decisionData `json:"data"`
}

type decisionData struct {
	ModelID  string         `json:"modelId"`
	Decision string         `json:"decision"`
	Input    map[string]any `json:"input"`
	Outputs  map[string]any `json:"outputs"`
}

// flowCloudEvent is the slice of a clio flow event this tool reads.
type flowCloudEvent struct {
	ID      string
	Subject string
	Data    flowData
}

type flowData struct {
	FlowID     string          `json:"flowId"`
	Flow       string          `json:"flow"`
	Descriptor json.RawMessage `json:"descriptor"`
	Input      map[string]any  `json:"input"`
	Outputs    map[string]any  `json:"outputs"`
}

// ReAudit reads decision events as NDJSON (or any whitespace-separated JSON) from
// r, re-evaluates each against the model resolved from models, and returns a
// report. Events whose type is not DecisionEventType are ignored, so the caller
// may point it at a broader stream. A decode error on the stream is returned;
// per-event problems (missing model, eval error, mismatch) are recorded as
// outcomes and never abort the run.
func ReAudit(ctx context.Context, eng *dmn.Engine, r io.Reader, models ModelSource) (Report, error) {
	if eng == nil {
		eng = dmn.New()
	}
	compiled := make(map[string]*dmn.Definitions) // modelId -> defs, compiled once
	var rep Report

	dec := json.NewDecoder(r)
	for {
		var raw struct {
			ID      string          `json:"id"`
			Subject string          `json:"subject"`
			Type    string          `json:"type"`
			Data    json.RawMessage `json:"data"`
		}
		if err := dec.Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return rep, fmt.Errorf("decode event stream: %w", err)
		}
		switch raw.Type {
		case "", DecisionEventType:
			var d decisionData
			if len(raw.Data) > 0 {
				if err := json.Unmarshal(raw.Data, &d); err != nil {
					return rep, fmt.Errorf("decode decision data: %w", err)
				}
			}
			rep.add(verify(ctx, eng, cloudEvent{ID: raw.ID, Subject: raw.Subject, Type: raw.Type, Data: d}, models, compiled))
		case FlowEventType:
			var d flowData
			if len(raw.Data) > 0 {
				if err := json.Unmarshal(raw.Data, &d); err != nil {
					return rep, fmt.Errorf("decode flow data: %w", err)
				}
			}
			rep.add(verifyFlow(ctx, eng, flowCloudEvent{ID: raw.ID, Subject: raw.Subject, Data: d}, models, compiled))
		default:
			continue
		}
	}
	return rep, nil
}

// add folds one outcome into the report, retaining only non-reproduced ones.
func (r *Report) add(o Outcome) {
	r.Total++
	switch o.Status {
	case Reproduced:
		r.OK++
		return
	case Discrepancy:
		r.Discrepancies++
	case ModelUnavailable:
		r.Unavailable++
	case EvalError:
		r.EvalErrors++
	}
	r.Outcomes = append(r.Outcomes, o)
}

// verify re-evaluates one event and classifies the result.
func verify(ctx context.Context, eng *dmn.Engine, ev cloudEvent, models ModelSource, compiled map[string]*dmn.Definitions) Outcome {
	out := Outcome{EventID: ev.ID, Subject: ev.Subject, Decision: ev.Data.Decision, ModelID: ev.Data.ModelID}

	defs, ok := compiled[ev.Data.ModelID]
	if !ok {
		xml, found := models.Model(ev.Data.ModelID)
		if !found {
			out.Status = ModelUnavailable
			out.Detail = "model not found in source"
			return out
		}
		d, _, err := eng.Compile(ctx, xml)
		if err != nil {
			out.Status = EvalError
			out.Detail = "compile: " + err.Error()
			return out
		}
		defs = d
		compiled[ev.Data.ModelID] = defs
	}

	dc, err := defs.Decision(ev.Data.Decision)
	if err != nil {
		out.Status = EvalError
		out.Detail = "decision: " + err.Error()
		return out
	}
	res, err := dc.Evaluate(ctx, dmn.Input(ev.Data.Input))
	if err != nil {
		out.Status = EvalError
		out.Detail = "evaluate: " + err.Error()
		return out
	}

	if same, detail := outputsEqual(ev.Data.Outputs, res.Outputs); !same {
		out.Status = Discrepancy
		out.Detail = detail
		return out
	}
	out.Status = Reproduced
	return out
}

// verifyFlow re-evaluates one flow event and classifies the result. It recompiles
// the recorded descriptor and resolves each step's model from the same source used
// for decision events, so a flow replays deterministically end to end.
func verifyFlow(ctx context.Context, eng *dmn.Engine, ev flowCloudEvent, models ModelSource, compiled map[string]*dmn.Definitions) Outcome {
	out := Outcome{EventID: ev.ID, Subject: ev.Subject, Decision: ev.Data.Flow, ModelID: ev.Data.FlowID}

	f, _, err := flow.Compile(ev.Data.Descriptor)
	if err != nil {
		out.Status = EvalError
		out.Detail = "compile flow: " + err.Error()
		return out
	}
	res, err := f.Evaluate(ctx, dmn.Input(ev.Data.Input), &sourceResolver{eng: eng, models: models, compiled: compiled})
	if err != nil {
		// A missing model surfaces as a flow diagnostic; classify it as
		// inconclusive (model_unavailable) rather than a reproduction failure.
		var fe *flow.Error
		if errors.As(err, &fe) {
			for _, d := range fe.Diagnostics {
				if d.Code == flow.CodeModelUnresolved {
					out.Status = ModelUnavailable
					out.Detail = d.Message
					return out
				}
			}
		}
		out.Status = EvalError
		out.Detail = "evaluate flow: " + err.Error()
		return out
	}
	if same, detail := outputsEqual(ev.Data.Outputs, res.Outputs); !same {
		out.Status = Discrepancy
		out.Detail = detail
		return out
	}
	out.Status = Reproduced
	return out
}

// sourceResolver is a flow.Resolver backed by a ModelSource: it compiles each
// referenced model on demand and caches it in the shared compiled map (also used
// by decision re-audit), so a model is compiled once per run.
type sourceResolver struct {
	eng      *dmn.Engine
	models   ModelSource
	compiled map[string]*dmn.Definitions
}

func (r *sourceResolver) Resolve(ctx context.Context, modelID string) (*dmn.Definitions, error) {
	if d, ok := r.compiled[modelID]; ok {
		return d, nil
	}
	xml, ok := r.models.Model(modelID)
	if !ok {
		return nil, fmt.Errorf("model %q not found in source", modelID)
	}
	d, _, err := r.eng.Compile(ctx, xml)
	if err != nil {
		return nil, err
	}
	r.compiled[modelID] = d
	return d, nil
}

// outputsEqual compares recorded and re-evaluated outputs by canonical JSON.
// encoding/json sorts map keys, and dmn renders numbers as their exact decimal
// string (ADR-0007), so two equal results always serialise identically. On a
// mismatch it returns a compact "recorded … got …" detail.
func outputsEqual(recorded, got map[string]any) (bool, string) {
	rj, err := json.Marshal(recorded)
	if err != nil {
		return false, "recorded outputs not encodable: " + err.Error()
	}
	gj, err := json.Marshal(got)
	if err != nil {
		return false, "re-evaluated outputs not encodable: " + err.Error()
	}
	if string(rj) == string(gj) {
		return true, ""
	}
	return false, fmt.Sprintf("recorded %s, got %s", rj, gj)
}

// DirModelSource is a ModelSource backed by a directory of DMN files. It hashes
// each file with the same scheme as the service's model cache
// ("sha256:" + hex SHA-256 of the bytes), so a file's id matches the modelId
// recorded in events produced from it.
type DirModelSource struct {
	byID map[string][]byte
}

// NewDirModelSource scans dir (non-recursively) for *.dmn and *.xml files and
// indexes them by content-addressed modelId. It returns an error if the
// directory cannot be read.
func NewDirModelSource(dir string) (*DirModelSource, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read model dir: %w", err)
	}
	src := &DirModelSource{byID: make(map[string][]byte)}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != ".dmn" && ext != ".xml" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}
		src.byID[ModelID(b)] = b
	}
	return src, nil
}

// Model implements ModelSource.
func (s *DirModelSource) Model(modelID string) ([]byte, bool) {
	b, ok := s.byID[modelID]
	return b, ok
}

// Len reports how many models were indexed.
func (s *DirModelSource) Len() int { return len(s.byID) }

// MapModelSource is a ModelSource backed by an in-memory id→xml map, handy for
// tests and for callers that resolve models another way.
type MapModelSource map[string][]byte

// Model implements ModelSource.
func (m MapModelSource) Model(modelID string) ([]byte, bool) {
	b, ok := m[modelID]
	return b, ok
}

// ModelID is the content-addressed id of a DMN document: "sha256:" followed by
// the hex SHA-256 of its bytes. It matches the service's cache key so ids line
// up across temisd and this tool.
func ModelID(xml []byte) string {
	sum := sha256.Sum256(xml)
	return fmt.Sprintf("sha256:%x", sum)
}

// SortOutcomes orders outcomes for stable reporting: by status, then decision,
// then subject.
func SortOutcomes(os []Outcome) {
	sort.SliceStable(os, func(i, j int) bool {
		if os[i].Status != os[j].Status {
			return os[i].Status < os[j].Status
		}
		if os[i].Decision != os[j].Decision {
			return os[i].Decision < os[j].Decision
		}
		return os[i].Subject < os[j].Subject
	})
}
