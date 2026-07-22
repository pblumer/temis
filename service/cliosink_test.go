package service

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// captureClio is an httptest stub standing in for a clio instance. It records
// every write-events request and replies with a configurable status.
type captureClio struct {
	mu     sync.Mutex
	reqs   []clioWriteRequest
	raws   [][]byte
	auths  []string
	paths  []string
	status int // status to return; 0 means 200
	// rejectAuthorship makes the stub behave like a clio that decodes the body
	// with DisallowUnknownFields and does not know the clioauthkid extension: any
	// write carrying it is answered with the same 400 the real clio returns.
	rejectAuthorship bool
}

func (c *captureClio) start(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req clioWriteRequest
		_ = json.Unmarshal(raw, &req)
		c.mu.Lock()
		c.paths = append(c.paths, r.URL.Path)
		c.auths = append(c.auths, r.Header.Get("Authorization"))
		c.reqs = append(c.reqs, req)
		c.raws = append(c.raws, raw)
		status := c.status
		reject := c.rejectAuthorship
		c.mu.Unlock()
		if reject && bytes.Contains(raw, []byte("clioauthkid")) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"type":"about:blank","title":"Bad Request","status":400,"detail":"ungültiger request-body: json: unknown field \"clioauthkid\""}`))
			return
		}
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (c *captureClio) calls() []clioWriteRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]clioWriteRequest, len(c.reqs))
	copy(out, c.reqs)
	return out
}

// rawBodies returns the raw request bodies captured, for decoding event shapes
// (e.g. flow events) the typed clioWriteRequest does not model.
func (c *captureClio) rawBodies() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([][]byte, len(c.raws))
	copy(out, c.raws)
	return out
}

// auditServer builds a server whose evaluations are audited to clio, returning
// the handler and the stub. cfgFn may tweak the sink config before construction.
func auditServer(t *testing.T, clio *captureClio, cfgFn func(*ClioConfig)) http.Handler {
	t.Helper()
	stub := clio.start(t)
	cfg := ClioConfig{URL: stub.URL, Token: "kid_t.secret", Engine: "temisd test"}
	if cfgFn != nil {
		cfgFn(&cfg)
	}
	sink, err := NewClioSink(cfg)
	if err != nil {
		t.Fatalf("NewClioSink: %v", err)
	}
	return NewServer(nil, WithClioSink(sink)).Handler()
}

// evalDish posts a stateless evaluation of the dish model and returns the recorder.
func evalDish(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	body := evaluateStatelessRequest{
		XML:      string(dishXML(t)),
		Decision: "Dish",
		Input:    map[string]any{"Season": "Winter", "Guest Count": 8},
	}
	raw, _ := json.Marshal(body)
	return do(t, h, "POST", "/v1/evaluate", "application/json", raw)
}

func TestClioSinkRecordsDecisionEvent(t *testing.T) {
	clio := &captureClio{}
	h := auditServer(t, clio, nil)

	rec := evalDish(t, h)
	if rec.Code != http.StatusOK {
		t.Fatalf("evaluate = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
	calls := clio.calls()
	if len(calls) != 1 {
		t.Fatalf("clio writes = %d, want 1", len(calls))
	}
	if got := clio.paths[0]; got != "/api/v1/write-events" {
		t.Errorf("path = %q, want /api/v1/write-events", got)
	}
	if got := clio.auths[0]; got != "Bearer kid_t.secret" {
		t.Errorf("auth = %q, want bearer token", got)
	}

	req := calls[0]
	if len(req.Events) != 1 {
		t.Fatalf("events = %d, want 1", len(req.Events))
	}
	ev := req.Events[0]
	if ev.Type != DecisionEventType {
		t.Errorf("type = %q, want %q", ev.Type, DecisionEventType)
	}
	if ev.Source != "temisd" {
		t.Errorf("source = %q, want temisd", ev.Source)
	}
	if ev.Subject != "/decisions/Dish" {
		t.Errorf("subject = %q, want /decisions/Dish", ev.Subject)
	}
	if ev.Data.Decision != "Dish" {
		t.Errorf("data.decision = %q, want Dish", ev.Data.Decision)
	}
	if ev.Data.Outputs["Dish"] != "Roastbeef" {
		t.Errorf("data.outputs[Dish] = %v, want Roastbeef", ev.Data.Outputs["Dish"])
	}
	if ev.Data.Engine != "temisd test" {
		t.Errorf("data.engine = %q, want temisd test", ev.Data.Engine)
	}
	if ev.Data.ModelID == "" {
		t.Error("data.modelId is empty")
	}
	if ev.Data.InputHash == "" {
		t.Error("data.inputHash is empty")
	}

	// The idempotency precondition must scope to the same subject and bind the
	// input hash, so a retry of the same decision is a no-op.
	if len(req.Preconditions) != 1 {
		t.Fatalf("preconditions = %d, want 1", len(req.Preconditions))
	}
	pc := req.Preconditions[0]
	if pc.Type != "isQueryResultEmpty" {
		t.Errorf("precondition type = %q, want isQueryResultEmpty", pc.Type)
	}
	if pc.Payload["subject"] != "/decisions/Dish" {
		t.Errorf("precondition subject = %v, want /decisions/Dish", pc.Payload["subject"])
	}
	where, _ := pc.Payload["where"].(string)
	if where == "" {
		t.Fatal("precondition where is empty")
	}
	if !strings.Contains(where, ev.Data.InputHash) {
		t.Errorf("precondition where %q does not bind inputHash %q", where, ev.Data.InputHash)
	}
	if !strings.Contains(where, DecisionEventType) {
		t.Errorf("precondition where %q does not constrain event.type", where)
	}
}

func TestClioSinkSubjectFromInputKey(t *testing.T) {
	clio := &captureClio{}
	h := auditServer(t, clio, func(c *ClioConfig) {
		c.SubjectPrefix = "/orders"
		c.SubjectKey = "Guest Count"
	})

	if rec := evalDish(t, h); rec.Code != http.StatusOK {
		t.Fatalf("evaluate = %d, want 200", rec.Code)
	}
	calls := clio.calls()
	if len(calls) != 1 {
		t.Fatalf("clio writes = %d, want 1", len(calls))
	}
	if got := calls[0].Events[0].Subject; got != "/orders/8" {
		t.Errorf("subject = %q, want /orders/8 (from input key)", got)
	}
}

func TestClioSinkBestEffortSurvivesClioFailure(t *testing.T) {
	clio := &captureClio{status: http.StatusInternalServerError}
	h := auditServer(t, clio, nil) // default: best-effort

	rec := evalDish(t, h)
	if rec.Code != http.StatusOK {
		t.Fatalf("evaluate = %d, want 200 despite clio failure (best-effort)", rec.Code)
	}
	resp := decode[evaluateResponse](t, rec)
	if resp.Outputs["Dish"] != "Roastbeef" {
		t.Errorf("outputs[Dish] = %v, want Roastbeef", resp.Outputs["Dish"])
	}
	if len(clio.calls()) != 1 {
		t.Errorf("clio writes = %d, want 1 (attempted)", len(clio.calls()))
	}
}

// TestClioSinkBestEffortLogsStructured asserts a best-effort write failure is
// emitted as a structured slog record carrying system=clio and an error
// attribute (WP-114), rather than an opaque printf line.
func TestClioSinkBestEffortLogsStructured(t *testing.T) {
	clio := &captureClio{status: http.StatusInternalServerError}
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	h := auditServer(t, clio, func(c *ClioConfig) { c.Logger = logger })

	if rec := evalDish(t, h); rec.Code != http.StatusOK {
		t.Fatalf("evaluate = %d, want 200 (best-effort)", rec.Code)
	}

	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec); err != nil {
		t.Fatalf("log line is not JSON: %v\n%s", err, buf.String())
	}
	if rec["system"] != "clio" {
		t.Errorf("log record system = %v, want clio", rec["system"])
	}
	if rec["level"] != "ERROR" {
		t.Errorf("log record level = %v, want ERROR", rec["level"])
	}
	if _, ok := rec["error"]; !ok {
		t.Errorf("log record missing error attribute: %s", buf.String())
	}
}

func TestClioSinkFailClosedAbortsOnClioFailure(t *testing.T) {
	clio := &captureClio{status: http.StatusInternalServerError}
	h := auditServer(t, clio, func(c *ClioConfig) { c.Strict = true })

	rec := evalDish(t, h)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("evaluate = %d, want 502 (fail-closed)", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("content-type = %q, want application/problem+json", ct)
	}
}

func TestClioSinkConflictIsIdempotentSuccess(t *testing.T) {
	// clio replies 409 when the precondition fails (the decision is already
	// logged). The sink must treat that as success, even fail-closed.
	clio := &captureClio{status: http.StatusConflict}
	h := auditServer(t, clio, func(c *ClioConfig) { c.Strict = true })

	rec := evalDish(t, h)
	if rec.Code != http.StatusOK {
		t.Fatalf("evaluate = %d, want 200 (409 precondition = already recorded)", rec.Code)
	}
}

func TestClioSinkInputHashStableAcrossFieldOrder(t *testing.T) {
	a := inputHash("sha256:abc", "Dish", map[string]any{"Season": "Winter", "Guest Count": 8})
	b := inputHash("sha256:abc", "Dish", map[string]any{"Guest Count": 8, "Season": "Winter"})
	if a != b {
		t.Errorf("inputHash not order-stable: %q != %q", a, b)
	}
	if c := inputHash("sha256:abc", "Dish", map[string]any{"Season": "Summer", "Guest Count": 8}); c == a {
		t.Error("inputHash collided for different input")
	}
}

func TestNewClioSinkRequiresURL(t *testing.T) {
	if _, err := NewClioSink(ClioConfig{}); err == nil {
		t.Fatal("NewClioSink with empty URL = nil error, want error")
	}
}

// TestClioSinkStampsAuthorship covers WP-105: the authenticated key's kid is
// stamped inside the decision event as data.clioauthkid.
func TestClioSinkStampsAuthorship(t *testing.T) {
	clio := &captureClio{}
	stub := clio.start(t)
	sink, err := NewClioSink(ClioConfig{URL: stub.URL, Token: "kid_t.secret", Engine: "temisd test"})
	if err != nil {
		t.Fatalf("NewClioSink: %v", err)
	}
	path := writeKeysFile(t, []scopedKey{{"agent7", "sec", []Scope{ScopeEvaluate}}})
	h := NewServer(nil, WithKeysFile(path), WithClioSink(sink)).Handler()

	body, _ := json.Marshal(evaluateStatelessRequest{
		XML:      string(dishXML(t)),
		Decision: "Dish",
		Input:    map[string]any{"Season": "Winter", "Guest Count": 8},
	})
	if rec := doAuth(t, h, "POST", "/v1/evaluate", "application/json", body, "agent7.sec"); rec.Code != http.StatusOK {
		t.Fatalf("evaluate = %d, want 200 (%s)", rec.Code, rec.Body)
	}
	calls := clio.calls()
	if len(calls) != 1 {
		t.Fatalf("clio writes = %d, want 1", len(calls))
	}
	if got := calls[0].Events[0].Data.ClioAuthKid; got != "agent7" {
		t.Errorf("clioauthkid = %q, want agent7", got)
	}
}

// TestClioSinkAuthorshipEmptyWhenOpen asserts no authorship is stamped on an open
// API (no key) — the extension is omitted rather than blank-filled.
func TestClioSinkAuthorshipEmptyWhenOpen(t *testing.T) {
	clio := &captureClio{}
	h := auditServer(t, clio, nil) // open API, no keys
	if rec := evalDish(t, h); rec.Code != http.StatusOK {
		t.Fatalf("evaluate = %d, want 200", rec.Code)
	}
	if got := clio.calls()[0].Events[0].Data.ClioAuthKid; got != "" {
		t.Errorf("clioauthkid = %q, want empty on open API", got)
	}
	// And the wire form omits the field entirely.
	if strings.Contains(string(clio.rawBodies()[0]), "clioauthkid") {
		t.Error("clioauthkid must be omitted from the wire when unknown")
	}
}

// TestClioSinkFallsBackWhenAuthorshipRejected covers the connection-lost fix: a
// clio that rejects the clioauthkid extension (400) must not lose the audit
// event. The sink retries the write without authorship, latches it off, and
// reports the write as succeeded — so the status goes green, not red.
func TestClioSinkFallsBackWhenAuthorshipRejected(t *testing.T) {
	clio := &captureClio{rejectAuthorship: true}
	stub := clio.start(t)
	sink, err := NewClioSink(ClioConfig{URL: stub.URL, Token: "kid_t.secret", Engine: "temisd test"})
	if err != nil {
		t.Fatalf("NewClioSink: %v", err)
	}
	path := writeKeysFile(t, []scopedKey{{"agent7", "sec", []Scope{ScopeEvaluate}}})
	h := NewServer(nil, WithKeysFile(path), WithClioSink(sink)).Handler()

	winter, _ := json.Marshal(evaluateStatelessRequest{
		XML:      string(dishXML(t)),
		Decision: "Dish",
		Input:    map[string]any{"Season": "Winter", "Guest Count": 8},
	})
	if rec := doAuth(t, h, "POST", "/v1/evaluate", "application/json", winter, "agent7.sec"); rec.Code != http.StatusOK {
		t.Fatalf("evaluate = %d, want 200 (%s)", rec.Code, rec.Body)
	}

	// The first attempt carries clioauthkid and is rejected; the retry omits it
	// and lands — two writes for one evaluation.
	raws := clio.rawBodies()
	if len(raws) != 2 {
		t.Fatalf("clio writes = %d, want 2 (rejected + retry)", len(raws))
	}
	if !strings.Contains(string(raws[0]), "clioauthkid") {
		t.Error("first write should carry the authorship extension")
	}
	if strings.Contains(string(raws[1]), "clioauthkid") {
		t.Error("retry must omit the authorship extension")
	}

	// Health is green: the event landed, so the status must not read as a failure.
	if snap := sink.snapshot(); !snap.reachable || snap.writesOk != 1 || snap.writesFailed != 0 {
		t.Errorf("snapshot reachable=%v writesOk=%d writesFailed=%d, want true/1/0",
			snap.reachable, snap.writesOk, snap.writesFailed)
	}

	// Authorship is now latched off: the next evaluation omits the field from the
	// first attempt, so it takes a single write with no retry.
	before := len(clio.rawBodies())
	summer, _ := json.Marshal(evaluateStatelessRequest{
		XML:      string(dishXML(t)),
		Decision: "Dish",
		Input:    map[string]any{"Season": "Summer", "Guest Count": 2},
	})
	if rec := doAuth(t, h, "POST", "/v1/evaluate", "application/json", summer, "agent7.sec"); rec.Code != http.StatusOK {
		t.Fatalf("2nd evaluate = %d, want 200 (%s)", rec.Code, rec.Body)
	}
	raws = clio.rawBodies()
	if n := len(raws) - before; n != 1 {
		t.Errorf("second evaluation writes = %d, want 1 (no retry after latch)", n)
	}
	if strings.Contains(string(raws[len(raws)-1]), "clioauthkid") {
		t.Error("after latch, writes must omit the authorship extension")
	}
}

// TestClioSinkAuthorshipDisabledByConfig asserts the -clio-authorship=false
// escape hatch: the extension is never stamped, so no write is ever rejected and
// no discovery retry is spent, even with an authenticated key.
func TestClioSinkAuthorshipDisabledByConfig(t *testing.T) {
	clio := &captureClio{rejectAuthorship: true}
	stub := clio.start(t)
	sink, err := NewClioSink(ClioConfig{URL: stub.URL, Token: "kid_t.secret", Engine: "temisd test", DisableAuthorship: true})
	if err != nil {
		t.Fatalf("NewClioSink: %v", err)
	}
	path := writeKeysFile(t, []scopedKey{{"agent7", "sec", []Scope{ScopeEvaluate}}})
	h := NewServer(nil, WithKeysFile(path), WithClioSink(sink)).Handler()

	body, _ := json.Marshal(evaluateStatelessRequest{
		XML:      string(dishXML(t)),
		Decision: "Dish",
		Input:    map[string]any{"Season": "Winter", "Guest Count": 8},
	})
	if rec := doAuth(t, h, "POST", "/v1/evaluate", "application/json", body, "agent7.sec"); rec.Code != http.StatusOK {
		t.Fatalf("evaluate = %d, want 200 (%s)", rec.Code, rec.Body)
	}
	raws := clio.rawBodies()
	if len(raws) != 1 {
		t.Fatalf("clio writes = %d, want 1 (no retry when authorship off)", len(raws))
	}
	if strings.Contains(string(raws[0]), "clioauthkid") {
		t.Error("disabled authorship must omit the field from the wire")
	}
}
