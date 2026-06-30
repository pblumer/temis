// Command temis-reaudit verifies a clio decision log: it reads the
// com.temis.decision.evaluated.v1 events temisd wrote (ADR-0023, WP-54),
// re-evaluates each recorded input against its recorded model, and reports
// whether every decision still reproduces. It is read-only — it never writes
// back to clio — and exits non-zero if any decision fails to reproduce, so it
// scripts like clio's own `verify`.
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

	"github.com/pblumer/temis/audit"
	"github.com/pblumer/temis/dmn"
	"github.com/pblumer/temis/internal/version"
)

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	clioURL := flag.String("clio-url", os.Getenv("TEMIS_CLIO_URL"),
		"base URL of the clio instance to read decision events from (default $TEMIS_CLIO_URL)")
	clioToken := flag.String("clio-token", os.Getenv("TEMIS_CLIO_TOKEN"),
		"clio API key (kid.secret) with read scope (default $TEMIS_CLIO_TOKEN)")
	models := flag.String("models", "",
		"directory of DMN files used to resolve recorded modelIds (required)")
	subject := flag.String("subject", "/", "clio subject scope to audit")
	recursive := flag.Bool("recursive", true, "audit the whole subject subtree")
	limit := flag.Int("limit", 1_000_000, "max events to read from clio (0 = clio default)")
	asJSON := flag.Bool("json", false, "print the report as JSON instead of text")
	flag.Parse()

	if *showVersion {
		fmt.Printf("temis-reaudit %s\n", version.Resolve())
		return
	}
	if *clioURL == "" || *models == "" {
		fmt.Fprintln(os.Stderr, "temis-reaudit: -clio-url and -models are required")
		flag.Usage()
		os.Exit(2)
	}

	src, err := audit.NewDirModelSource(*models)
	if err != nil {
		fail(err)
	}

	ctx := context.Background()
	stream, err := openClioStream(ctx, *clioURL, *clioToken, *subject, *recursive, *limit)
	if err != nil {
		fail(err)
	}
	defer func() { _ = stream.Close() }()

	rep, err := audit.ReAudit(ctx, dmn.New(), stream, src)
	if err != nil {
		fail(err)
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(rep)
	} else {
		printReport(rep, src.Len())
	}
	if !rep.Reproduced() {
		os.Exit(1)
	}
}

// openClioStream issues a clio run-query for decision events and returns the
// NDJSON response body to stream into the auditor. The caller closes it.
func openClioStream(ctx context.Context, baseURL, token, subject string, recursive bool, limit int) (io.ReadCloser, error) {
	q := map[string]any{
		"subject":   subject,
		"recursive": recursive,
		"where":     fmt.Sprintf("event.type == %q", audit.DecisionEventType),
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

func printReport(rep audit.Report, modelCount int) {
	audit.SortOutcomes(rep.Outcomes)
	for _, o := range rep.Outcomes {
		fmt.Printf("  ✗ %-17s %s [%s] %s\n", o.Status, o.Decision, short(o.ModelID), o.Detail)
	}
	fmt.Printf("\nre-audited %d decision event(s) against %d model(s): %d reproduced",
		rep.Total, modelCount, rep.OK)
	if rep.Discrepancies > 0 {
		fmt.Printf(", %d discrepancy", rep.Discrepancies)
	}
	if rep.Unavailable > 0 {
		fmt.Printf(", %d model unavailable", rep.Unavailable)
	}
	if rep.EvalErrors > 0 {
		fmt.Printf(", %d eval error", rep.EvalErrors)
	}
	if rep.Reproduced() {
		fmt.Println(" — OK ✓")
	} else {
		fmt.Println(" — FAILED ✗")
	}
}

// short trims a "sha256:<hex>" id to a readable prefix for the report.
func short(id string) string {
	const n = len("sha256:") + 12
	if len(id) > n {
		return id[:n] + "…"
	}
	return id
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "temis-reaudit: %v\n", err)
	os.Exit(1)
}
