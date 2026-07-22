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
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
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
	// QualitySubjectPrefix is the clio subject quality events are filed under;
	// default "/quality". Quality events are written on an ENTITY (their subject is
	// this prefix plus the entity id), so reports can query violations per entity.
	QualitySubjectPrefix string
	// Engine identifies the producing engine in data.engine, e.g. "temisd v1.2.3".
	Engine string
	// Strict makes the sink fail-closed: when the write to clio fails, the
	// evaluation request is aborted (502) rather than answered normally. The
	// default (false) is best-effort: a failed write is logged and the
	// evaluation still succeeds.
	Strict bool
	// DisableAuthorship turns off stamping the authenticated key's kid as the
	// clioauthkid CloudEvents extension (WP-105). The default (false) stamps it;
	// the sink also disables it automatically if clio rejects the extension. Set
	// this to force it off up front for a clio that does not support the extension
	// and to avoid the one rejected write that the auto-fallback recovers from.
	DisableAuthorship bool
	// HTTPClient overrides the HTTP client used to reach clio (e.g. in tests).
	// Nil uses a client with a 5s timeout.
	HTTPClient *http.Client
	// Logf overrides where best-effort write failures are logged. Nil uses
	// log.Printf. Ignored when Logger is set.
	Logf func(format string, args ...any)
	// Logger, when set, receives best-effort write failures as a structured
	// slog record (system=clio, error attribute) instead of the Logf text line
	// (WP-114). Nil falls back to Logf.
	Logger *slog.Logger
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
	qualityPrefix string
	subjectKey    string
	engine        string
	strict        bool
	logf          func(format string, args ...any)
	logger        *slog.Logger

	// authorship gates stamping the clioauthkid extension (WP-105). It starts from
	// cfg.DisableAuthorship and latches off the first time clio rejects the
	// extension, so at most one write is spent discovering that clio does not
	// support it. Safe for concurrent use.
	authorship atomic.Bool

	// health tracks the outcome of real writes so the status endpoint (WP-112)
	// and the readiness probe (WP-110) can report whether audits are getting
	// through — without any extra network call. Safe for concurrent use.
	health clioHealth
}

// clioHealth accumulates the outcome of clio writes: how many succeeded, failed
// or were idempotent no-ops, and when the last success/failure happened. All
// fields are atomic; observe is allocation-free on the success path.
type clioHealth struct {
	writesOk        atomic.Uint64
	writesFailed    atomic.Uint64
	idempotentSkips atomic.Uint64
	lastOkUnix      atomic.Int64
	lastErrUnix     atomic.Int64
	lastErr         atomic.Pointer[string]
}

// observe records one write outcome: a non-nil err is a failure, otherwise
// idempotent distinguishes a 409 no-op (already logged) from a fresh write.
func (h *clioHealth) observe(idempotent bool, err error) {
	if err != nil {
		h.writesFailed.Add(1)
		h.lastErrUnix.Store(time.Now().Unix())
		msg := err.Error()
		h.lastErr.Store(&msg)
		return
	}
	if idempotent {
		h.idempotentSkips.Add(1)
	} else {
		h.writesOk.Add(1)
	}
	h.lastOkUnix.Store(time.Now().Unix())
}

// clioSnapshot is a point-in-time, secret-free view of the sink for the status
// endpoint. reachable is derived from the last observed outcome: true until a
// write fails and stays failed (no later success) — no network call needed.
type clioSnapshot struct {
	writesOk        uint64
	writesFailed    uint64
	idempotentSkips uint64
	lastOkUnix      int64
	lastErrUnix     int64
	lastErr         string
	reachable       bool
	url             string
	strict          bool
	subjectPrefix   string
	subjectKey      string
}

// snapshot returns the sink's current health and configuration for GET
// /v1/status. It never exposes the API token.
func (c *ClioSink) snapshot() clioSnapshot {
	okUnix := c.health.lastOkUnix.Load()
	errUnix := c.health.lastErrUnix.Load()
	var lastErr string
	if p := c.health.lastErr.Load(); p != nil {
		lastErr = *p
	}
	return clioSnapshot{
		writesOk:        c.health.writesOk.Load(),
		writesFailed:    c.health.writesFailed.Load(),
		idempotentSkips: c.health.idempotentSkips.Load(),
		lastOkUnix:      okUnix,
		lastErrUnix:     errUnix,
		lastErr:         lastErr,
		reachable:       errUnix == 0 || okUnix >= errUnix,
		url:             c.baseURL,
		strict:          c.strict,
		subjectPrefix:   c.subjectPrefix,
		subjectKey:      c.subjectKey,
	}
}

