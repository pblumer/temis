package anthropic

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pblumer/temis/assist"
)

// TestDefaultClientTimesOut asserts the client New builds carries a request
// deadline, so a stalled provider aborts instead of hanging the caller forever
// (audit finding H4). The server blocks past a short injected timeout; Complete
// must return an error rather than block.
func TestDefaultClientTimesOut(t *testing.T) {
	// The handler stalls longer than the client's deadline. Using a bounded sleep
	// (not a channel the test must unblock) keeps srv.Close() from waiting on an
	// indefinitely-parked handler.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
	}))
	defer srv.Close()

	// New's default client has a (long) timeout; override with a short one via the
	// same mechanism operators use, proving the wiring bounds the request.
	c := New("sk-test", WithBaseURL(srv.URL), WithHTTPClient(&http.Client{Timeout: 100 * time.Millisecond}))

	start := time.Now()
	_, err := c.Complete(context.Background(), assist.Request{Messages: []assist.Message{{Role: assist.RoleUser, Text: "hi"}}})
	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Complete took %v; request was not bounded by the client timeout", elapsed)
	}
}

// TestNewDefaultsToBoundedClient documents that the zero-option client is not the
// unbounded http.DefaultClient.
func TestNewDefaultsToBoundedClient(t *testing.T) {
	c := New("sk-test")
	if c.http == http.DefaultClient {
		t.Fatal("New used http.DefaultClient (no timeout); expected a bounded client")
	}
	if c.http.Timeout <= 0 {
		t.Fatalf("New's client has no timeout (%v)", c.http.Timeout)
	}
}
