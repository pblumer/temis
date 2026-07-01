package service

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"
)

// Operational observability (ADR-0030): temis exposes honest signals — an honest
// readiness probe (WP-110), per-subsystem counters (WP-111) and the GET
// /v1/status view over the connected Umsysteme (WP-112) — but does not aggregate,
// dashboard or alert. That is the external ops layer's job. Everything here lives
// in the adapter, uses only the standard library, and never leaks a secret.

// metrics holds the process-level counters the status endpoint reports and the
// expvar exporter (WP-113) will publish. All fields are atomic and updated
// allocation-free, so the evaluate hot path is unaffected. The clio counters
// live on the sink (its natural owner); cache counters live on the cache.
type metrics struct {
	start time.Time

	evalOk     atomic.Uint64
	evalFailed atomic.Uint64

	llmOk     atomic.Uint64
	llmFailed atomic.Uint64
	llmLastOk atomic.Int64
}

func newMetrics() *metrics { return &metrics{start: time.Now()} }

// --- status DTOs (secret-free by construction) ---

type statusResponse struct {
	Engine engineStatus `json:"engine"`
	Clio   clioStatus   `json:"clio"`
	LLM    llmStatus    `json:"llm"`
	Git    gitStatus    `json:"git"`
}

type engineStatus struct {
	Version       string     `json:"version"`
	Uptime        string     `json:"uptime"`
	UptimeSeconds int64      `json:"uptimeSeconds"`
	Models        int        `json:"models"`
	Cache         cacheStats `json:"cache"`
}

// clioStatus reports the audit sink's configuration and observed health. It
// never carries the clio API token.
type clioStatus struct {
	Enabled         bool   `json:"enabled"`
	Mode            string `json:"mode,omitempty"` // best-effort | strict
	URL             string `json:"url,omitempty"`
	WritesOk        uint64 `json:"writesOk"`
	WritesFailed    uint64 `json:"writesFailed"`
	IdempotentSkips uint64 `json:"idempotentSkips"`
	LastOk          string `json:"lastOk,omitempty"`
	LastError       string `json:"lastError,omitempty"`
	LastErrorAt     string `json:"lastErrorAt,omitempty"`
	Reachable       bool   `json:"reachable"`
}

// llmStatus reports the modeling assistant's configuration and call counters. It
// never carries the provider API key; byok merely says a caller may bring one.
type llmStatus struct {
	Enabled     bool   `json:"enabled"`
	Provider    string `json:"provider,omitempty"`
	BYOK        bool   `json:"byok"`
	CallsOk     uint64 `json:"callsOk"`
	CallsFailed uint64 `json:"callsFailed"`
	LastOk      string `json:"lastOk,omitempty"`
}

type gitStatus struct {
	Available bool `json:"available"`
}

// handleStatus reports the state of the connected Umsysteme (clio, LLM, git) and
// the engine's own load, for operations (WP-112, ADR-0030). It is gated by the
// same optional /v1 token as the other data routes and is secret-free: no API
// key or token ever appears in the body. When the active clio probe is enabled
// (WithClioActiveProbe) reachability comes from a live GET to clio; otherwise it
// is the passive health derived from real writes.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	up := time.Since(s.metrics.start)
	writeJSON(w, http.StatusOK, statusResponse{
		Engine: engineStatus{
			Version:       s.versionOrDefault(),
			Uptime:        up.Round(time.Second).String(),
			UptimeSeconds: int64(up.Seconds()),
			Models:        s.cache.len(),
			Cache:         s.cache.stats(),
		},
		Clio: s.clioStatus(r.Context()),
		LLM:  s.llmStatus(),
		Git:  gitStatus{Available: true},
	})
}

func (s *Server) versionOrDefault() string {
	if s.version != "" {
		return s.version
	}
	return "0.0.0-dev"
}

// clioStatus assembles the audit sink's status. With the active probe enabled it
// issues a live GET to clio's health endpoint to determine reachability; without
// it, reachability is the passive last-write outcome (no network call).
func (s *Server) clioStatus(ctx context.Context) clioStatus {
	if s.sink == nil {
		return clioStatus{Enabled: false}
	}
	snap := s.sink.snapshot()
	st := clioStatus{
		Enabled:         true,
		Mode:            "best-effort",
		URL:             snap.url,
		WritesOk:        snap.writesOk,
		WritesFailed:    snap.writesFailed,
		IdempotentSkips: snap.idempotentSkips,
		Reachable:       snap.reachable,
	}
	if snap.strict {
		st.Mode = "strict"
	}
	if snap.lastOkUnix > 0 {
		st.LastOk = time.Unix(snap.lastOkUnix, 0).UTC().Format(time.RFC3339)
	}
	if snap.lastErrUnix > 0 {
		st.LastErrorAt = time.Unix(snap.lastErrUnix, 0).UTC().Format(time.RFC3339)
		st.LastError = snap.lastErr
	}
	if s.clioActiveProbe {
		pctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		if err := s.sink.Ping(pctx); err != nil {
			st.Reachable = false
			if st.LastError == "" {
				st.LastError = err.Error()
			}
		} else {
			st.Reachable = true
		}
	}
	return st
}

func (s *Server) llmStatus() llmStatus {
	if s.assist == nil {
		return llmStatus{Enabled: false}
	}
	provider := s.assist.Provider
	if provider == "" {
		provider = "anthropic"
	}
	st := llmStatus{
		Enabled:     true,
		Provider:    provider,
		BYOK:        s.assist.AllowBYOK,
		CallsOk:     s.metrics.llmOk.Load(),
		CallsFailed: s.metrics.llmFailed.Load(),
	}
	if u := s.metrics.llmLastOk.Load(); u > 0 {
		st.LastOk = time.Unix(u, 0).UTC().Format(time.RFC3339)
	}
	return st
}

// --- liveness / readiness (WP-110) ---

type readyResponse struct {
	Status string   `json:"status"`
	Checks []string `json:"checks,omitempty"`
}

// handleLive is the liveness probe: it answers 200 as long as the process is up
// and serving. It makes no dependency checks, so a transient Umsystem problem
// never takes the instance out of rotation.
func (s *Server) handleLive(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleReady is the readiness probe: it answers 200 only when the server can
// actually serve, and 503 (with the failing checks) otherwise. Only hard start
// conditions count — a best-effort clio hiccup does not fail readiness; a
// fail-closed (strict) clio does, because then a decision cannot be recorded and
// so cannot be made.
func (s *Server) handleReady(w http.ResponseWriter, _ *http.Request) {
	if checks := s.notReady(); len(checks) > 0 {
		writeJSON(w, http.StatusServiceUnavailable, readyResponse{Status: "not ready", Checks: checks})
		return
	}
	writeJSON(w, http.StatusOK, readyResponse{Status: "ready"})
}

// notReady returns the hard start conditions that are currently unmet. Empty
// means ready. The strict-clio check reads the passive last-write outcome, so
// the probe stays fast (no network call in the readiness path).
func (s *Server) notReady() []string {
	var checks []string
	if s.storeDir != "" && s.store == nil {
		checks = append(checks, "model store configured but failed to open")
	}
	if s.sink != nil {
		if snap := s.sink.snapshot(); snap.strict && !snap.reachable {
			msg := "clio audit sink is fail-closed and unreachable"
			if snap.lastErr != "" {
				msg += ": " + snap.lastErr
			}
			checks = append(checks, msg)
		}
	}
	return checks
}