// Ping actively checks whether clio is reachable by issuing a GET to its health
// endpoint. It is used only by the optional active probe in the status endpoint
// (WP-112); the passive health derived from real writes needs no network call.
func (c *ClioSink) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/healthz", nil)
	if err != nil {
		return err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("GET healthz: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("clio healthz: status %d", resp.StatusCode)
	}
	return nil
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

	qualityPrefix := strings.TrimSpace(cfg.QualitySubjectPrefix)
	if qualityPrefix == "" {
		qualityPrefix = "/quality"
	}
	if !strings.HasPrefix(qualityPrefix, "/") {
		qualityPrefix = "/" + qualityPrefix
	}
	qualityPrefix = strings.TrimRight(qualityPrefix, "/")

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
	sink := &ClioSink{
		client:        client,
		baseURL:       strings.TrimRight(cfg.URL, "/"),
		token:         cfg.Token,
		source:        source,
		subjectPrefix: prefix,
		qualityPrefix: qualityPrefix,
		subjectKey:    cfg.SubjectKey,
		engine:        cfg.Engine,
		strict:        cfg.Strict,
		logf:          logf,
		logger:        cfg.Logger,
	}
	sink.authorship.Store(!cfg.DisableAuthorship)
	return sink, nil
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
	// AuthKid is the kid of the API key that authorised the evaluation, stamped on
	// the event as the clioauthkid CloudEvents extension for authorship (WP-105).
	// Empty when the API is open or the caller used the legacy token.
	AuthKid string
}

