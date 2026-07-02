// Command temis-clio-worker consumes clio command events and answers them with
// temis decisions (ADR-0033) — the reverse direction of the audit sink
// (ADR-0023). It watches a clio subject subtree for command events
// (com.temis.decision.requested.v1), evaluates each one against the models and
// flows in a local directory, and writes the result back to clio as the same
// versioned events temisd's sink writes (com.temis.decision.evaluated.v1 /
// com.temis.flow.evaluated.v1), filed under the command's subject and correlated
// by requestId. A command that cannot be evaluated is answered with a
// com.temis.decision.failed.v1 event, so every command gets an answer.
//
// It is a stateless transform: clio owns all state (the command log, the result
// log and the idempotency query). New commands are consumed live over clio's
// observe stream (or, with -poll, by periodic run-query); a startup and
// per-reconnect run-query backfill catches anything appended while disconnected.
// Every write carries a precondition on the command's requestId, so re-delivery
// and overlap never double-answer a command.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pblumer/temis/consume"
	"github.com/pblumer/temis/dmn"
	"github.com/pblumer/temis/internal/version"
)

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	clioURL := flag.String("clio-url", os.Getenv("TEMIS_CLIO_URL"),
		"base URL of the clio instance to consume command events from (default $TEMIS_CLIO_URL)")
	clioToken := flag.String("clio-token", os.Getenv("TEMIS_CLIO_TOKEN"),
		"clio API key (kid.secret) with read + write scope on the subject subtree (default $TEMIS_CLIO_TOKEN)")
	source := flag.String("clio-source", envOr("TEMIS_CLIO_SOURCE", "temis-clio-worker"),
		"CloudEvents source stamped on the result events (default $TEMIS_CLIO_SOURCE, else temis-clio-worker)")
	models := flag.String("models", os.Getenv("TEMIS_MODELS_DIR"),
		"directory of DMN files (*.dmn/*.xml) and flow descriptors (*.flow.json) resolving the modelIds/flowIds commands reference (required)")
	subject := flag.String("subject", "/", "clio subject scope to watch for command events")
	recursive := flag.Bool("recursive", true, "watch the whole subject subtree")
	observePath := flag.String("observe-path", "/api/v1/observe", "clio live-observe route (streams new matching events)")
	poll := flag.Bool("poll", false, "poll with run-query on an interval instead of using the observe stream")
	pollInterval := flag.Duration("poll-interval", 2*time.Second, "interval between run-query polls when -poll is set")
	once := flag.Bool("once", false, "process the current command backlog with run-query and exit (no live watch)")
	flag.Parse()

	if *showVersion {
		fmt.Printf("temis-clio-worker %s\n", version.Resolve())
		return
	}
	if *clioURL == "" || *models == "" {
		fmt.Fprintln(os.Stderr, "temis-clio-worker: -clio-url and -models are required")
		flag.Usage()
		os.Exit(2)
	}

	src, err := consume.NewDirSource(*models)
	if err != nil {
		fail(err)
	}
	log.Printf("temis-clio-worker %s: %d model(s), %d flow(s) from %s", version.Resolve(), src.Models(), src.Flows(), *models)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	w := &worker{
		client:    &http.Client{Timeout: 30 * time.Second},
		baseURL:   trimSlash(*clioURL),
		token:     *clioToken,
		source:    *source,
		engine:    "temis-clio-worker " + version.Resolve(),
		subject:   *subject,
		recursive: *recursive,
		eng:       dmn.New(),
		src:       src,
		processed: map[string]bool{},
	}

	// Backfill first so a command written while the worker was down is answered
	// before we start the live watch.
	if err := w.backfill(ctx); err != nil && ctx.Err() == nil {
		log.Printf("temis-clio-worker: backfill: %v", err)
	}
	if *once {
		return
	}

	if *poll {
		w.pollLoop(ctx, *pollInterval)
	} else {
		w.observeLoop(ctx, *observePath)
	}
}

// worker holds the immutable configuration and the in-process dedupe set. Events
// are processed sequentially (one goroutine), so the set needs no lock and the
// engine is used serially.
type worker struct {
	client    *http.Client
	baseURL   string
	token     string
	source    string
	engine    string
	subject   string
	recursive bool

	eng       *dmn.Engine
	src       consume.Source
	processed map[string]bool
}

// maxProcessed caps the in-memory dedup set (see process); beyond it the set is
// reset, bounding memory without weakening idempotency (clio's precondition).
const maxProcessed = 100_000

// observeLoop watches clio's live observe stream, reconnecting with backoff. Each
// (re)connection is preceded by a run-query backfill so events appended during a
// disconnect are not missed; the requestId precondition makes the overlap safe.
func (w *worker) observeLoop(ctx context.Context, observePath string) {
	backoff := 500 * time.Millisecond
	const maxBackoff = 30 * time.Second
	for ctx.Err() == nil {
		if err := w.backfill(ctx); err != nil && ctx.Err() == nil {
			log.Printf("temis-clio-worker: catch-up backfill: %v", err)
		}
		err := w.stream(ctx, http.MethodPost, observePath)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Printf("temis-clio-worker: observe stream ended: %v (reconnecting in %s)", err, backoff)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff *= 2; backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// pollLoop repeatedly runs a run-query backfill on an interval — the simpler,
// reconnect-free alternative to the observe stream.
func (w *worker) pollLoop(ctx context.Context, interval time.Duration) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if err := w.backfill(ctx); err != nil && ctx.Err() == nil {
				log.Printf("temis-clio-worker: poll: %v", err)
			}
		}
	}
}

