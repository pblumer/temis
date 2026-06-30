package github_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/pblumer/temis/vcs"
	"github.com/pblumer/temis/vcs/github"
)

func decodeBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("decode request body %q: %v", b, err)
	}
	return m
}

func TestCreateBranch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/pblumer/temis/commits/main", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"sha":"base123"}`)
	})
	mux.HandleFunc("/repos/pblumer/temis/git/refs", func(w http.ResponseWriter, r *http.Request) {
		body := decodeBody(t, r)
		if body["ref"] != "refs/heads/feature" {
			t.Errorf("ref = %v, want refs/heads/feature", body["ref"])
		}
		if body["sha"] != "base123" {
			t.Errorf("sha = %v, want base123", body["sha"])
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprint(w, `{"ref":"refs/heads/feature","object":{"sha":"base123"}}`)
	})
	c := newClient(t, mux)

	br, err := c.CreateBranch(context.Background(), repo, "feature", "main")
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if br != (vcs.Branch{Name: "feature", Commit: "base123"}) {
		t.Errorf("branch = %+v", br)
	}
}

func TestCreateBranch_alreadyExistsIsConflict(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/pblumer/temis/commits/main", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"sha":"base123"}`)
	})
	mux.HandleFunc("/repos/pblumer/temis/git/refs", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = fmt.Fprint(w, `{"message":"Reference already exists"}`)
	})
	c := newClient(t, mux)

	_, err := c.CreateBranch(context.Background(), repo, "feature", "main")
	if !errors.Is(err, vcs.ErrConflict) {
		t.Errorf("error %v does not wrap ErrConflict", err)
	}
}

func TestCommit_createNewFile(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /repos/pblumer/temis/contents/models/dish.dmn", func(w http.ResponseWriter, r *http.Request) {
		body := decodeBody(t, r)
		if body["branch"] != "feature" {
			t.Errorf("branch = %v, want feature", body["branch"])
		}
		if body["message"] != "add dish" {
			t.Errorf("message = %v", body["message"])
		}
		if _, ok := body["sha"]; ok {
			t.Errorf("create must not send sha, got %v", body["sha"])
		}
		content, _ := base64.StdEncoding.DecodeString(body["content"].(string))
		if string(content) != "<xml/>" {
			t.Errorf("decoded content = %q, want <xml/>", content)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprint(w, `{"content":{"sha":"blob9"},"commit":{"sha":"commit9"}}`)
	})
	c := newClient(t, mux)

	res, err := c.Commit(context.Background(), repo, vcs.CommitRequest{
		Branch: "feature", Path: "models/dish.dmn", Content: []byte("<xml/>"), Message: "add dish",
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if res != (vcs.CommitResult{CommitSHA: "commit9", BlobSHA: "blob9"}) {
		t.Errorf("result = %+v", res)
	}
}

func TestCommit_updateSendsPrevSHA(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /repos/pblumer/temis/contents/models/dish.dmn", func(w http.ResponseWriter, r *http.Request) {
		body := decodeBody(t, r)
		if body["sha"] != "old-blob" {
			t.Errorf("update sha = %v, want old-blob", body["sha"])
		}
		_, _ = fmt.Fprint(w, `{"content":{"sha":"blob10"},"commit":{"sha":"commit10"}}`)
	})
	c := newClient(t, mux)

	res, err := c.Commit(context.Background(), repo, vcs.CommitRequest{
		Branch: "feature", Path: "models/dish.dmn", Content: []byte("<xml/>"), Message: "edit", PrevSHA: "old-blob",
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if res.BlobSHA != "blob10" {
		t.Errorf("blob = %q, want blob10", res.BlobSHA)
	}
}

func TestCommit_staleSHAIsConflict(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /repos/pblumer/temis/contents/models/dish.dmn", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = fmt.Fprint(w, `{"message":"dish.dmn does not match abc"}`)
	})
	c := newClient(t, mux)

	_, err := c.Commit(context.Background(), repo, vcs.CommitRequest{
		Branch: "feature", Path: "models/dish.dmn", Content: []byte("<xml/>"), Message: "edit", PrevSHA: "stale",
	})
	if !errors.Is(err, vcs.ErrConflict) {
		t.Errorf("error %v does not wrap ErrConflict", err)
	}
}