// Record emits rec to clio. It returns a non-nil error only when the sink is
// fail-closed (Strict) and the write failed — the caller must then abort the
// request. In best-effort mode a failed write is logged and Record returns nil,
// so the evaluation result is never withheld because of an audit problem.
func (c *ClioSink) Record(ctx context.Context, rec DecisionRecord) error {
	idempotent, err := c.write(ctx, rec)
	c.health.observe(idempotent, err)
	if err != nil {
		if c.strict {
			return err
		}
		if c.logger != nil {
			c.logger.Error("clio audit write failed (best-effort)",
				slog.String("system", "clio"), slog.Any("error", err))
		} else {
			c.logf("temisd: clio audit sink: %v", err)
		}
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
	// ClioAuthKid is the authorship CloudEvents extension: the kid of the key that
	// authorised the evaluation (WP-105, ADR-0028 §3 Phase 3). Omitted when unknown
	// (open API or legacy token). clio binds it into the event's hash chain.
	ClioAuthKid string `json:"clioauthkid,omitempty"`
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

// errAuthorshipRejected is returned by send when clio refuses the write because
// it does not recognise the clioauthkid authorship extension — an older clio, or
// one whose registered event schema omits the extension. The write path retries
// once without authorship and latches it off (see authorship): a lost authorship
// stamp — which clio could not store anyway — must never cost the whole audit
// event (ADR-0023, best-effort).
var errAuthorshipRejected = errors.New("clio rejected the clioauthkid authorship extension")

// warnAuthorshipDisabled logs, once, that authorship stamping has been turned off
// because clio rejected the extension. detail carries clio's own error text.
func (c *ClioSink) warnAuthorshipDisabled(detail string) {
	const msg = "clio rejected the clioauthkid authorship extension; recording audit events without authorship (upgrade clio or register the extension in its event schema to restore it, or set -clio-authorship=false to silence this)"
	if c.logger != nil {
		c.logger.Warn(msg, slog.String("system", "clio"), slog.String("detail", detail))
	} else {
		c.logf("temisd: clio audit sink: %s: %s", msg, detail)
	}
}

// postWithAuthorshipFallback marshals body and posts it. If clio rejects the
// clioauthkid extension, it latches authorship off, calls stripKid to drop the
// extension from body, and retries once — so the audit event still lands, losing
// only the authorship stamp. stripKid must zero every event's ClioAuthKid.
func (c *ClioSink) postWithAuthorshipFallback(ctx context.Context, body any, stripKid func()) (idempotent bool, err error) {
	buf, err := json.Marshal(body)
	if err != nil {
		return false, fmt.Errorf("encode event: %w", err)
	}
	idempotent, err = c.send(ctx, buf)
	if !errors.Is(err, errAuthorshipRejected) {
		return idempotent, err
	}
	if c.authorship.CompareAndSwap(true, false) {
		c.warnAuthorshipDisabled(err.Error())
	}
	stripKid()
	if buf, err = json.Marshal(body); err != nil {
		return false, fmt.Errorf("encode event: %w", err)
	}
	return c.send(ctx, buf)
}

// write builds the decision event and posts it to clio. A 409 (precondition
// failed) means an identical decision was already recorded and is reported as an
// idempotent no-op (still a success) — that is what makes recording idempotent
// under retries.
func (c *ClioSink) write(ctx context.Context, rec DecisionRecord) (idempotent bool, err error) {
	subject := c.subjectFor(rec.Decision, rec.Input)
	hash := inputHash(rec.ModelID, rec.Decision, rec.Input)

	authKid := rec.AuthKid
	if !c.authorship.Load() {
		authKid = ""
	}
	body := clioWriteRequest{
		Events: []clioEvent{{
			Source:      c.source,
			Subject:     subject,
			Type:        DecisionEventType,
			ClioAuthKid: authKid,
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

	return c.postWithAuthorshipFallback(ctx, &body, func() { body.Events[0].ClioAuthKid = "" })
}

// send POSTs a pre-marshaled write-events body to clio and maps the response. A
// 409 (precondition failed) means the event is already logged and is reported as
// an idempotent no-op (idempotent=true, err=nil) — that is what makes recording
// idempotent under retries. Shared by the decision and flow write paths.
func (c *ClioSink) send(ctx context.Context, buf []byte) (idempotent bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/write-events", bytes.NewReader(buf))
	if err != nil {
		return false, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("POST write-events: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	switch {
	case resp.StatusCode == http.StatusConflict:
		return true, nil
	case resp.StatusCode/100 == 2:
		return false, nil
	default:
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		msg := strings.TrimSpace(string(snippet))
		if resp.StatusCode == http.StatusBadRequest && strings.Contains(msg, "clioauthkid") {
			return false, fmt.Errorf("%w: %s", errAuthorshipRejected, msg)
		}
		return false, fmt.Errorf("clio write-events: status %d: %s", resp.StatusCode, msg)
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

// FlowEventType is the CloudEvents `type` of an audit event emitted for each
// decision-flow evaluation (WP-93, ADR-0026). Like the decision event it is
// versioned via the `.v1` suffix.
const FlowEventType = "com.temis.flow.evaluated.v1"

// FlowRecord is the data the sink needs to record one flow evaluation. Descriptor
// is the raw flow descriptor, carried so a re-audit can recompile and replay the
// whole flow deterministically (audit.ReAudit); Models is the ordered list of the
// steps' modelIds, for provenance.
type FlowRecord struct {
	FlowID     string
	Name       string
	Version    string
	Models     []string
	Descriptor []byte
	Input      map[string]any
	Outputs    map[string]any
	// AuthKid is the kid of the key that authorised the flow evaluation, stamped as
	// the clioauthkid extension for authorship (WP-105). Empty when unknown.
	AuthKid string
}

// RecordFlow emits a flow event to clio. Like Record, it returns a non-nil error
// only when the sink is fail-closed (Strict) and the write failed.
func (c *ClioSink) RecordFlow(ctx context.Context, rec FlowRecord) error {
	idempotent, err := c.writeFlow(ctx, rec)
	c.health.observe(idempotent, err)
	if err != nil {
		if c.strict {
			return err
		}
		c.logf("temisd: clio audit sink (flow): %v", err)
	}
	return nil
}

type clioFlowWriteRequest struct {
	Events        []clioFlowEvent    `json:"events"`
	Preconditions []clioPrecondition `json:"preconditions,omitempty"`
}

type clioFlowEvent struct {
	Source      string        `json:"source"`
	Subject     string        `json:"subject"`
	Type        string        `json:"type"`
	Data        flowEventData `json:"data"`
	ClioAuthKid string        `json:"clioauthkid,omitempty"`
}

// flowEventData is the versioned payload of a flow event (FlowEventType). The
// descriptor makes the event self-contained for replay.
type flowEventData struct {
	FlowID     string          `json:"flowId"`
	Flow       string          `json:"flow,omitempty"`
	Version    string          `json:"version,omitempty"`
	Models     []string        `json:"models"`
	Descriptor json.RawMessage `json:"descriptor"`
	Input      map[string]any  `json:"input"`
	Outputs    map[string]any  `json:"outputs"`
	Engine     string          `json:"engine,omitempty"`
	InputHash  string          `json:"inputHash"`
}

// writeFlow builds the flow event and posts it to clio, idempotent on (subject,
// inputHash) like the decision path.
func (c *ClioSink) writeFlow(ctx context.Context, rec FlowRecord) (idempotent bool, err error) {
	entity := rec.Name
	if entity == "" {
		entity = rec.FlowID
	}
	subject := c.subjectFor(entity, rec.Input)
	hash := flowInputHash(rec.FlowID, rec.Input)

	authKid := rec.AuthKid
	if !c.authorship.Load() {
		authKid = ""
	}
	body := clioFlowWriteRequest{
		Events: []clioFlowEvent{{
			Source:      c.source,
			Subject:     subject,
			Type:        FlowEventType,
			ClioAuthKid: authKid,
			Data: flowEventData{
				FlowID:     rec.FlowID,
				Flow:       rec.Name,
				Version:    rec.Version,
				Models:     rec.Models,
				Descriptor: rec.Descriptor,
				Input:      rec.Input,
				Outputs:    rec.Outputs,
				Engine:     c.engine,
				InputHash:  hash,
			},
		}},
		Preconditions: []clioPrecondition{{
			Type: "isQueryResultEmpty",
			Payload: map[string]any{
				"subject": subject,
				"where":   fmt.Sprintf("event.type == %q && event.data.inputHash == %q", FlowEventType, hash),
			},
		}},
	}

	return c.postWithAuthorshipFallback(ctx, &body, func() { body.Events[0].ClioAuthKid = "" })
}

// flowInputHash is the idempotency key for a flow evaluation: a stable digest over
// the flowId and input.
func flowInputHash(flowID string, input map[string]any) string {
	payload := struct {
		FlowID string         `json:"flowId"`
		Input  map[string]any `json:"input"`
	}{flowID, input}
	buf, _ := json.Marshal(payload)
	sum := sha256.Sum256(buf)
	return hex.EncodeToString(sum[:])
}

// QualityEventType is the CloudEvents `type` of a quality event emitted for a
// test case run against a model (Import cockpit productive run). Unlike the
// decision event, it is written ON AN ENTITY (its subject is the entity id) and
// carries a `violation` flag, so clio can report quality violations per entity.
// Versioned via the `.v1` suffix.
const QualityEventType = "com.temis.quality.evaluated.v1"

// QualityRecord is one test case's quality observation on an entity: the model it
// ran against, the entity the observation is filed on, the case's decision
// outputs and (optional) expectations, and whether they were violated. Input is
// carried for the idempotency key (a re-run of the same case+input on the same
// entity is deduplicated by clio's precondition).
type QualityRecord struct {
	ModelID   string
	ModelName string
	Entity    string
	Case      string
	Input     map[string]any
	Decisions map[string]any
	Expected  map[string]any
	// Violation is true when Expected is non-empty and some expected value did not
	// match the computed one; false when all matched; nil when the case declared no
	// expectations (a coverage observation without a pass/fail).
	Violation *bool
}

// RecordQuality writes rec to clio and returns the real error (nil on success or
// idempotent 409). Unlike Record/RecordFlow it does NOT swallow errors in
// best-effort mode: the QualityQueue owns retry/guaranteed delivery and needs the
// true outcome to decide whether to retry.
func (c *ClioSink) RecordQuality(ctx context.Context, rec QualityRecord) error {
	return c.writeQuality(ctx, rec)
}

type clioQualityWriteRequest struct {
	Events        []clioQualityEvent `json:"events"`
	Preconditions []clioPrecondition `json:"preconditions,omitempty"`
}

type clioQualityEvent struct {
	Source  string           `json:"source"`
	Subject string           `json:"subject"`
	Type    string           `json:"type"`
	Data    qualityEventData `json:"data"`
}

// qualityEventData is the versioned payload of a quality event (QualityEventType).
type qualityEventData struct {
	ModelID   string         `json:"modelId"`
	Model     string         `json:"model,omitempty"`
	Entity    string         `json:"entity"`
	Case      string         `json:"case,omitempty"`
	Input     map[string]any `json:"input"`
	Decisions map[string]any `json:"decisions"`
	Expected  map[string]any `json:"expected,omitempty"`
	Violation *bool          `json:"violation,omitempty"`
	Engine    string         `json:"engine,omitempty"`
	InputHash string         `json:"inputHash"`
}

// writeQuality builds the quality event and posts it to clio, idempotent on
// (subject, inputHash) so a retry — or a re-run of the identical case+input on the
// same entity — is deduplicated.
func (c *ClioSink) writeQuality(ctx context.Context, rec QualityRecord) error {
	entity := strings.TrimSpace(rec.Entity)
	if entity == "" {
		entity = strings.TrimSpace(rec.Case)
	}
	if entity == "" {
		entity = "unknown"
	}
	subject := c.qualityPrefix + "/" + entity
	hash := qualityHash(rec.ModelID, entity, rec.Input)

	body := clioQualityWriteRequest{
		Events: []clioQualityEvent{{
			Source:  c.source,
			Subject: subject,
			Type:    QualityEventType,
			Data: qualityEventData{
				ModelID:   rec.ModelID,
				Model:     rec.ModelName,
				Entity:    entity,
				Case:      rec.Case,
				Input:     rec.Input,
				Decisions: rec.Decisions,
				Expected:  rec.Expected,
				Violation: rec.Violation,
				Engine:    c.engine,
				InputHash: hash,
			},
		}},
		Preconditions: []clioPrecondition{{
			Type: "isQueryResultEmpty",
			Payload: map[string]any{
				"subject": subject,
				"where":   fmt.Sprintf("event.type == %q && event.data.inputHash == %q", QualityEventType, hash),
			},
		}},
	}

	buf, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode quality event: %w", err)
	}
	// send reports an idempotent 409 as (true, nil); for the quality path a
	// duplicate is a successful no-op, so we care only about the error.
	_, err = c.send(ctx, buf)
	return err
}

// QueryQuality issues a clio run-query for this sink's quality events and returns
// the NDJSON response body for the caller to stream and close. It is the READ side
// of the quality log (writeQuality is the write side): because the server holds the
// clio token, a browser never needs it — the quality report endpoint and the
// temis-quality-report CLI both read through this. subject scopes the query (empty
// uses the sink's quality prefix); recursive covers the subtree; limit caps the
// rows (0 = clio default). It uses a generous timeout independent of the short
// write budget, since a fleet-sized report streams many events.
func (c *ClioSink) QueryQuality(ctx context.Context, subject string, recursive bool, limit int) (io.ReadCloser, error) {
	if strings.TrimSpace(subject) == "" {
		subject = c.qualityPrefix
	}
	q := map[string]any{
		"subject":   subject,
		"recursive": recursive,
		"where":     fmt.Sprintf("event.type == %q", QualityEventType),
	}
	if limit > 0 {
		q["limit"] = limit
	}
	buf, _ := json.Marshal(q)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/run-query", bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("build run-query request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("run-query: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		_ = resp.Body.Close()
		return nil, fmt.Errorf("clio run-query: status %d: %s", resp.StatusCode, bytes.TrimSpace(snippet))
	}
	return resp.Body, nil
}

// qualityHash is the idempotency key for a quality observation: a stable digest
// over the model, entity and input.
func qualityHash(modelID, entity string, input map[string]any) string {
	payload := struct {
		ModelID string         `json:"modelId"`
		Entity  string         `json:"entity"`
		Input   map[string]any `json:"input"`
	}{modelID, entity, input}
	buf, _ := json.Marshal(payload)
	sum := sha256.Sum256(buf)
	return hex.EncodeToString(sum[:])
}

// --- read side: replay decision events back into the modeler (ADR-0033) ---

// defaultQueryLimit caps how many events Query returns when the caller passes 0,
// so a broad subtree query can't stream an unbounded number of events into the
// modeler. maxQueryLimit is the hard ceiling regardless of what the caller asks.
const (
	defaultQueryLimit = 200
	maxQueryLimit     = 1000
)

// ClioEvent is one decision/flow event read back from clio for replay in the
// Operate view: the envelope fields the modeler shows plus the payload a run
// needs to be replayed (the recorded input) and compared (the recorded outputs).
// Only the replay-relevant fields are kept — the browser never sees the raw
// event or the clio token (the server queries on its behalf).
type ClioEvent struct {
	ID       string         `json:"id,omitempty"`
	Subject  string         `json:"subject"`
	Type     string         `json:"type"`
	Time     string         `json:"time,omitempty"`
	ModelID  string         `json:"modelId,omitempty"`
	FlowID   string         `json:"flowId,omitempty"`
	Decision string         `json:"decision,omitempty"`
	Input    map[string]any `json:"input,omitempty"`
	Outputs  map[string]any `json:"outputs,omitempty"`
}

// clioReadEvent is the slice of a clio CloudEvent the read side parses: the
// envelope (id/subject/type/time) plus the decision/flow data payload common to
// the temis result events (ADR-0023/WP-93/ADR-0033).
type clioReadEvent struct {
	ID      string       `json:"id"`
	Subject string       `json:"subject"`
	Type    string       `json:"type"`
	Time    string       `json:"time"`
	Data    clioReadData `json:"data"`
}

type clioReadData struct {
	ModelID  string         `json:"modelId"`
	FlowID   string         `json:"flowId"`
	Decision string         `json:"decision"`
	Input    map[string]any `json:"input"`
	Outputs  map[string]any `json:"outputs"`
}

// Query reads events from clio's run-query for a subject subtree, optionally
// filtered to a single CloudEvents type, and returns the replay-relevant slice
// of each — the read side of the sink (ADR-0033). subject defaults to the sink's
// configured subject prefix when empty; recursive covers the whole subtree so an
// entity segment (…/42) is included. eventType, when set, filters server-side to
// that CloudEvents type. limit caps the result (0 → defaultQueryLimit, capped at
// maxQueryLimit). Coupling stays over clio's HTTP contract only — no clio import.
func (c *ClioSink) Query(ctx context.Context, subject, eventType string, limit int) ([]ClioEvent, error) {
	if strings.TrimSpace(subject) == "" {
		subject = c.subjectPrefix
	}
	if limit <= 0 {
		limit = defaultQueryLimit
	}
	if limit > maxQueryLimit {
		limit = maxQueryLimit
	}

	q := map[string]any{"subject": subject, "recursive": true}
	if eventType != "" {
		q["where"] = fmt.Sprintf("event.type == %q", eventType)
	}
	buf, err := json.Marshal(q)
	if err != nil {
		return nil, fmt.Errorf("encode query: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/run-query", bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POST run-query: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode/100 != 2 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("clio run-query: status %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	// clio streams the result as successive JSON events (one per line); decode
	// them in a loop, exactly like the worker's read path (temis-clio-worker).
	events := make([]ClioEvent, 0, limit)
	dec := json.NewDecoder(resp.Body)
	for len(events) < limit {
		var ev clioReadEvent
		if err := dec.Decode(&ev); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("decode event stream: %w", err)
		}
		events = append(events, ClioEvent{
			ID:       ev.ID,
			Subject:  ev.Subject,
			Type:     ev.Type,
			Time:     ev.Time,
			ModelID:  ev.Data.ModelID,
			FlowID:   ev.Data.FlowID,
			Decision: ev.Data.Decision,
			Input:    ev.Data.Input,
			Outputs:  ev.Data.Outputs,
		})
	}
	return events, nil
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