// backfill reads the current command events via run-query and processes each.
func (w *worker) backfill(ctx context.Context) error {
	return w.stream(ctx, http.MethodPost, "/api/v1/run-query")
}

// stream opens a clio query/observe request for command events and processes the
// NDJSON response, one event per line, until the stream ends or ctx is cancelled.
func (w *worker) stream(ctx context.Context, method, path string) error {
	q := map[string]any{
		"subject":   w.subject,
		"recursive": w.recursive,
		"where":     fmt.Sprintf("event.type == %q", consume.CommandEventType),
	}
	body, _ := json.Marshal(q)
	req, err := http.NewRequestWithContext(ctx, method, w.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if w.token != "" {
		req.Header.Set("Authorization", "Bearer "+w.token)
	}
	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode/100 != 2 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("clio %s: status %d: %s", path, resp.StatusCode, bytes.TrimSpace(snippet))
	}

	dec := json.NewDecoder(resp.Body)
	for {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("decode event stream: %w", err)
		}
		if err := w.process(ctx, raw); err != nil {
			// A write failure (e.g. clio unreachable) is transient: leave the
			// command unprocessed so the next backfill retries it, and surface the
			// error to trigger a reconnect/backoff.
			return err
		}
	}
}

// process handles one raw stream event: parse, evaluate, write the result(s)
// back. A non-command event or an already-processed command is skipped. An
// evaluation failure is answered with a failure event (still a successful
// answer); only a clio write failure is returned as an error.
func (w *worker) process(ctx context.Context, raw []byte) error {
	cmd, ok, err := consume.ParseCommand(raw)
	if err != nil {
		log.Printf("temis-clio-worker: skipping unparseable event: %v", err)
		return nil
	}
	if !ok || w.processed[cmd.EventID] {
		return nil
	}

	evs, evalErr := consume.Handle(ctx, w.eng, cmd, w.src, w.engine)
	if evalErr != nil {
		log.Printf("temis-clio-worker: command %s not evaluable: %v", cmd.EventID, evalErr)
		evs = []consume.ResultEvent{consume.FailureEvent(cmd, evalErr, w.engine)}
	}
	for _, ev := range evs {
		if err := w.write(ctx, ev); err != nil {
			return fmt.Errorf("write result for command %s: %w", cmd.EventID, err)
		}
	}
	// Bound the in-memory dedup set so a long-running worker cannot leak memory
	// proportional to the command history (audit finding M4). This set is only a
	// fast-path optimisation to skip re-evaluating a command that the backfill and
	// the live stream both deliver; true idempotency is clio's requestId
	// precondition (a duplicate write 409s as a harmless no-op), so clearing the
	// set when it grows large is safe.
	if len(w.processed) >= maxProcessed {
		w.processed = map[string]bool{}
	}
	w.processed[cmd.EventID] = true
	if evalErr == nil {
		log.Printf("temis-clio-worker: answered command %s (%s) with %d event(s)", cmd.EventID, cmd.Subject, len(evs))
	}
	return nil
}

// writeRequest mirrors clio's POST /api/v1/write-events body.
type writeRequest struct {
	Events        []writeEvent   `json:"events"`
	Preconditions []precondition `json:"preconditions,omitempty"`
}

type writeEvent struct {
	Source  string `json:"source"`
	Subject string `json:"subject"`
	Type    string `json:"type"`
	Data    any    `json:"data"`
}

type precondition struct {
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload"`
}

// write posts one result event to clio with an idempotency precondition on the
// command's requestId (and decision, for a whole-graph command's per-decision
// events). A 409 means the command was already answered — a successful no-op.
func (w *worker) write(ctx context.Context, ev consume.ResultEvent) error {
	where := fmt.Sprintf("event.data.requestId == %q", ev.RequestID)
	if ev.Decision != "" {
		where += fmt.Sprintf(" && event.data.decision == %q", ev.Decision)
	}
	body, err := json.Marshal(writeRequest{
		Events: []writeEvent{{
			Source:  w.source,
			Subject: ev.Subject,
			Type:    ev.Type,
			Data:    ev.Data,
		}},
		Preconditions: []precondition{{
			Type: "isQueryResultEmpty",
			Payload: map[string]any{
				"subject": ev.Subject,
				"where":   where,
			},
		}},
	})
	if err != nil {
		return fmt.Errorf("encode event: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.baseURL+"/api/v1/write-events", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if w.token != "" {
		req.Header.Set("Authorization", "Bearer "+w.token)
	}
	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("POST write-events: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	switch {
	case resp.StatusCode == http.StatusConflict, resp.StatusCode/100 == 2:
		return nil
	default:
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("clio write-events: status %d: %s", resp.StatusCode, bytes.TrimSpace(snippet))
	}
}

func trimSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

func envOr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "temis-clio-worker: %v\n", err)
	os.Exit(1)
}
