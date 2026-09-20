package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pblumer/temis/quality"
)

// TestDurchstichServerComplianceReport is the end-to-end walk-through of the whole
// use case: run a ruleset over a large fleet of servers, record a quality event per
// server, then aggregate them into a "which server failed which rule" report.
//
// It uploads the bundled server_compliance ruleset, streams ~70 000 synthetic
// servers through the productive batch endpoint IN CHUNKS (a single request is
// capped at 8 MiB, so a real client streams a fleet in blocks — the honest limit),
// drains the guaranteed-delivery quality queue, then feeds the recorded events
// through the same quality.ReadReport the CLI and HTTP endpoint use and checks the
// per-entity and per-rule tallies match what the inputs were built to violate.
func TestDurchstichServerComplianceReport(t *testing.T) {
	const chunk = 10000
	n := 70000
	if testing.Short() {
		n = 3000
	}

	mock := &mockQW{}
	q := NewQualityQueue(mock, QualityQueueConfig{Buffer: n + 1000, Workers: 8, Logf: func(string, ...any) {}})
	h := NewServer(nil, WithQualityQueue(q)).Handler()

	xml, err := os.ReadFile("examples/server_compliance_15.dmn")
	if err != nil {
		t.Fatalf("read example: %v", err)
	}
	rec := do(t, h, "POST", "/v1/models", "application/xml", xml)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload model = %d, want 201 (body %s)", rec.Code, rec.Body)
	}
	id := decode[modelResponse](t, rec).ModelID

	// Build the fleet and the expected verdict in lockstep, so the assertions are
	// derived from the very inputs we send (no hand-maintained magic numbers).
	cases := make([]batchCase, n)
	wantFailed := 0
	wantRule := map[string]int{}
	for i := range cases {
		in := map[string]any{
			"Days Since Patch":  5,
			"TLS Version":       "1.3",
			"Disk Free Percent": 50,
			"Firewall Enabled":  true,
			"Root SSH Login":    false,
		}
		var rules []string
		if i%2 == 0 {
			in["Days Since Patch"] = 60
			rules = append(rules, "PATCH_OUTDATED")
		}
		if i%3 == 0 {
			in["TLS Version"] = "1.0"
			rules = append(rules, "TLS_OUTDATED")
		}
		if i%5 == 0 {
			in["Disk Free Percent"] = 3
			rules = append(rules, "DISK_LOW")
		}
		if i%7 == 0 {
			in["Firewall Enabled"] = false
			rules = append(rules, "FIREWALL_OFF")
		}
		if i%11 == 0 {
			in["Root SSH Login"] = true
			rules = append(rules, "ROOT_SSH_ENABLED")
		}
		cases[i] = batchCase{Entity: fmt.Sprintf("srv-%06d", i), Input: in}
		if len(rules) > 0 {
			wantFailed++
			for _, r := range rules {
				wantRule[r]++
			}
		}
	}

	// Stream the fleet in chunks; a real client cannot fit 70 000 rich rows in one
	// 8 MiB request. Each chunk records its cases independently.
	recorded := 0
	start := time.Now()
	for off := 0; off < n; off += chunk {
		end := off + chunk
		if end > n {
			end = n
		}
		body := mustJSON(t, evaluateGraphBatchRequest{Cases: cases[off:end], Record: true})
		if len(body) > maxBodyBytes {
			t.Fatalf("chunk [%d:%d] is %d bytes, over the %d cap — lower the chunk size", off, end, len(body), maxBodyBytes)
		}
		rc := do(t, h, "POST", "/v1/models/"+id+"/evaluate-graph-batch", "application/json", body)
		if rc.Code != http.StatusOK {
			t.Fatalf("batch [%d:%d] = %d, want 200 (body %s)", off, end, rc.Code, rc.Body)
		}
		recorded += decode[evaluateGraphBatchResponse](t, rc).Recorded
	}
	elapsed := time.Since(start)
	if recorded != n {
		t.Fatalf("recorded %d cases, want %d", recorded, n)
	}

	// Drain the guaranteed-delivery queue so every quality event reaches the writer.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	q.Close(ctx)
	if mock.count() != n {
		t.Fatalf("queue delivered %d events, want %d", mock.count(), n)
	}

	// Aggregate exactly as the CLI / HTTP endpoint do: the recorded events as
	// clio-shaped NDJSON, through quality.ReadReport.
	rep, err := quality.ReadReport(context.Background(), strings.NewReader(eventsNDJSON(mock)), "")
	if err != nil {
		t.Fatalf("ReadReport: %v", err)
	}
	if rep.Servers != n {
		t.Errorf("servers = %d, want %d", rep.Servers, n)
	}
	if rep.Failed != wantFailed {
		t.Errorf("failed = %d, want %d", rep.Failed, wantFailed)
	}
	if rep.Passed != n-wantFailed {
		t.Errorf("passed = %d, want %d", rep.Passed, n-wantFailed)
	}
	gotRule := map[string]int{}
	for _, rs := range rep.Rules {
		gotRule[rs.Rule] = rs.Failures
	}
	for rule, want := range wantRule {
		if gotRule[rule] != want {
			t.Errorf("rule %s failures = %d, want %d", rule, gotRule[rule], want)
		}
	}
	t.Logf("durchstich: %d servers streamed + evaluated + recorded in %s; %d failed, top rule %v",
		n, elapsed.Round(time.Millisecond), rep.Failed, rep.Rules[0])
}

// eventsNDJSON renders the writer's captured quality records as the clio-shaped
// NDJSON quality.ReadReport consumes (only the fields it reads).
func eventsNDJSON(m *mockQW) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var b strings.Builder
	for _, r := range m.got {
		line, _ := json.Marshal(map[string]any{
			"type": quality.EventType,
			"data": map[string]any{
				"entity":    r.Entity,
				"decisions": r.Decisions,
				"violation": r.Violation,
			},
		})
		b.Write(line)
		b.WriteByte('\n')
	}
	return b.String()
}
