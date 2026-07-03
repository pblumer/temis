package quality

import (
	"context"
	"strings"
	"testing"
)

// ndjson builds a whitespace-separated stream of quality events from raw JSON
// bodies, so a test reads exactly what a clio run-query would return.
func ndjson(lines ...string) string { return strings.Join(lines, "\n") }

func TestReadReportAggregatesViolationsPerEntity(t *testing.T) {
	stream := ndjson(
		// srv-1: two rules violated.
		`{"type":"`+EventType+`","data":{"entity":"srv-1","decisions":{"Violations":["PATCH_OUTDATED","TLS_OUTDATED"]}}}`,
		// srv-2: clean.
		`{"type":"`+EventType+`","data":{"entity":"srv-2","decisions":{"Violations":[]}}}`,
		// srv-3: one rule.
		`{"type":"`+EventType+`","data":{"entity":"srv-3","decisions":{"Violations":["PATCH_OUTDATED"]}}}`,
	)
	rep, err := ReadReport(context.Background(), strings.NewReader(stream), "")
	if err != nil {
		t.Fatalf("ReadReport: %v", err)
	}
	if rep.Total != 3 || rep.Servers != 3 {
		t.Fatalf("total/servers = %d/%d, want 3/3", rep.Total, rep.Servers)
	}
	if rep.Passed != 1 || rep.Failed != 2 {
		t.Fatalf("passed/failed = %d/%d, want 1/2", rep.Passed, rep.Failed)
	}
	if len(rep.Entities) != 2 {
		t.Fatalf("entities = %d, want 2 (only failing)", len(rep.Entities))
	}
	// Sorted by entity id: srv-1 then srv-3.
	if rep.Entities[0].Entity != "srv-1" || len(rep.Entities[0].Rules) != 2 {
		t.Errorf("entities[0] = %+v", rep.Entities[0])
	}
	if got := rep.Entities[0].Rules; got[0] != "PATCH_OUTDATED" || got[1] != "TLS_OUTDATED" {
		t.Errorf("srv-1 rules not sorted: %v", got)
	}
	// PATCH_OUTDATED hit two servers → top of the rule tally.
	if len(rep.Rules) != 2 || rep.Rules[0].Rule != "PATCH_OUTDATED" || rep.Rules[0].Failures != 2 {
		t.Errorf("rule tally = %+v, want PATCH_OUTDATED×2 first", rep.Rules)
	}
}

func TestReadReportDedupesSameEntityAcrossEvents(t *testing.T) {
	// The same server observed twice (e.g. a re-run) must not double-count the rule.
	stream := ndjson(
		`{"type":"`+EventType+`","data":{"entity":"srv-1","decisions":{"Violations":["DISK_LOW"]}}}`,
		`{"type":"`+EventType+`","data":{"entity":"srv-1","decisions":{"Violations":["DISK_LOW","FIREWALL_OFF"]}}}`,
	)
	rep, err := ReadReport(context.Background(), strings.NewReader(stream), "")
	if err != nil {
		t.Fatalf("ReadReport: %v", err)
	}
	if rep.Servers != 1 || rep.Failed != 1 {
		t.Fatalf("servers/failed = %d/%d, want 1/1", rep.Servers, rep.Failed)
	}
	if len(rep.Entities[0].Rules) != 2 {
		t.Fatalf("rules = %v, want the union DISK_LOW+FIREWALL_OFF once", rep.Entities[0].Rules)
	}
	for _, rs := range rep.Rules {
		if rs.Failures != 1 {
			t.Errorf("rule %s failures = %d, want 1 (distinct entities, not events)", rs.Rule, rs.Failures)
		}
	}
}

