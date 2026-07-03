package service

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestMetricsOffByDefault: without WithMetrics both endpoints are 404, so the
// default posture is unchanged (WP-113).
func TestMetricsOffByDefault(t *testing.T) {
	h := NewServer(nil).Handler()
	for _, path := range []string{"/metrics", "/debug/vars"} {
		if rec := do(t, h, "GET", path, "", nil); rec.Code != http.StatusNotFound {
			t.Errorf("%s without WithMetrics = %d, want 404", path, rec.Code)
		}
	}
}

// TestMetricsExpvar: with metrics on, /debug/vars is valid JSON carrying the
// standard expvar memstats plus our temis counters, and an evaluation moves the
// evaluation counter.
func TestMetricsExpvar(t *testing.T) {
	h := NewServer(nil, WithMetrics(true), WithExamples()).Handler()

	rec := do(t, h, "GET", "/debug/vars", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("/debug/vars = %d (%s)", rec.Code, rec.Body)
	}
	var doc struct {
		Memstats map[string]any `json:"memstats"`
		Temis    struct {
			Evaluations struct{ Ok, Failed uint64 } `json:"evaluations"`
			Cache       struct{ Size int }          `json:"cache"`
		} `json:"temis"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("invalid expvar JSON: %v\n%s", err, rec.Body)
	}
	if doc.Memstats == nil {
		t.Error("expvar output missing the standard memstats var")
	}
	if doc.Temis.Cache.Size == 0 {
		t.Error("temis.cache.size should reflect the preloaded examples")
	}
}

// TestMetricsPrometheus: /metrics returns valid Prometheus text with the
// expected series, and the evaluation counter increments after an evaluate.
func TestMetricsPrometheus(t *testing.T) {
	h := NewServer(nil, WithMetrics(true)).Handler()

	// Seed and evaluate a model so the counter is non-zero.
	rec := do(t, h, "POST", "/v1/models", "application/xml", dishXML(t))
	if rec.Code != http.StatusCreated {
		t.Fatalf("seed model = %d (%s)", rec.Code, rec.Body)
	}
	id := decode[modelResponse](t, rec).ModelID
	ev := do(t, h, "POST", "/v1/models/"+id+"/evaluate", "application/json",
		mustJSON(t, evaluateModelRequest{Decision: "Dish", Input: map[string]any{"Season": "Winter", "Guest Count": 4}}))
	if ev.Code != http.StatusOK {
		t.Fatalf("evaluate = %d (%s)", ev.Code, ev.Body)
	}

	rec = do(t, h, "GET", "/metrics", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("/metrics = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"# TYPE temis_evaluations_total counter",
		"# TYPE temis_uptime_seconds gauge",
		"temis_cache_hits_total",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/metrics missing %q\n%s", want, body)
		}
	}
	if !strings.Contains(body, "temis_evaluations_total 1") {
		t.Errorf("evaluations_total should be 1 after one evaluate:\n%s", body)
	}
}

// TestMetricsRequiresAuditScope: with auth enabled, /metrics is gated by the
// audit scope — a reader key is 403, an audit key passes.
func TestMetricsRequiresAuditScope(t *testing.T) {
	path := writeKeysFile(t, []scopedKey{
		{"reader", "r", []Scope{ScopeModelsRead}},
		{"auditor", "au", []Scope{ScopeAudit}},
	})
	h := NewServer(nil, WithMetrics(true), WithKeysFile(path)).Handler()

	if rec := doAuth(t, h, "GET", "/metrics", "", nil, "reader.r"); rec.Code != http.StatusForbidden {
		t.Errorf("reader on /metrics = %d, want 403", rec.Code)
	}
	if rec := doAuth(t, h, "GET", "/metrics", "", nil, "auditor.au"); rec.Code != http.StatusOK {
		t.Errorf("auditor on /metrics = %d, want 200", rec.Code)
	}
}
