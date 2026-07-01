package github_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pblumer/temis/vcs"
	"github.com/pblumer/temis/vcs/github"
)

// TestListCommits_pagedError exercises the error branch of ListCommits when the
// underlying paged GET fails (here a 404).
func TestListCommits_pagedError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/pblumer/temis/commits", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"message":"Not Found"}`)
	})
	c := newClient(t, mux)

	if _, err := c.ListCommits(context.Background(), repo, "release"); !errors.Is(err, vcs.ErrNotFound) {
		t.Errorf("error %v does not wrap ErrNotFound", err)
	}
}

// TestListFiles_doError exercises the error branch of ListFiles when the
// contents request fails outright.
func TestListFiles_doError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/pblumer/temis/contents/models", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprint(w, `{"message":"Bad credentials"}`)
	})
	c := newClient(t, mux)

	if _, err := c.ListFiles(context.Background(), repo, "main", "models"); !errors.Is(err, vcs.ErrUnauthorized) {
		t.Errorf("error %v does not wrap ErrUnauthorized", err)
	}
}

// TestListFiles_undecodable covers the path where the contents body is neither a
// valid array nor a valid object, so both unmarshal attempts fail.
func TestListFiles_undecodable(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/pblumer/temis/contents/models", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `not json at all`)
	})
	c := newClient(t, mux)

	_, err := c.ListFiles(context.Background(), repo, "main", "models")
	if err == nil {
		t.Fatal("expected decode error")
	}
	if !strings.Contains(err.Error(), "decode contents") {
		t.Errorf("error %v should mention decode contents", err)
	}
}

// TestListBranches_pageDecodeError covers getPaged's decode-page error branch:
// a successful (2xx) response whose body is not a JSON array.
func TestListBranches_pageDecodeError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/pblumer/temis/branches", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"unexpected":"object"}`)
	})
	c := newClient(t, mux)

	_, err := c.ListBranches(context.Background(), repo)
	if err == nil {
		t.Fatal("expected decode error")
	}
	if !strings.Contains(err.Error(), "decode page") {
		t.Errorf("error %v should mention decode page", err)
	}
}

// TestSend_buildRequestError covers send's http.NewRequestWithContext failure by
// using a method string containing an illegal character.
func TestSend_buildRequestError(t *testing.T) {
	// A base URL with a control character makes NewRequestWithContext fail when
	// parsing the request URL.
	c := github.New("", github.WithBaseURL("http://example.com/\x7f"))
	_, err := c.ListBranches(context.Background(), repo)
	if err == nil {
		t.Fatal("expected build-request error")
	}
	if !strings.Contains(err.Error(), "build request") {
		t.Errorf("error %v should mention build request", err)
	}
}

// TestSend_transportError covers send's c.http.Do failure: the server closes the
// connection without a response.
func TestSend_transportError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Hijack and close the connection so no HTTP response is written.
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("server does not support hijacking")
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Fatalf("hijack: %v", err)
		}
		_ = conn.Close()
	}))
	t.Cleanup(srv.Close)
	c := github.New("", github.WithBaseURL(srv.URL), github.WithHTTPClient(srv.Client()))

	_, err := c.ListBranches(context.Background(), repo)
	if err == nil {
		t.Fatal("expected transport error")
	}
	if !strings.Contains(err.Error(), "request ") {
		t.Errorf("error %v should mention the failed request", err)
	}
}

// TestSend_readBodyError covers send's io.ReadAll failure: the server advertises
// a larger Content-Length than it actually writes, then closes the connection.
func TestSend_readBodyError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("server does not support hijacking")
		}
		conn, buf, err := hj.Hijack()
		if err != nil {
			t.Fatalf("hijack: %v", err)
		}
		// Promise 100 bytes but send only a few, then drop the connection so
		// ReadAll on the client side errors with an unexpected EOF.
		_, _ = buf.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 100\r\n\r\nshort")
		_ = buf.Flush()
		_ = conn.Close()
	}))
	t.Cleanup(srv.Close)
	c := github.New("", github.WithBaseURL(srv.URL), github.WithHTTPClient(srv.Client()))

	_, err := c.ListBranches(context.Background(), repo)
	if err == nil {
		t.Fatal("expected read-body error")
	}
	if !strings.Contains(err.Error(), "read response") {
		t.Errorf("error %v should mention read response", err)
	}
}

