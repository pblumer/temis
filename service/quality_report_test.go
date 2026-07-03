package service

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pblumer/temis/quality"
)

// qualityClio is a clio stub that answers run-query with a fixed NDJSON body and
// records the query it was asked, so a test can assert both the request and the
// aggregated report.
type qualityClio struct {
	body       string
	status     int
	gotWhere   string
	gotSubject string
}

func (q *qualityClio) start(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/run-query" {
			t.Errorf("unexpected clio path %q", r.URL.Path)
		}
		var body struct {
			Subject string `json:"subject"`
			Where   string `json:"where"`
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		q.gotWhere = body.Where
		q.gotSubject = body.Subject
		if q.status != 0 {
			w.WriteHeader(q.status)
			_, _ = io.WriteString(w, "boom")
			return
		}
		_, _ = io.WriteString(w, q.body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func reportServer(t *testing.T, q *qualityClio) http.Handler {
	t.Helper()
	stub := q.start(t)
	sink, err := NewClioSink(ClioConfig{URL: stub.URL, Token: "kid_t.secret"})
	if err != nil {
		t.Fatalf("NewClioSink: %v", err)
	}
	return NewServer(nil, WithClioSink(sink)).Handler()
}

func TestQualityReportEndpointAggregates(t *testing.T) {
	q := &qualityClio{body: strings.Join([]string{
		`{"type":"` + quality.EventType + `","data":{"entity":"srv-1","decisions":{"Violations":["PATCH_OUTDATED","DISK_LOW"]}}}`,
		`{"type":"` + quality.EventType + `","data":{"entity":"srv-2","decisions":{"Violations":[]}}}`,
		`{"type":"` + quality.EventType + `","data":{"entity":"srv-3","decisions":{"Violations":["PATCH_OUTDATED"]}}}`,
	}, "\n")}
	h := reportServer(t, q)

	rec := do(t, h, "GET", "/v1/quality/report?limit=500", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
	rep := decode[quality.Report](t, rec)
	if rep.Servers != 3 || rep.Passed != 1 || rep.Failed != 2 {
		t.Fatalf("report = %+v, want 3 servers / 1 passed / 2 failed", rep)
	}
	if len(rep.Rules) == 0 || rep.Rules[0].Rule != "PATCH_OUTDATED" || rep.Rules[0].Failures != 2 {
		t.Errorf("rule tally = %+v, want PATCH_OUTDATED×2 first", rep.Rules)
	}
	if !strings.Contains(q.gotWhere, quality.EventType) {
		t.Errorf("clio query where = %q, want a quality-event filter", q.gotWhere)
	}
	if q.gotSubject != "/quality" {
		t.Errorf("default subject = %q, want /quality", q.gotSubject)
	}
}

func TestQualityReportEndpointHonoursSubject(t *testing.T) {
	q := &qualityClio{body: ""}
	h := reportServer(t, q)
	if rec := do(t, h, "GET", "/v1/quality/report?subject=/quality/prod", "", nil); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if q.gotSubject != "/quality/prod" {
		t.Errorf("subject = %q, want /quality/prod", q.gotSubject)
	}
}

func TestQualityReportEndpointRejectsBadLimit(t *testing.T) {
	q := &qualityClio{body: ""}
	h := reportServer(t, q)
	if rec := do(t, h, "GET", "/v1/quality/report?limit=-3", "", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 on a negative limit", rec.Code)
	}
}

func TestQualityReportEndpoint409WithoutSink(t *testing.T) {
	h := NewServer(nil).Handler() // no clio sink
	rec := do(t, h, "GET", "/v1/quality/report", "", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 when clio is not configured", rec.Code)
	}
}

func TestQualityReportEndpoint502OnClioError(t *testing.T) {
	q := &qualityClio{status: http.StatusForbidden}
	h := reportServer(t, q)
	rec := do(t, h, "GET", "/v1/quality/report", "", nil)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 when clio query fails", rec.Code)
	}
}
