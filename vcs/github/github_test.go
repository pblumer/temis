package github_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pblumer/temis/dmn"
	"github.com/pblumer/temis/vcs"
	"github.com/pblumer/temis/vcs/github"
)

var repo = vcs.RepoRef{Owner: "pblumer", Name: "temis"}

func dishXML(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "dmn", "testdata", "models", "dish_15.dmn"))
	if err != nil {
		t.Fatalf("read dish model: %v", err)
	}
	return b
}

// newClient wires a github.Client to a test server and returns both.
func newClient(t *testing.T, h http.Handler) *github.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return github.New("test-token", github.WithBaseURL(srv.URL), github.WithHTTPClient(srv.Client()))
}

func TestListBranches_followsPagination(t *testing.T) {
	var srvURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/pblumer/temis/branches", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q, want bearer token", got)
		}
		if r.URL.Query().Get("page") == "2" {
			_, _ = fmt.Fprint(w, `[{"name":"release","commit":{"sha":"def"}}]`)
			return
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s/repos/pblumer/temis/branches?per_page=100&page=2>; rel="next"`, srvURL))
		_, _ = fmt.Fprint(w, `[{"name":"main","commit":{"sha":"abc"}}]`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	srvURL = srv.URL
	c := github.New("test-token", github.WithBaseURL(srv.URL), github.WithHTTPClient(srv.Client()))

	got, err := c.ListBranches(context.Background(), repo)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	want := []vcs.Branch{{Name: "main", Commit: "abc"}, {Name: "release", Commit: "def"}}
	if len(got) != len(want) {
		t.Fatalf("got %d branches, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("branch[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestListCommits_mapsFields(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/pblumer/temis/commits", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("sha"); got != "release" {
			t.Errorf("sha query = %q, want release", got)
		}
		_, _ = fmt.Fprint(w, `[{"sha":"c1","commit":{"message":"add dish","author":{"name":"Pat","date":"2026-06-29T10:00:00Z"}}}]`)
	})
	c := newClient(t, mux)

	got, err := c.ListCommits(context.Background(), repo, "release")
	if err != nil {
		t.Fatalf("ListCommits: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d commits, want 1", len(got))
	}
	want := vcs.Commit{SHA: "c1", Message: "add dish", Author: "Pat", Date: "2026-06-29T10:00:00Z"}
	if got[0] != want {
		t.Errorf("commit = %+v, want %+v", got[0], want)
	}
}

func TestListFiles_directory(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/pblumer/temis/contents/models", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("ref"); got != "main" {
			t.Errorf("ref = %q, want main", got)
		}
		_, _ = fmt.Fprint(w, `[
			{"name":"dish.dmn","path":"models/dish.dmn","sha":"s1","size":42,"type":"file"},
			{"name":"sub","path":"models/sub","sha":"s2","type":"dir"}
		]`)
	})
	c := newClient(t, mux)

	got, err := c.ListFiles(context.Background(), repo, "main", "models")
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
	if got[0] != (vcs.File{Path: "models/dish.dmn", Name: "dish.dmn", Size: 42, SHA: "s1", IsDir: false}) {
		t.Errorf("file entry = %+v", got[0])
	}
	if !got[1].IsDir {
		t.Errorf("entry %q should be a dir", got[1].Name)
	}
}

func TestReadFile_rawContent(t *testing.T) {
	want := dishXML(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/pblumer/temis/contents/models/dish.dmn", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept"), "raw") {
			t.Errorf("Accept = %q, want raw media type", r.Header.Get("Accept"))
		}
		_, _ = w.Write(want)
	})
	c := newClient(t, mux)

	got, err := c.ReadFile(context.Background(), repo, "main", "models/dish.dmn")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("content mismatch: got %d bytes, want %d", len(got), len(want))
	}
}

func TestReadFile_notFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/pblumer/temis/contents/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"message":"Not Found"}`)
	})
	c := newClient(t, mux)

	_, err := c.ReadFile(context.Background(), repo, "main", "models/nope.dmn")
	if !errors.Is(err, vcs.ErrNotFound) {
		t.Errorf("error %v does not wrap ErrNotFound", err)
	}
}

func TestReadFile_unauthorized(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/pblumer/temis/contents/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprint(w, `{"message":"Bad credentials"}`)
	})
	c := newClient(t, mux)

	_, err := c.ReadFile(context.Background(), repo, "main", "models/dish.dmn")
	if !errors.Is(err, vcs.ErrUnauthorized) {
		t.Errorf("error %v does not wrap ErrUnauthorized", err)
	}
}