func TestReadReportViolationFlagWithoutNamedRules(t *testing.T) {
	// An expectation mismatch (violation:true) with no rule-list output still fails
	// the entity, with an empty rule list.
	stream := `{"type":"` + EventType + `","data":{"entity":"srv-9","decisions":{"Answer":42},"violation":true}}`
	rep, err := ReadReport(context.Background(), strings.NewReader(stream), "")
	if err != nil {
		t.Fatalf("ReadReport: %v", err)
	}
	if rep.Failed != 1 || len(rep.Entities) != 1 || len(rep.Entities[0].Rules) != 0 {
		t.Fatalf("want one failing entity with no named rules, got %+v", rep)
	}
}

func TestReadReportRuleFieldSelectsOneOutput(t *testing.T) {
	// With a ruleField, only that output contributes; other list outputs are ignored.
	stream := `{"type":"` + EventType + `","data":{"entity":"srv-1","decisions":{"Violations":["DISK_LOW"],"Tags":["prod","eu"]}}}`
	rep, err := ReadReport(context.Background(), strings.NewReader(stream), "Violations")
	if err != nil {
		t.Fatalf("ReadReport: %v", err)
	}
	if len(rep.Entities[0].Rules) != 1 || rep.Entities[0].Rules[0] != "DISK_LOW" {
		t.Fatalf("ruleField ignored other outputs? got %v", rep.Entities[0].Rules)
	}
}

func TestReadReportAutoDetectIgnoresNonStringListElements(t *testing.T) {
	stream := `{"type":"` + EventType + `","data":{"entity":"srv-1","decisions":{"Scores":[1,2,3]}}}`
	rep, err := ReadReport(context.Background(), strings.NewReader(stream), "")
	if err != nil {
		t.Fatalf("ReadReport: %v", err)
	}
	if rep.Failed != 0 || rep.Passed != 1 {
		t.Fatalf("numeric list should yield no rules; got %+v", rep)
	}
}

func TestReadReportSkipsForeignEventTypes(t *testing.T) {
	stream := ndjson(
		`{"type":"com.temis.decision.evaluated.v1","data":{"entity":"srv-x","decisions":{"Violations":["X"]}}}`,
		`{"type":"`+EventType+`","data":{"entity":"srv-1","decisions":{"Violations":["DISK_LOW"]}}}`,
	)
	rep, err := ReadReport(context.Background(), strings.NewReader(stream), "")
	if err != nil {
		t.Fatalf("ReadReport: %v", err)
	}
	if rep.Total != 1 || rep.Servers != 1 || rep.Entities[0].Entity != "srv-1" {
		t.Fatalf("foreign event not skipped: %+v", rep)
	}
}

func TestReadReportEmptyStream(t *testing.T) {
	rep, err := ReadReport(context.Background(), strings.NewReader(""), "")
	if err != nil {
		t.Fatalf("ReadReport: %v", err)
	}
	if rep.Total != 0 || rep.Servers != 0 || rep.Entities != nil || rep.Rules != nil {
		t.Fatalf("empty stream should give a zero report, got %+v", rep)
	}
}

func TestReadReportMissingEntityFallsBackToUnknown(t *testing.T) {
	stream := `{"type":"` + EventType + `","data":{"decisions":{"Violations":["X"]}}}`
	rep, err := ReadReport(context.Background(), strings.NewReader(stream), "")
	if err != nil {
		t.Fatalf("ReadReport: %v", err)
	}
	if rep.Entities[0].Entity != "unknown" {
		t.Fatalf("missing entity should fall back to unknown, got %q", rep.Entities[0].Entity)
	}
}

func TestReadReportDecodeErrorSurfaces(t *testing.T) {
	_, err := ReadReport(context.Background(), strings.NewReader(`{"type":"x","data":`), "")
	if err == nil {
		t.Fatal("want a decode error on a truncated stream")
	}
}

func TestReadReportRespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ReadReport(ctx, strings.NewReader(`{"type":"`+EventType+`","data":{"entity":"a"}}`), "")
	if err == nil {
		t.Fatal("want a context error when ctx is already cancelled")
	}
}
