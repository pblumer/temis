package service

import (
	"net/http"
	"strings"
	"testing"
)

// TestStatusReportsEngineAndSubsystems checks the /v1/status view: engine
// version/uptime, cache counters and the clio subsystem, populated from a real
// evaluation. The clio token must never appear in the body (WP-112).
func TestStatusReportsEngineAndSubsystems(t *testing.T) {
	clio := &captureClio{}
	h := auditServer(t, clio, nil)

	if rec := evalDish(t, h); rec.Code != http.StatusOK {
		t.Fatalf("evaluate = %d, want 200", rec.Code)
	}

	rec := do(t, h, "GET", "/v1/status", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/status = %d, want 200", rec.Code)
	}
	// The token "kid_t.secret" is configured on the sink; it must not leak.
	if strings.Contains(rec.Body.String(), "kid_t.secret") {
		t.Fatalf("status body leaks the clio token: %s", rec.Body.String())
	}

	st := decode[statusResponse](t, rec)
	if st.Engine.Version == "" {
		t.Error("engine.version is empty")
	}
	if st.Engine.Models < 1 {
		t.Errorf("engine.models = %d, want >= 1 after an evaluation", st.Engine.Models)
	}
	if !st.Clio.Enabled {
		t.Error("clio.enabled = false, want true")
	}
	if st.Clio.Mode != "best-effort" {
		t.Errorf("clio.mode = %q, want best-effort", st.Clio.Mode)
	}
	if st.Clio.WritesOk < 1 {
		t.Errorf("clio.writesOk = %d, want >= 1", st.Clio.WritesOk)
	}
	if !st.Clio.Reachable {
		t.Error("clio.reachable = false, want true after a successful write")
	}
	if st.Clio.LastOk == "" {
		t.Error("clio.lastOk is empty after a successful write")
	}
	if st.LLM.Enabled {
		t.Error("llm.enabled = true, want false (no assistant configured)")
	}
	if !st.Git.Available {
		t.Error("git.available = false, want true")
	}
}

// TestStatusCountsIdempotentSkips checks that a clio 409 (already recorded) is
// reported as an idempotent no-op, not a plain write.
func TestStatusCountsIdempotentSkips(t *testing.T) {
	clio := &captureClio{status: http.StatusConflict}
	h := auditServer(t, clio, nil)

	if rec := evalDish(t, h); rec.Code != http.StatusOK {
		t.Fatalf("evaluate = %d, want 200", rec.Code)
	}
	st := decode[statusResponse](t, do(t, h, "GET", "/v1/status", "", nil))
	if st.Clio.IdempotentSkips < 1 {
		t.Errorf("clio.idempotentSkips = %d, want >= 1", st.Clio.IdempotentSkips)
	}
	if st.Clio.WritesOk != 0 {
		t.Errorf("clio.writesOk = %d, want 0 (409 is not a fresh write)", st.Clio.WritesOk)
	}
	if !st.Clio.Reachable {
		t.Error("clio.reachable = false, want true (409 is a success)")
	}
}

// TestStatusReportsFailedClio checks a failed best-effort write surfaces as a
// counter, a lastError and reachable=false — the previously blind spot.
func TestStatusReportsFailedClio(t *testing.T) {
	clio := &captureClio{status: http.StatusInternalServerError}
	h := auditServer(t, clio, nil)

	if rec := evalDish(t, h); rec.Code != http.StatusOK {
		t.Fatalf("best-effort evaluate = %d, want 200 (audit failure is logged, not fatal)", rec.Code)
	}
	st := decode[statusResponse](t, do(t, h, "GET", "/v1/status", "", nil))
	if st.Clio.WritesFailed < 1 {
		t.Errorf("clio.writesFailed = %d, want >= 1", st.Clio.WritesFailed)
	}
	if st.Clio.LastError == "" {
		t.Error("clio.lastError is empty after a failed write")
	}
	if st.Clio.Reachable {
		t.Error("clio.reachable = true, want false after a failed write")
	}
}

