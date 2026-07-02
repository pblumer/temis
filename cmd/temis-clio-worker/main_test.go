package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/pblumer/temis/audit"
	"github.com/pblumer/temis/consume"
	"github.com/pblumer/temis/dmn"
)

// fakeClio stands in for a clio instance: run-query returns a fixed NDJSON
// backlog of command events, and write-events records what the worker writes back
// (with a configurable status so we can assert 409 = no-op is tolerated).
type fakeClio struct {
	mu          sync.Mutex
	backlog     string // NDJSON command events returned by run-query
	writes      []writeRequest
	writeStatus int
}

func (f *fakeClio) start(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/run-query":
			_, _ = io.WriteString(w, f.backlog)
		case "/api/v1/write-events":
			raw, _ := io.ReadAll(r.Body)
			var req writeRequest
			_ = json.Unmarshal(raw, &req)
			f.mu.Lock()
			f.writes = append(f.writes, req)
			status := f.writeStatus
			f.mu.Unlock()
			if status == 0 {
				status = http.StatusOK
			}
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"ok":true}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (f *fakeClio) written() []writeRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]writeRequest, len(f.writes))
	copy(out, f.writes)
	return out
}

func commandEvent(t *testing.T, id, subject string, data map[string]any) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{"id": id, "type": consume.CommandEventType, "subject": subject, "data": data})
	if err != nil {
		t.Fatal(err)
	}
	return string(b) + "\n"
}

func newWorker(t *testing.T, baseURL string) *worker {
	t.Helper()
	xml, err := os.ReadFile("../../dmn/testdata/models/dish_15.dmn")
	if err != nil {
		t.Fatalf("read dish model: %v", err)
	}
	src := consume.MapSource{ModelsByID: map[string][]byte{audit.ModelID(xml): xml}}
	return &worker{
		client:    &http.Client{Timeout: 5 * time.Second},
		baseURL:   baseURL,
		source:    "temis-clio-worker test",
		engine:    "temisd test",
		subject:   "/",
		recursive: true,
		eng:       dmn.New(),
		src:       src,
		processed: map[string]bool{},
	}
}

func TestBackfillAnswersCommand(t *testing.T) {
	xml, _ := os.ReadFile("../../dmn/testdata/models/dish_15.dmn")
	modelID := audit.ModelID(xml)

	clio := &fakeClio{backlog: commandEvent(t, "cmd-1", "/orders/42", map[string]any{
		"modelId":  modelID,
		"decision": "Dish",
		"input":    map[string]any{"Season": "Winter", "Guest Count": 8},
	})}
	srv := clio.start(t)
	w := newWorker(t, srv.URL)

	if err := w.backfill(context.Background()); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	writes := clio.written()
	if len(writes) != 1 {
		t.Fatalf("write count = %d, want 1", len(writes))
	}
	ev := writes[0].Events[0]
	if ev.Type != consume.DecisionEventType {
		t.Errorf("type = %q, want %q", ev.Type, consume.DecisionEventType)
	}
	if ev.Subject != "/orders/42" {
		t.Errorf("subject = %q, want the command's subject", ev.Subject)
	}
	if ev.Source != "temis-clio-worker test" {
		t.Errorf("source = %q", ev.Source)
	}
	// The precondition must key on the command's requestId so a redelivery is a no-op.
	where, _ := writes[0].Preconditions[0].Payload["where"].(string)
	if want := `event.data.requestId == "cmd-1"`; where == "" || !contains(where, want) {
		t.Errorf("precondition where = %q, want it to contain %q", where, want)
	}
	// The result data must carry the decision outputs and the correlation id.
	data, _ := json.Marshal(ev.Data)
	var dd consume.DecisionData
	_ = json.Unmarshal(data, &dd)
	if dd.Outputs["Dish"] != "Roastbeef" || dd.RequestID != "cmd-1" {
		t.Errorf("result data = %+v, want Dish=Roastbeef requestId=cmd-1", dd)
	}

	// Re-running the same backlog must not process the command again (in-memory dedupe).
	if err := w.backfill(context.Background()); err != nil {
		t.Fatalf("second backfill: %v", err)
	}
	if got := len(clio.written()); got != 1 {
		t.Errorf("write count after re-backfill = %d, want still 1 (deduped)", got)
	}
}

func TestUnresolvableCommandWritesFailureEvent(t *testing.T) {
	clio := &fakeClio{backlog: commandEvent(t, "cmd-x", "/s/1", map[string]any{
		"modelId":  "sha256:missing",
		"decision": "Dish",
		"input":    map[string]any{},
	})}
	srv := clio.start(t)
	w := newWorker(t, srv.URL)

	if err := w.backfill(context.Background()); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	writes := clio.written()
	if len(writes) != 1 {
		t.Fatalf("write count = %d, want 1 (a failure event)", len(writes))
	}
	if got := writes[0].Events[0].Type; got != consume.CommandFailedType {
		t.Errorf("type = %q, want %q", got, consume.CommandFailedType)
	}
}

func TestWriteFailureLeavesCommandUnprocessed(t *testing.T) {
	xml, _ := os.ReadFile("../../dmn/testdata/models/dish_15.dmn")
	clio := &fakeClio{
		backlog: commandEvent(t, "cmd-1", "/orders/42", map[string]any{
			"modelId": audit.ModelID(xml), "decision": "Dish",
			"input": map[string]any{"Season": "Winter", "Guest Count": 8},
		}),
		writeStatus: http.StatusInternalServerError,
	}
	srv := clio.start(t)
	w := newWorker(t, srv.URL)

	// A clio write error must surface (so the caller reconnects/retries) and must
	// NOT mark the command processed.
	if err := w.backfill(context.Background()); err == nil {
		t.Fatalf("backfill: want error on clio 500")
	}
	if w.processed["cmd-1"] {
		t.Errorf("command marked processed despite write failure")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
