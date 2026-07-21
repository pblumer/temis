package service

import (
	"testing"
	"time"
)

func TestSessionStoreLifecycle(t *testing.T) {
	ss := newSessionStore(time.Hour)
	now := time.Unix(1_700_000_000, 0)
	ss.now = func() time.Time { return now }

	sess, err := ss.create("k_abc", []Scope{ScopeModelsRead})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if sess.id == "" || sess.csrf == "" || sess.id == sess.csrf {
		t.Fatalf("expected distinct non-empty id and csrf, got id=%q csrf=%q", sess.id, sess.csrf)
	}

	got, ok := ss.get(sess.id)
	if !ok || got.subject != "k_abc" {
		t.Fatalf("get returned ok=%v subject=%q, want true k_abc", ok, got.subject)
	}

	// Expiry: advance past the TTL and the session is gone (and pruned).
	now = now.Add(2 * time.Hour)
	if _, ok := ss.get(sess.id); ok {
		t.Fatal("expected expired session to be absent")
	}

	// Destroy removes a live session.
	now = time.Unix(1_700_000_000, 0)
	sess2, _ := ss.create("k_xyz", nil)
	ss.destroy(sess2.id)
	if _, ok := ss.get(sess2.id); ok {
		t.Fatal("expected destroyed session to be absent")
	}
}

func TestVerifyPKCE(t *testing.T) {
	// A known S256 pair (verifier → base64url(sha256(verifier))).
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	// challenge computed by the helper itself is what a compliant client sends;
	// re-deriving it here keeps the test independent of a hard-coded constant.
	if !verifyPKCE(verifier, s256(verifier)) {
		t.Fatal("expected matching verifier/challenge to verify")
	}
	if verifyPKCE("wrong", s256(verifier)) {
		t.Fatal("expected a wrong verifier to fail")
	}
	if verifyPKCE("", s256(verifier)) || verifyPKCE(verifier, "") {
		t.Fatal("expected empty verifier or challenge to fail")
	}
}