// TestReadyzHonestSplit checks WP-110: /healthz stays liveness-only, /readyz is
// ready on a plain server, and a best-effort clio outage does not fail readiness.
func TestReadyzHonestSplit(t *testing.T) {
	h := newTestServer(t)
	if rec := do(t, h, "GET", "/healthz", "", nil); rec.Code != http.StatusOK {
		t.Errorf("/healthz = %d, want 200", rec.Code)
	}
	rec := do(t, h, "GET", "/readyz", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("/readyz = %d, want 200 on a plain server", rec.Code)
	}
	if st := decode[readyResponse](t, rec); st.Status != "ready" {
		t.Errorf("readyz status = %q, want ready", st.Status)
	}

	// A best-effort clio that is failing must NOT take the instance out of
	// rotation: readiness stays 200.
	clio := &captureClio{status: http.StatusInternalServerError}
	hb := auditServer(t, clio, nil)
	if rec := evalDish(t, hb); rec.Code != http.StatusOK {
		t.Fatalf("best-effort evaluate = %d, want 200", rec.Code)
	}
	if rec := do(t, hb, "GET", "/readyz", "", nil); rec.Code != http.StatusOK {
		t.Errorf("/readyz with failing best-effort clio = %d, want 200", rec.Code)
	}
}

// TestReadyzFailsWhenStrictClioUnreachable checks WP-110: a fail-closed (strict)
// clio that is unreachable makes /readyz answer 503 with the failing check.
func TestReadyzFailsWhenStrictClioUnreachable(t *testing.T) {
	clio := &captureClio{status: http.StatusInternalServerError}
	h := auditServer(t, clio, func(cfg *ClioConfig) { cfg.Strict = true })

	// In strict mode the failed audit write aborts the evaluation (502) and marks
	// the sink unreachable.
	if rec := evalDish(t, h); rec.Code != http.StatusBadGateway {
		t.Fatalf("strict evaluate with failing clio = %d, want 502", rec.Code)
	}
	rec := do(t, h, "GET", "/readyz", "", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("/readyz with unreachable strict clio = %d, want 503", rec.Code)
	}
	st := decode[readyResponse](t, rec)
	if st.Status != "not ready" || len(st.Checks) == 0 {
		t.Errorf("readyz = %+v, want not-ready with a failing check", st)
	}
}

// TestStatusActiveProbe checks the opt-in active clio probe (WP-112): reachability
// then comes from a live GET to clio's health endpoint.
func TestStatusActiveProbe(t *testing.T) {
	// Healthy clio → probe succeeds → reachable.
	up := &captureClio{}
	stub := up.start(t)
	sink, err := NewClioSink(ClioConfig{URL: stub.URL, Token: "kid_t.secret"})
	if err != nil {
		t.Fatalf("NewClioSink: %v", err)
	}
	h := NewServer(nil, WithClioSink(sink), WithClioActiveProbe(true)).Handler()
	st := decode[statusResponse](t, do(t, h, "GET", "/v1/status", "", nil))
	if !st.Clio.Reachable {
		t.Error("active probe against a healthy clio: reachable = false, want true")
	}

	// Unhealthy clio → probe fails → not reachable, even with no prior writes.
	down := &captureClio{status: http.StatusInternalServerError}
	dstub := down.start(t)
	dsink, err := NewClioSink(ClioConfig{URL: dstub.URL})
	if err != nil {
		t.Fatalf("NewClioSink: %v", err)
	}
	hd := NewServer(nil, WithClioSink(dsink), WithClioActiveProbe(true)).Handler()
	std := decode[statusResponse](t, do(t, hd, "GET", "/v1/status", "", nil))
	if std.Clio.Reachable {
		t.Error("active probe against an unhealthy clio: reachable = true, want false")
	}
}