// TestApiMessage_bodyFallback covers apiMessage's fallback path: a non-2xx body
// with no "message" field is reported via a trimmed body snippet, and a long
// body is truncated to 200 characters.
func TestApiMessage_bodyFallback(t *testing.T) {
	long := strings.Repeat("x", 500)
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/pblumer/temis/branches", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		// Valid JSON object but without a "message" field, so apiMessage falls
		// through to the trimmed-body snippet.
		_, _ = fmt.Fprintf(w, `{"error":%q}`, long)
	})
	c := newClient(t, mux)

	_, err := c.ListBranches(context.Background(), repo)
	if err == nil {
		t.Fatal("expected error")
	}
	// The snippet is truncated to 200 chars, so the full 500-char run never
	// appears in the message.
	if strings.Contains(err.Error(), long) {
		t.Errorf("snippet should be truncated to 200 chars; got %v", err)
	}
	if !strings.Contains(err.Error(), "xxxxx") {
		t.Errorf("error %v should carry a body snippet", err)
	}
}

// TestNextLink_variants exercises nextLink through getPaged with Link headers
// that hit each branch: a malformed single-segment entry (skipped), a non-next
// rel (skipped), and overall no rel="next" (loop terminates).
func TestNextLink_variants(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/pblumer/temis/branches", func(w http.ResponseWriter, _ *http.Request) {
		// First segment has no ';' (len(segs)<2 → continue); second has a
		// rel="prev" (not next → continue). No next link at all, so the loop
		// stops after one page.
		w.Header().Set("Link", `<https://api.github.com/x>, <https://api.github.com/y>; rel="prev"`)
		_, _ = fmt.Fprint(w, `[{"name":"main","commit":{"sha":"abc"}}]`)
	})
	c := newClient(t, mux)

	got, err := c.ListBranches(context.Background(), repo)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	if len(got) != 1 || got[0].Name != "main" {
		t.Errorf("got %+v, want single branch main", got)
	}
}

// TestCommit_decodeResponseError covers Commit's response-decode error branch: a
// 2xx response whose body is not valid JSON.
func TestCommit_decodeResponseError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /repos/pblumer/temis/contents/models/dish.dmn", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `not json`)
	})
	c := newClient(t, mux)

	_, err := c.Commit(context.Background(), repo, vcs.CommitRequest{
		Branch: "feature", Path: "models/dish.dmn", Content: []byte("<xml/>"), Message: "edit",
	})
	if err == nil || !strings.Contains(err.Error(), "decode commit response") {
		t.Errorf("error %v should mention decode commit response", err)
	}
}

// TestOpenPullRequest_decodeResponseError covers OpenPullRequest's decode error
// branch.
func TestOpenPullRequest_decodeResponseError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /repos/pblumer/temis/pulls", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprint(w, `not json`)
	})
	c := newClient(t, mux)

	_, err := c.OpenPullRequest(context.Background(), repo, vcs.PullRequestRequest{Head: "feature", Base: "main"})
	if err == nil || !strings.Contains(err.Error(), "decode pull-request response") {
		t.Errorf("error %v should mention decode pull-request response", err)
	}
}

// TestResolveCommit_decodeError covers resolveCommit's decode-error branch via
// CreateBranch, when the commits endpoint returns an undecodable body.
func TestResolveCommit_decodeError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/pblumer/temis/commits/main", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `not json`)
	})
	c := newClient(t, mux)

	_, err := c.CreateBranch(context.Background(), repo, "feature", "main")
	if err == nil || !strings.Contains(err.Error(), "decode commit ref") {
		t.Errorf("error %v should mention decode commit ref", err)
	}
}

// TestResolveCommit_emptySHA covers resolveCommit's empty-SHA branch: a valid
// JSON response that carries no sha is treated as not found.
func TestResolveCommit_emptySHA(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/pblumer/temis/commits/main", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"sha":""}`)
	})
	c := newClient(t, mux)

	_, err := c.CreateBranch(context.Background(), repo, "feature", "main")
	if !errors.Is(err, vcs.ErrNotFound) {
		t.Errorf("error %v does not wrap ErrNotFound", err)
	}
}
