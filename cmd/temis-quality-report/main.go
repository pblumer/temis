// Command temis-quality-report turns the quality events temisd wrote for
// productive Import runs (com.temis.quality.evaluated.v1, ADR-0031) into a
// violation report: run a whole ruleset over a dataset — say 70 000 servers —
// then ask which server failed which rule. It reads the events straight from clio
// (read-only, like temis-reaudit) and prints a per-entity / per-rule summary as
// text or JSON. With -fail-on-violation it exits non-zero when any entity failed,
// so it gates a CI pipeline ("no server may be non-compliant").
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/pblumer/temis/internal/version"
	"github.com/pblumer/temis/quality"
)

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	clioURL := flag.String("clio-url", os.Getenv("TEMIS_CLIO_URL"),
		"base URL of the clio instance to read quality events from (default $TEMIS_CLIO_URL)")
	clioToken := flag.String("clio-token", os.Getenv("TEMIS_CLIO_TOKEN"),
		"clio API key (kid.secret) with read scope (default $TEMIS_CLIO_TOKEN)")
	subject := flag.String("subject", "/quality", "clio subject scope to report on")
	recursive := flag.Bool("recursive", true, "report on the whole subject subtree")
	ruleField := flag.String("rule-field", "",
		"decision output holding the violated-rule list (empty = auto-detect list-of-strings outputs)")
	limit := flag.Int("limit", 1_000_000, "max events to read from clio (0 = clio default)")
	asJSON := flag.Bool("json", false, "print the report as JSON instead of text")
	failOnViolation := flag.Bool("fail-on-violation", false, "exit non-zero when any entity failed a rule")
	flag.Parse()

	if *showVersion {
		fmt.Printf("temis-quality-report %s\n", version.Resolve())
		return
	}
	if *clioURL == "" {
		fmt.Fprintln(os.Stderr, "temis-quality-report: -clio-url is required")
		flag.Usage()
		os.Exit(2)
	}

	ctx := context.Background()
	stream, err := openQualityStream(ctx, *clioURL, *clioToken, *subject, *recursive, *limit)
	if err != nil {
		fail(err)
	}
	defer func() { _ = stream.Close() }()

	rep, err := quality.ReadReport(ctx, stream, *ruleField)
	if err != nil {
		fail(err)
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(rep)
	} else {
		printReport(rep)
	}
	if *failOnViolation && rep.Failed > 0 {
		os.Exit(1)
	}
}

// openQualityStream issues a clio run-query for quality events and returns the
// NDJSON response body to stream into the reporter. The caller closes it.
func openQualityStream(ctx context.Context, baseURL, token, subject string, recursive bool, limit int) (io.ReadCloser, error) {
	q := map[string]any{
		"subject":   subject,
		"recursive": recursive,
		"where":     fmt.Sprintf("event.type == %q", quality.EventType),
	}
	if limit > 0 {
		q["limit"] = limit
	}
	buf, _ := json.Marshal(q)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v1/run-query", bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("build run-query request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
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

// printReport renders the report as a compact, scriptable text summary: the
// worst-offending rules, then each failing entity with its violated rules.
func printReport(rep quality.Report) {
	if len(rep.Rules) > 0 {
		fmt.Println("Rule violations (entities affected):")
		for _, rs := range rep.Rules {
			fmt.Printf("  %-24s %d\n", rs.Rule, rs.Failures)
		}
		fmt.Println()
	}
	for _, e := range rep.Entities {
		fmt.Printf("  ✗ %-24s %s\n", e.Entity, joinRules(e.Rules))
	}
	if len(rep.Entities) > 0 {
		fmt.Println()
	}
	fmt.Printf("%d server(s): %d passed, %d failed (from %d event(s))\n",
		rep.Servers, rep.Passed, rep.Failed, rep.Total)
	if rep.Failed == 0 {
		fmt.Println("all compliant — OK ✓")
	} else {
		fmt.Println("compliance violations found ✗")
	}
}

func joinRules(rules []string) string {
	if len(rules) == 0 {
		return "(expectation mismatch)"
	}
	out := rules[0]
	for _, r := range rules[1:] {
		out += ", " + r
	}
	return out
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "temis-quality-report: %v\n", err)
	os.Exit(1)
}
