package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"
)

// mockQW is a qualityWriter that counts deliveries and can fail the first few
// attempts (to exercise the queue's retry).
type mockQW struct {
	mu        sync.Mutex
	got       []QualityRecord
	failFirst int
}

func (m *mockQW) RecordQuality(_ context.Context, rec QualityRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failFirst > 0 {
		m.failFirst--
		return errors.New("clio unreachable")
	}
	m.got = append(m.got, rec)
	return nil
}

func (m *mockQW) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.got)
}

// TestQualityQueueDeliversAll enqueues many events and checks Close drains them
// all to the writer (guaranteed delivery for the process lifetime).
func TestQualityQueueDeliversAll(t *testing.T) {
	w := &mockQW{}
	q := NewQualityQueue(w, QualityQueueConfig{Workers: 4, Logf: func(string, ...any) {}})
	const n = 500
	for i := 0; i < n; i++ {
		if !q.Enqueue(QualityRecord{Entity: "e", ModelID: "m"}) {
			t.Fatalf("enqueue %d refused", i)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	q.Close(ctx)
	if w.count() != n {
		t.Fatalf("delivered %d, want %d", w.count(), n)
	}
	if enq, written, dropped := q.Stats(); enq != n || written != n || dropped != 0 {
		t.Errorf("stats = (enq %d, written %d, dropped %d), want (%d, %d, 0)", enq, written, dropped, n, n)
	}
}

// TestQualityQueueRetriesUntilDelivered checks a transient clio failure is
// retried rather than dropped.
func TestQualityQueueRetriesUntilDelivered(t *testing.T) {
	w := &mockQW{failFirst: 2}
	q := NewQualityQueue(w, QualityQueueConfig{Workers: 1, Logf: func(string, ...any) {}})
	q.Enqueue(QualityRecord{Entity: "e", ModelID: "m"})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	q.Close(ctx)
	if w.count() != 1 {
		t.Fatalf("delivered %d after retries, want 1", w.count())
	}
}

// TestClioSinkRecordsQualityEventOnEntity checks writeQuality files the event on
// the entity subject (/quality/<entity>) with the violation flag set.
func TestClioSinkRecordsQualityEventOnEntity(t *testing.T) {
	clio := &captureClio{}
	stub := clio.start(t)
	sink, err := NewClioSink(ClioConfig{URL: stub.URL, Token: "kid_t.secret", Engine: "temisd test"})
	if err != nil {
		t.Fatalf("NewClioSink: %v", err)
	}
	viol := true
	if err := sink.RecordQuality(context.Background(), QualityRecord{
		ModelID:   "sha256:abc",
		ModelName: "Routing",
		Entity:    "cust-42",
		Case:      "case A",
		Input:     map[string]any{"Applicant Age": 15},
		Decisions: map[string]any{"Routing": "DECLINE"},
		Expected:  map[string]any{"Routing": "ACCEPT"},
		Violation: &viol,
	}); err != nil {
		t.Fatalf("RecordQuality: %v", err)
	}

	raws := clio.rawBodies()
	if len(raws) != 1 {
		t.Fatalf("clio writes = %d, want 1", len(raws))
	}
	var body clioQualityWriteRequest
	if err := json.Unmarshal(raws[0], &body); err != nil {
		t.Fatalf("decode quality event: %v", err)
	}
	ev := body.Events[0]
	if ev.Type != QualityEventType {
		t.Errorf("type = %q, want %q", ev.Type, QualityEventType)
	}
	if ev.Subject != "/quality/cust-42" {
		t.Errorf("subject = %q, want /quality/cust-42", ev.Subject)
	}
	if ev.Data.Entity != "cust-42" || ev.Data.Violation == nil || !*ev.Data.Violation {
		t.Errorf("data = %+v, want entity cust-42 and violation=true", ev.Data)
	}
	if ev.Data.InputHash == "" {
		t.Error("inputHash empty")
	}
}

// TestEvaluateGraphBatchProductiveRecords drives a PRODUCTIVE run through the HTTP
// endpoint: cases carry an entity + expectations, the response reports how many
// events were queued, and the queue writes one quality event per case on its
// entity with the right violation flag.
func TestEvaluateGraphBatchProductiveRecords(t *testing.T) {
	clio := &captureClio{}
	stub := clio.start(t)
	sink, err := NewClioSink(ClioConfig{URL: stub.URL, Token: "kid_t.secret"})
	if err != nil {
		t.Fatalf("NewClioSink: %v", err)
	}
	q := NewQualityQueue(sink, QualityQueueConfig{Workers: 2, Logf: func(string, ...any) {}})
	h := NewServer(nil, WithClioSink(sink), WithQualityQueue(q)).Handler()

	xml, err := os.ReadFile("../dmn/testdata/models/routing_13.dmn")
	if err != nil {
		t.Fatalf("read routing model: %v", err)
	}
	id := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", xml)).ModelID

	body := mustJSON(t, evaluateGraphBatchRequest{
		Strict: true,
		Record: true,
		Cases: []batchCase{
			{Name: "A", Entity: "cust-1", Input: map[string]any{"Applicant Age": 20}, Expect: map[string]any{"Routing": "ACCEPT"}},
			{Name: "B", Entity: "cust-2", Input: map[string]any{"Applicant Age": 15}, Expect: map[string]any{"Routing": "ACCEPT"}},
		},
	})
	rec := do(t, h, "POST", "/v1/models/"+id+"/evaluate-graph-batch", "application/json", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("productive batch = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
	res := decode[evaluateGraphBatchResponse](t, rec)
	if res.Recorded != 2 {
		t.Fatalf("recorded = %d, want 2", res.Recorded)
	}

	// Drain the queue, then inspect the events clio received.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	q.Close(ctx)

	subjects := map[string]*bool{}
	for _, raw := range clio.rawBodies() {
		var b clioQualityWriteRequest
		if err := json.Unmarshal(raw, &b); err != nil || len(b.Events) == 0 {
			t.Fatalf("decode quality event: %v", err)
		}
		subjects[b.Events[0].Subject] = b.Events[0].Data.Violation
	}
	if len(subjects) != 2 {
		t.Fatalf("distinct entity subjects = %d, want 2 (%v)", len(subjects), subjects)
	}
	if v := subjects["/quality/cust-1"]; v == nil || *v {
		t.Errorf("cust-1 violation = %v, want false (Age 20 → ACCEPT)", v)
	}
	if v := subjects["/quality/cust-2"]; v == nil || !*v {
		t.Errorf("cust-2 violation = %v, want true (Age 15 ≠ ACCEPT)", v)
	}
}

// TestEvaluateGraphAudited checks the whole-graph evaluation (the modeler's
// "Auswerten" path) records one decision event per evaluated decision to clio —
// closing the gap where only single-decision and flow evals were audited.
func TestEvaluateGraphAudited(t *testing.T) {
	clio := &captureClio{}
	h := auditServer(t, clio, nil)

	xml, err := os.ReadFile("../dmn/testdata/models/routing_13.dmn")
	if err != nil {
		t.Fatalf("read routing model: %v", err)
	}
	id := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", xml)).ModelID

	body := mustJSON(t, evaluateGraphRequest{Input: map[string]any{"Applicant Age": 20}, Explain: true, Strict: true})
	rec := do(t, h, "POST", "/v1/models/"+id+"/evaluate-graph", "application/json", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("evaluate-graph = %d, want 200 (body %s)", rec.Code, rec.Body)
	}

	// One decision event per evaluated decision (Eligibility + Routing).
	decisions := map[string]bool{}
	for _, call := range clio.calls() {
		if len(call.Events) == 0 {
			continue
		}
		ev := call.Events[0]
		if ev.Type != DecisionEventType {
			t.Errorf("event type = %q, want %q", ev.Type, DecisionEventType)
		}
		if ev.Data.InputHash == "" {
			t.Error("inputHash empty")
		}
		decisions[ev.Data.Decision] = true
	}
	if !decisions["Eligibility"] || !decisions["Routing"] {
		t.Errorf("audited decisions = %v, want Eligibility and Routing", decisions)
	}
}

// TestEvaluateGraphBatchProductiveNeedsQueue checks a productive run is refused
// with a clear code when no quality queue (clio) is configured.
func TestEvaluateGraphBatchProductiveNeedsQueue(t *testing.T) {
	h := newTestServer(t)
	xml, err := os.ReadFile("../dmn/testdata/models/routing_13.dmn")
	if err != nil {
		t.Fatalf("read routing model: %v", err)
	}
	id := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", xml)).ModelID

	body := mustJSON(t, evaluateGraphBatchRequest{
		Record: true,
		Cases:  []batchCase{{Name: "A", Input: map[string]any{"Applicant Age": 20}}},
	})
	rec := do(t, h, "POST", "/v1/models/"+id+"/evaluate-graph-batch", "application/json", body)
	if rec.Code != http.StatusConflict {
		t.Fatalf("productive run without clio = %d, want 409 (body %s)", rec.Code, rec.Body)
	}
}
