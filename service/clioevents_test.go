package service

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// queryClio is an httptest stub standing in for clio's run-query endpoint. It
// records the query body it received and streams back a fixed set of events as
// successive JSON values (the NDJSON-style stream the worker/read side decode).
type queryClio struct {
	body   []byte
	events []string // each a JSON CloudEvent
	status int      // 0 means 200
}

func (q *queryClio) start(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q.body, _ = io.ReadAll(r.Body)
		if q.status != 0 {
			w.WriteHeader(q.status)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		for _, e := range q.events {
			_, _ = w.Write([]byte(e))
			_, _ = w.Write([]byte("\n"))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func decisionEventJSON(subject, decision string, input, outputs map[string]any) string {
	ev := map[string]any{
		"id":      "evt-" + decision,
		"subject": subject,
		"type":    DecisionEventType,
		"time":    "2026-07-02T10:00:00Z",
		"data": map[string]any{
			"modelId":  "sha256:abc",
			"decision": decision,
			"input":    input,
			"outputs":  outputs,
		},
	}
	b, _ := json.Marshal(ev)
	return string(b)
}

// queryServer builds a server whose sink points at the given clio stub.
func queryServer(t *testing.T, stubURL string, cfgFn func(*ClioConfig)) http.Handler {
	t.Helper()
	cfg := ClioConfig{URL: stubURL, Token: "kid_t.secret", SubjectPrefix: "/decisions", SubjectKey: "Order ID"}
	if cfgFn != nil {
		cfgFn(&cfg)
	}
	sink, err := NewClioSink(cfg)
	if err != nil {
		t.Fatalf("NewClioSink: %v", err)
	}
	return NewServer(nil, WithClioSink(sink)).Handler()
}

func TestClioEventsReadsBackDecisionEvents(t *testing.T) {
	stub := &queryClio{events: []string{
		decisionEventJSON("/decisions/42", "FinalPremium", map[string]any{"VehicleValue": 1200.0}, map[string]any{"FinalPremium": 4212.0}),
		decisionEventJSON("/decisions/42", "BasePremium", map[string]any{"VehicleValue": 1200.0}, map[string]any{"BasePremium": 324.0}),
	}}
	srv := stub.start(t)
	h := queryServer(t, srv.URL, nil)

	rec := do(t, h, "GET", "/v1/clio/events?subject=/decisions&type="+DecisionEventType+"&limit=50", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var resp clioEventsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Enabled {
		t.Fatalf("enabled = false, want true")
	}
	if len(resp.Events) != 2 {
		t.Fatalf("events = %d, want 2", len(resp.Events))
	}
	if resp.Events[0].Decision != "FinalPremium" || resp.Events[0].Type != DecisionEventType {
		t.Errorf("first event = %+v", resp.Events[0])
	}
	if got := resp.Events[0].Outputs["FinalPremium"]; got != 4212.0 {
		t.Errorf("recorded output = %v, want 4212", got)
	}
	if resp.SubjectPrefix != "/decisions" || resp.SubjectKey != "Order ID" {
		t.Errorf("mapping = %q/%q, want /decisions/Order ID", resp.SubjectPrefix, resp.SubjectKey)
	}
	// The query the server sent clio must carry the subject subtree and the type filter.
	var q map[string]any
	if err := json.Unmarshal(stub.body, &q); err != nil {
		t.Fatalf("decode query: %v", err)
	}
	if q["subject"] != "/decisions" || q["recursive"] != true {
		t.Errorf("query = %v, want subject /decisions recursive true", q)
	}
	if w, _ := q["where"].(string); !strings.Contains(w, DecisionEventType) {
		t.Errorf("where = %q, want a type filter on %s", w, DecisionEventType)
	}
}

func TestClioEventsDefaultsSubjectToPrefix(t *testing.T) {
	stub := &queryClio{events: []string{decisionEventJSON("/decisions/x", "D", nil, nil)}}
	srv := stub.start(t)
	h := queryServer(t, srv.URL, func(c *ClioConfig) { c.SubjectPrefix = "/kfz" })

	rec := do(t, h, "GET", "/v1/clio/events", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var q map[string]any
	_ = json.Unmarshal(stub.body, &q)
	if q["subject"] != "/kfz" {
		t.Errorf("subject = %v, want /kfz (the configured prefix)", q["subject"])
	}
	if _, ok := q["where"]; ok {
		t.Errorf("where present with no type filter: %v", q["where"])
	}
}

func TestClioEventsDisabledWithoutSink(t *testing.T) {
	h := NewServer(nil).Handler()
	rec := do(t, h, "GET", "/v1/clio/events", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp clioEventsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Enabled {
		t.Errorf("enabled = true, want false when no sink is configured")
	}
	if len(resp.Types) == 0 {
		t.Errorf("types empty, want the replayable event types offered even when disabled")
	}
}

func TestClioEventsBadGatewayOnClioFailure(t *testing.T) {
	stub := &queryClio{status: http.StatusInternalServerError}
	srv := stub.start(t)
	h := queryServer(t, srv.URL, nil)
	rec := do(t, h, "GET", "/v1/clio/events", "", nil)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
}