// TestEndToEnd_loadFromGitHubAndEvaluate proves the full chain: a model fetched
// from (a fake) GitHub at a ref, compiled by the engine and evaluated.
func TestEndToEnd_loadFromGitHubAndEvaluate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/pblumer/temis/contents/models/dish.dmn", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(dishXML(t))
	})
	c := newClient(t, mux)
	m := vcs.NewModels(c, dmn.New())

	defs, _, err := m.Load(context.Background(), repo, "main", "models/dish.dmn")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	dec, err := defs.Decision("Dish")
	if err != nil {
		t.Fatalf("Decision: %v", err)
	}
	res, err := dec.Evaluate(context.Background(), dmn.Input{"Season": "Winter", "Guest Count": 8})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if got := res.Outputs["Dish"]; got != "Roastbeef" {
		t.Errorf("Dish = %v, want Roastbeef", got)
	}
}

func TestListFiles_singleFileObjectFallback(t *testing.T) {
	mux := http.NewServeMux()
	// The contents endpoint returns an object (not an array) when the path is a
	// file rather than a directory.
	mux.HandleFunc("/repos/pblumer/temis/contents/models/dish.dmn", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"name":"dish.dmn","path":"models/dish.dmn","sha":"s1","size":42,"type":"file"}`)
	})
	c := newClient(t, mux)

	got, err := c.ListFiles(context.Background(), repo, "", "models/dish.dmn")
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(got) != 1 || got[0].Name != "dish.dmn" || got[0].IsDir {
		t.Errorf("got %+v, want single file dish.dmn", got)
	}
}

func TestListCommits_defaultBranchOmitsSha(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/pblumer/temis/commits", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("sha") {
			t.Errorf("default-branch request must not set sha; got %q", r.URL.RawQuery)
		}
		_, _ = fmt.Fprint(w, `[]`)
	})
	c := newClient(t, mux)

	if _, err := c.ListCommits(context.Background(), repo, ""); err != nil {
		t.Fatalf("ListCommits: %v", err)
	}
}

func TestRequest_unexpectedStatusNonJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/pblumer/temis/branches", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, "upstream exploded")
	})
	c := newClient(t, mux)

	_, err := c.ListBranches(context.Background(), repo)
	if err == nil {
		t.Fatal("expected error on 500")
	}
	if errors.Is(err, vcs.ErrNotFound) || errors.Is(err, vcs.ErrUnauthorized) {
		t.Errorf("500 should map to neither sentinel; got %v", err)
	}
	if !strings.Contains(err.Error(), "upstream exploded") || !strings.Contains(err.Error(), "500") {
		t.Errorf("error %v should carry status and body snippet", err)
	}
}

func TestReadFile_emptyPath(t *testing.T) {
	c := github.New("", github.WithBaseURL("http://127.0.0.1:0"))
	_, err := c.ReadFile(context.Background(), repo, "main", "")
	if !errors.Is(err, vcs.ErrNotFound) {
		t.Errorf("empty path should wrap ErrNotFound; got %v", err)
	}
}

// TestReadFile_rejectsTraversal guards audit finding N6: a path with a ".." (or
// ".") segment is refused before any request is made, so it cannot redirect the
// API URL. The client points at a dead address; a rejection must come from the
// segment check, not a network error.
func TestReadFile_rejectsTraversal(t *testing.T) {
	c := github.New("", github.WithBaseURL("http://127.0.0.1:0"))
	for _, bad := range []string{"../secrets", "models/../../etc", "a/./b"} {
		if _, err := c.ReadFile(context.Background(), repo, "main", bad); !errors.Is(err, vcs.ErrNotFound) {
			t.Errorf("ReadFile(%q) err = %v, want ErrNotFound (traversal rejected)", bad, err)
		}
	}
	if _, err := c.ListFiles(context.Background(), repo, "main", "../up"); !errors.Is(err, vcs.ErrNotFound) {
		t.Errorf("ListFiles traversal err = %v, want ErrNotFound", err)
	}
}

func TestAnonymous_noAuthHeader(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/pblumer/temis/branches", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("anonymous client sent Authorization: %q", r.Header.Get("Authorization"))
		}
		_, _ = fmt.Fprint(w, `[]`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := github.New("", github.WithBaseURL(srv.URL), github.WithHTTPClient(srv.Client()))

	if _, err := c.ListBranches(context.Background(), repo); err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
}
