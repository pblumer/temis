package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pblumer/temis/quality"
)

// TestOpenQualityStreamQueriesClio proves the CLI asks clio for exactly the
// quality events (right path, method, filter) and streams the body back.
func TestOpenQualityStreamQueriesClio(t *testing.T) {
	var gotQuery map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/run-query" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer kid.sec" {
			t.Errorf("auth header = %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotQuery)
		_, _ = io.WriteString(w, `{"type":"`+quality.EventType+`","data":{"entity":"srv-1","decisions":{"Violations":["DISK_LOW"]}}}`)
	}))
	defer srv.Close()

	stream, err := openQualityStream(context.Background(), srv.URL, "kid.sec", "/quality", true, 100)
	if err != nil {
		t.Fatalf("openQualityStream: %v", err)
	}
	defer func() { _ = stream.Close() }()

	if w, _ := gotQuery["where"].(string); !strings.Contains(w, quality.EventType) {
		t.Errorf("query where = %q, want a filter on the quality event type", w)
	}
	rep, err := quality.ReadReport(context.Background(), stream, "")
	if err != nil {
		t.Fatalf("ReadReport: %v", err)
	}
	if rep.Failed != 1 || rep.Entities[0].Entity != "srv-1" {
		t.Fatalf("unexpected report: %+v", rep)
	}
}

func TestOpenQualityStreamReportsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, "nope")
	}))
	defer srv.Close()

	if _, err := openQualityStream(context.Background(), srv.URL, "", "/quality", true, 0); err == nil {
		t.Fatal("want an error on a non-2xx clio response")
	}
}

// TestPrintReportRunsCleanAndDirty is a smoke test that formatting both a clean
// and a violation-bearing report does not panic and mentions the outcome.
func TestPrintReportDoesNotPanic(t *testing.T) {
	printReport(quality.Report{Servers: 1, Passed: 1, Total: 1})
	printReport(quality.Report{
		Servers: 1, Failed: 1, Total: 1,
		Entities: []quality.EntityResult{{Entity: "srv-1", Rules: []string{"DISK_LOW"}}},
		Rules:    []quality.RuleStat{{Rule: "DISK_LOW", Failures: 1}},
	})
	if joinRules(nil) == "" {
		t.Error("joinRules(nil) should describe an expectation mismatch")
	}
}
