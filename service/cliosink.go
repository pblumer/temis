package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/pblumer/temis/dmn"
)

// DecisionEventType is the CloudEvents `type` of an audit event emitted for each
// evaluation (ADR-0023, docs/80-clio-decision-log.md). It is versioned: a
// breaking change to the event's data shape bumps the `.v1` suffix.
const DecisionEventType = "com.temis.decision.evaluated.v1"

// ClioConfig configures a ClioSink. Only URL is required; the rest have sensible
// defaults. See NewClioSink.
type ClioConfig struct {
	// URL is the base address of the clio instance, e.g. http://127.0.0.1:3000.
	URL string
	// Token is the clio API key (format kid.secret); empty sends no
	// Authorization header. A key scoped to write the subject subtree is
	// recommended (clio ADR-025/033), not an admin key.
	Token string
	// Source is the CloudEvents `source` stamped on every event; default
	// "temisd". Use it to identify the writing instance.
	Source string
	// SubjectPrefix is the clio subject the decision is filed under; default
	// "/decisions". A leading slash is added if missing and a trailing slash
	// trimmed.
	SubjectPrefix string
	// SubjectKey, when set, names an input field whose value becomes the entity
	// segment of the subject (e.g. SubjectKey "Order ID" → /decisions/42). Empty
	// uses the decision name instead.
	SubjectKey string
	// Engine identifies the producing engine in data.engine, e.g. "temisd v1.2.3".
	Engine string
	// Strict makes the sink fail-closed: when the write to clio fails, the
	// evaluation request is aborted (502) rather than answered normally. The
	// default (false) is best-effort: a failed write is logged and the
	// evaluation still succeeds.
	Strict bool
	// HTTPClient overrides the HTTP client used to reach clio (e.g. in tests).
	// Nil uses a client with a 5s timeout.
	HTTPClient *http.Client
	// Logf overrides where best-effort write failures are logged. Nil uses
	// log.Printf.
	Logf func(format string, args ...any)
}

// ClioSink emits a tamper-evident decision event to a clio instance after each
// evaluation (ADR-0023). It couples to clio only over its HTTP write-events API
// — no Go import of clio, no shared process — so temisd and clio stay
// independent, dependency-free binaries. The sink is safe for concurrent use.
type ClioSink struct {
	client        *http.Client
	baseURL       string
	token         string
	source        string
	subjectPrefix string
	subjectKey    string
	engine        string
	strict        bool
	logf          func(format string, args ...any)
}

// NewClioSink builds a ClioSink from cfg. It returns an error when cfg.URL is
// empty. Defaults are applied for the source, subject prefix, HTTP client and
// logger.
func NewClioSink(cfg ClioConfig) (*ClioSink, error) {
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, errors.New("clio sink: URL is required")
	}
	prefix := strings.TrimSpace(cfg.SubjectPrefix)
	if prefix == "" {
		prefix = "/decisions"
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	prefix = strings.TrimRight(prefix, "/")

	source := cfg.Source
	if source == "" {
		source = "temisd"
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	logf := cfg.Logf
	if logf == nil {
		logf = log.Printf
	}
	return &ClioSink{
		client:        client,
		baseURL:       strings.TrimRight(cfg.URL, "/"),
		token:         cfg.Token,
		source:        source,
		subjectPrefix: prefix,
		subjectKey:    cfg.SubjectKey,
		engine:        cfg.Engine,
		strict:        cfg.Strict,
		logf:          logf,
	}, nil
}

// DecisionRecord is the data the sink needs to record one evaluation. Trace is
// included only when the evaluation produced one (explain).
type DecisionRecord struct {
	ModelID  string
	Decision string
	Input    map[string]any
	Outputs  map[string]any
	Trace    *dmn.Trace
	Strict   bool
}

// Record emits rec to clio. It returns a non-nil error only when the sink is
// fail-closed (Strict) and the write failed — the caller must then abort the
// request. In best-effort mode a failed write is logged and Record returns nil,
// so the evaluation result is never withheld because of an audit problem.
func (c *ClioSink) Record(ctx context.Context, rec DecisionRecord) error {
	if err := c.write(ctx, rec); err != nil {
		if c.strict {
			return err
		}
		c.logf("temisd: clio audit sink: %v", err)
	}
	return nil
}

// clioWriteRequest mirrors clio's POST /api/v1/write-events body: a batch of
// CloudEvents plus optional preconditions checked atomically with the write.
type clioWriteRequest struct {
	Events        []clioEvent        `json:"events"`
	Preconditions []clioPrecondition `json:"preconditions,omitempty"`
}

type clioEvent struct {
	Source  string            `json:"source"`
	Subject string            `json:"subject"`
	Type    string            `json:"type"`
	Data    decisionEventData `json:"data"`
}

// decisionEventData is the versioned data payload of a decision event
// (DecisionEventType). See docs/80-clio-decision-log.md for the field contract.
type decisionEventData struct {
	ModelID   string         `json:"modelId"`
	Decision  string         `json:"decision"`
	Input     map[string]any `json:"input"`
	Outputs   map[string]any `json:"outputs"`
	Trace     *dmn.Trace     `json:"trace,omitempty"`
	Engine    string         `json:"engine,omitempty"`
	Strict    bool           `json:"strict"`
	InputHash string         `json:"inputHash"`
}

type clioPrecondition struct {
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload"`
}

// write builds the decision event and posts it to clio. A 409 (precondition
// failed) means an identical decision was already recorded and is treated as
// success — that is what makes recording idempotent under retries.
func (c *ClioSink) write(ctx context.Context, rec DecisionRecord) error {
	subject := c.subjectFor(rec.Decision, rec.Input)
	hash := inputHash(rec.ModelID, rec.Decision, rec.Input)

	body := clioWriteRequest{
		Events: []clioEvent{{
			Source:  c.source,
			Subject: subject,
			Type:    DecisionEventType,
			Data: decisionEventData{
				ModelID:   rec.ModelID,
				Decision:  rec.Decision,
				Input:     rec.Input,
				Outputs:   rec.Outputs,
				Trace:     rec.Trace,
				Engine:    c.engine,
				Strict:    rec.Strict,
				InputHash: hash,
			},
		}},
		Preconditions: []clioPrecondition{{
			Type: "isQueryResultEmpty",
			Payload: map[string]any{
				"subject": subject,
				"where":   fmt.Sprintf("event.type == %q && event.data.inputHash == %q", DecisionEventType, hash),
			},
		}},
	}

	buf, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode event: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/write-events", bytes.NewReader(buf))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("POST write-events: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	switch {
	case resp.StatusCode == http.StatusConflict:
		// Precondition failed: this exact decision is already logged (idempotent).
		return nil
	case resp.StatusCode/100 == 2:
		return nil
	default:
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("clio write-events: status %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
}

// subjectFor maps a decision and its input to the clio subject the event is
// filed under: the configured prefix plus an entity segment — the value of the
// configured SubjectKey input field, or the decision name when no key is set or
// the field is absent/blank.
func (c *ClioSink) subjectFor(decision string, input map[string]any) string {
	entity := decision
	if c.subjectKey != "" {
		if v, ok := input[c.subjectKey]; ok {
			if s := strings.TrimSpace(fmt.Sprint(v)); s != "" {
				entity = s
			}
		}
	}
	return c.subjectPrefix + "/" + entity
}

// inputHash is a stable digest over the model, decision and input — the
// idempotency key. encoding/json sorts map keys, so the same logical input
// always hashes the same regardless of field order.
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
