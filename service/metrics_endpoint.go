package service

import (
	"expvar" // registers memstats/cmdline; we also expose our own counters
	"fmt"
	"net/http"
	"time"
)

// Operational metrics export (ADR-0030, WP-113). Opt-in (WithMetrics): when off,
// neither endpoint is mounted (404), so the default posture is unchanged. When
// on, both sit behind the audit scope like GET /v1/status and are secret-free —
// they publish counters, never a token or model content.
//
//   - GET /debug/vars : the standard expvar view (memstats + cmdline) plus a
//     "temis" object with our per-subsystem counters.
//   - GET /metrics    : the same counters in Prometheus text exposition format,
//     rendered by a tiny stdlib encoder (no Prometheus client dependency).

// metricsSnapshot is a point-in-time read of every operational counter, gathered
// from the counters' natural owners (process metrics, the clio sink, the cache).
type metricsSnapshot struct {
	uptimeSeconds  float64
	models         int
	evalOk         uint64
	evalFailed     uint64
	llmOk          uint64
	llmFailed      uint64
	clioWritesOk   uint64
	clioWritesFail uint64
	clioIdempotent uint64
	cacheHits      uint64
	cacheMisses    uint64
	cacheEvictions uint64
	cacheSize      int
	cacheCapacity  int
}

func (s *Server) metricsSnapshot() metricsSnapshot {
	cs := s.cache.stats()
	ms := metricsSnapshot{
		uptimeSeconds:  time.Since(s.metrics.start).Seconds(),
		models:         cs.Size,
		evalOk:         s.metrics.evalOk.Load(),
		evalFailed:     s.metrics.evalFailed.Load(),
		llmOk:          s.metrics.llmOk.Load(),
		llmFailed:      s.metrics.llmFailed.Load(),
		cacheHits:      cs.Hits,
		cacheMisses:    cs.Misses,
		cacheEvictions: cs.Evictions,
		cacheSize:      cs.Size,
		cacheCapacity:  cs.Capacity,
	}
	if s.sink != nil {
		snap := s.sink.snapshot()
		ms.clioWritesOk = snap.writesOk
		ms.clioWritesFail = snap.writesFailed
		ms.clioIdempotent = snap.idempotentSkips
	}
	return ms
}

// handleExpvars serves the expvar JSON: every globally published var (memstats,
// cmdline — registered by importing expvar) plus our own "temis" object. We
// render it directly rather than registering per-server vars in expvar's global
// registry, which would collide across multiple Server instances (tests).
func (s *Server) handleExpvars(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	// Writes to the response are best-effort; a broken client connection is not an
	// error we can act on, so the results are deliberately discarded (as elsewhere
	// in the service layer, e.g. openapi.go).
	_, _ = fmt.Fprint(w, "{\n")
	first := true
	expvar.Do(func(kv expvar.KeyValue) {
		if !first {
			_, _ = fmt.Fprint(w, ",\n")
		}
		first = false
		_, _ = fmt.Fprintf(w, "%q: %s", kv.Key, kv.Value)
	})
	if !first {
		_, _ = fmt.Fprint(w, ",\n")
	}
	_, _ = fmt.Fprintf(w, "%q: ", "temis")
	s.metricsSnapshot().writeExpvarJSON(w)
	_, _ = fmt.Fprint(w, "\n}\n")
}

func (m metricsSnapshot) writeExpvarJSON(w http.ResponseWriter) {
	_, _ = fmt.Fprintf(w,
		`{"uptimeSeconds":%d,"models":%d,`+
			`"evaluations":{"ok":%d,"failed":%d},`+
			`"llm":{"ok":%d,"failed":%d},`+
			`"clio":{"writesOk":%d,"writesFailed":%d,"idempotentSkips":%d},`+
			`"cache":{"hits":%d,"misses":%d,"evictions":%d,"size":%d,"capacity":%d}}`,
		int64(m.uptimeSeconds), m.models,
		m.evalOk, m.evalFailed,
		m.llmOk, m.llmFailed,
		m.clioWritesOk, m.clioWritesFail, m.clioIdempotent,
		m.cacheHits, m.cacheMisses, m.cacheEvictions, m.cacheSize, m.cacheCapacity,
	)
}

// handleMetrics serves the Prometheus text exposition format (v0.0.4).
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	m := s.metricsSnapshot()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	counter(w, "temis_evaluations_total", "Single-decision and whole-graph evaluations that succeeded.", m.evalOk)
	counter(w, "temis_evaluations_failed_total", "Evaluations that returned an error.", m.evalFailed)
	counter(w, "temis_llm_calls_total", "Modeling-assistant LLM calls that succeeded.", m.llmOk)
	counter(w, "temis_llm_calls_failed_total", "Modeling-assistant LLM calls that failed.", m.llmFailed)
	counter(w, "temis_clio_writes_total", "Audit events written to clio.", m.clioWritesOk)
	counter(w, "temis_clio_writes_failed_total", "Audit writes to clio that failed.", m.clioWritesFail)
	counter(w, "temis_clio_idempotent_skips_total", "Audit writes skipped as already-recorded (idempotent).", m.clioIdempotent)
	counter(w, "temis_cache_hits_total", "Model-cache lookups served from memory.", m.cacheHits)
	counter(w, "temis_cache_misses_total", "Model-cache lookups that missed.", m.cacheMisses)
	counter(w, "temis_cache_evictions_total", "Models evicted from the bounded cache.", m.cacheEvictions)
	gauge(w, "temis_models", "Models currently cached in memory.", float64(m.models))
	gauge(w, "temis_cache_capacity", "Model-cache capacity (0 = unbounded).", float64(m.cacheCapacity))
	gauge(w, "temis_uptime_seconds", "Seconds since the process started.", m.uptimeSeconds)
}

func counter(w http.ResponseWriter, name, help string, v uint64) {
	_, _ = fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n%s %d\n", name, help, name, name, v)
}

func gauge(w http.ResponseWriter, name, help string, v float64) {
	_, _ = fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s gauge\n%s %g\n", name, help, name, name, v)
}
