package service

import (
	"net/http"
	"testing"
	"time"
)

// TestRateLimiterBucket drives the token bucket with a controlled clock: a burst
// of B is allowed immediately, the next request is denied, and after enough time
// to refill one token a further request succeeds (audit findings H6/M2).
func TestRateLimiterBucket(t *testing.T) {
	now := time.Unix(0, 0)
	rl := newRateLimiter(1 /*rps*/, 3 /*burst*/, func() time.Time { return now })

	for i := 0; i < 3; i++ {
		if !rl.allow("1.2.3.4") {
			t.Fatalf("request %d within burst was denied", i+1)
		}
	}
	if rl.allow("1.2.3.4") {
		t.Fatal("request beyond burst was allowed")
	}
	// A different client has its own bucket.
	if !rl.allow("5.6.7.8") {
		t.Fatal("a distinct client was throttled by another client's bucket")
	}
	// One second later, exactly one token has refilled.
	now = now.Add(time.Second)
	if !rl.allow("1.2.3.4") {
		t.Fatal("token did not refill after a second")
	}
	if rl.allow("1.2.3.4") {
		t.Fatal("more than one token refilled in a second")
	}
}

// TestRateLimitHTTP asserts a configured server answers 429 once a client floods
// the gated /v1 surface faster than its bucket allows.
func TestRateLimitHTTP(t *testing.T) {
	h := NewServer(nil, WithRateLimit(1, 2)).Handler()
	// httptest.NewRequest gives every request the same RemoteAddr, so they share a
	// bucket. Burst is 2; the third rapid request must be shed.
	if rec := do(t, h, "GET", "/v1/models", "", nil); rec.Code == http.StatusTooManyRequests {
		t.Fatalf("first request throttled unexpectedly: %d", rec.Code)
	}
	do(t, h, "GET", "/v1/models", "", nil)
	rec := do(t, h, "GET", "/v1/models", "", nil)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after exhausting the burst, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("429 lacks a Retry-After header")
	}
}

// TestRateLimitOffByDefault confirms an unconfigured server never throttles.
func TestRateLimitOffByDefault(t *testing.T) {
	h := NewServer(nil).Handler()
	for i := 0; i < 50; i++ {
		if rec := do(t, h, "GET", "/v1/models", "", nil); rec.Code == http.StatusTooManyRequests {
			t.Fatalf("default server throttled at request %d", i+1)
		}
	}
}