func TestCommit_existingFileWithoutSHAIsConflict(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /repos/pblumer/temis/contents/models/dish.dmn", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = fmt.Fprint(w, `{"message":"Invalid request.\n\n\"sha\" wasn't supplied."}`)
	})
	c := newClient(t, mux)

	_, err := c.Commit(context.Background(), repo, vcs.CommitRequest{
		Branch: "feature", Path: "models/dish.dmn", Content: []byte("<xml/>"), Message: "create",
	})
	if !errors.Is(err, vcs.ErrConflict) {
		t.Errorf("error %v does not wrap ErrConflict", err)
	}
}

func TestOpenPullRequest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /repos/pblumer/temis/pulls", func(w http.ResponseWriter, r *http.Request) {
		body := decodeBody(t, r)
		if body["head"] != "feature" || body["base"] != "main" {
			t.Errorf("head/base = %v/%v, want feature/main", body["head"], body["base"])
		}
		if body["title"] != "Update dish" {
			t.Errorf("title = %v", body["title"])
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprint(w, `{"number":42,"html_url":"https://github.com/pblumer/temis/pull/42","state":"open"}`)
	})
	c := newClient(t, mux)

	pr, err := c.OpenPullRequest(context.Background(), repo, vcs.PullRequestRequest{
		Title: "Update dish", Body: "edits", Head: "feature", Base: "main",
	})
	if err != nil {
		t.Fatalf("OpenPullRequest: %v", err)
	}
	want := vcs.PullRequest{Number: 42, URL: "https://github.com/pblumer/temis/pull/42", State: "open"}
	if pr != want {
		t.Errorf("pr = %+v, want %+v", pr, want)
	}
}

func TestCreateBranch_defaultBaseResolvesHEAD(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/pblumer/temis/commits/HEAD", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"sha":"headsha"}`)
	})
	mux.HandleFunc("/repos/pblumer/temis/git/refs", func(w http.ResponseWriter, r *http.Request) {
		if body := decodeBody(t, r); body["sha"] != "headsha" {
			t.Errorf("sha = %v, want headsha", body["sha"])
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprint(w, `{}`)
	})
	c := newClient(t, mux)

	if _, err := c.CreateBranch(context.Background(), repo, "feature", ""); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
}

func TestCreateBranch_emptyName(t *testing.T) {
	_, err := github.New("").CreateBranch(context.Background(), repo, "", "main")
	if !errors.Is(err, vcs.ErrNotFound) {
		t.Errorf("empty name should error, got %v", err)
	}
}

func TestCreateBranch_unresolvableBase(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/pblumer/temis/commits/nope", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"message":"No commit found"}`)
	})
	c := newClient(t, mux)

	_, err := c.CreateBranch(context.Background(), repo, "feature", "nope")
	if !errors.Is(err, vcs.ErrNotFound) {
		t.Errorf("error %v does not wrap ErrNotFound", err)
	}
}

func TestCommit_guards(t *testing.T) {
	c := github.New("")
	if _, err := c.Commit(context.Background(), repo, vcs.CommitRequest{Path: "x"}); err == nil {
		t.Error("empty branch should error")
	}
	if _, err := c.Commit(context.Background(), repo, vcs.CommitRequest{Branch: "b"}); err == nil {
		t.Error("empty path should error")
	}
}

func TestOpenPullRequest_unprocessableNotConflict(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /repos/pblumer/temis/pulls", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = fmt.Fprint(w, `{"message":"No commits between main and feature"}`)
	})
	c := newClient(t, mux)

	_, err := c.OpenPullRequest(context.Background(), repo, vcs.PullRequestRequest{Head: "feature", Base: "main"})
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, vcs.ErrConflict) {
		t.Errorf("a no-commits 422 is not a conflict: %v", err)
	}
}

// TestWriterInterface asserts the client satisfies vcs.Writer.
func TestWriterInterface(t *testing.T) {
	var _ vcs.Writer = github.New("")
}
