package service

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// rateLimiter is an opt-in, per-client token-bucket throttle guarding the /v1
// surface against request floods — anonymous cost abuse of the BYOK assistant
// endpoint and DoS via the recompiling modeler routes (audit findings H6/M2).
// It is disabled unless an operator configures a positive rate, so the default
// posture is byte-identical to before.
//
// Keying is by client IP (RemoteAddr host): the abuse scenario is an anonymous
// or unauthenticated caller, so the network origin is the stable identity. The
// X-Forwarded-For header is deliberately NOT trusted (it is client-spoofable);
// operators terminating behind a proxy should rate-limit there too.
type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rps     float64
	burst   float64
	now     func() time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

// maxRateKeys caps the bucket map so a stream of distinct source IPs cannot grow
// it without bound; once crossed, idle (full) buckets are pruned.
const maxRateKeys = 100_000

func newRateLimiter(rps, burst float64, now func() time.Time) *rateLimiter {
	if now == nil {
		now = time.Now
	}
	return &rateLimiter{buckets: map[string]*bucket{}, rps: rps, burst: burst, now: now}
}

// allow reports whether a request from key may proceed, consuming one token.
func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	t := rl.now()
	b, ok := rl.buckets[key]
	if !ok {
		if len(rl.buckets) >= maxRateKeys {
			rl.prune()
		}
		b = &bucket{tokens: rl.burst, last: t}
		rl.buckets[key] = b
	}
	// Refill proportionally to elapsed time, capped at the burst size.
	b.tokens += t.Sub(b.last).Seconds() * rl.rps
	if b.tokens > rl.burst {
		b.tokens = rl.burst
	}
	b.last = t
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// prune drops buckets that are back at full capacity (idle long enough to have
// refilled), reclaiming memory without affecting any actively-throttled client.
func (rl *rateLimiter) prune() {
	for k, b := range rl.buckets {
		if b.tokens >= rl.burst {
			delete(rl.buckets, k)
		}
	}
}

// wrap throttles next per client IP, answering 429 with a Retry-After hint when
// the caller's bucket is empty.
func (rl *rateLimiter) wrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !rl.allow(clientIP(r)) {
			w.Header().Set("Retry-After", "1")
			writeProblem(w, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests; slow down")
			return
		}
		next(w, r)
	}
}

// clientIP returns the connection's source IP (host part of RemoteAddr), or the
// raw RemoteAddr if it has no port.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
