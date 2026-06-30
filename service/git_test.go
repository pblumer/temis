package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// gitTestServer returns a temisd handler whose /v1/git endpoints talk to gh (a
// fake GitHub) instead of the real API.
func gitTestServer(t *testing.T, gh http.Handler) http.Handler {
	t.Helper()
	fake := httptest.NewServer(gh)
	t.Cleanup(fake.Close)
	return NewServer(nil, WithGitHubBaseURL(fake.URL)).Handler()
}

// doGit issues a request to the temisd handler with a per-request git token.
func doGit(t *testing.T, h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("X-Git-Token", token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// dishContentsHandler serves the dish model at models/dish.dmn: raw bytes for a
// raw Accept (ReadFile), a JSON object otherwise (ListFiles / blob-sha lookup).
func dishContentsHandler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Accept"), "raw") {
			_, _ = w.Write(dishXML(t))
			return
		}
		_, _ = fmt.Fprint(w, `{"name":"dish.dmn","path":"models/dish.dmn","sha":"blobABC","size":10,"type":"file"}`)
	}
}

func TestGitBranches_forwardsTokenAndLists(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/pblumer/temis/branches", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer ghp_test" {
			t.Errorf("Authorization = %q, want bearer ghp_test", got)
		}
		_, _ = fmt.Fprint(w, `[{"name":"main","commit":{"sha":"abc"}},{"name":"release","commit":{"sha":"def"}}]`)
	})
	h := gitTestServer(t, mux)

	rec := doGit(t, h, "GET", "/v1/git/branches?owner=pblumer&repo=temis", "ghp_test", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
	resp := decode[gitBranchesResponse](t, rec)
	if resp.Count != 2 || resp.Branches[0].Name != "main" {
		t.Errorf("branches = %+v", resp)
	}
}

func TestGitBranches_missingRepoIs400(t *testing.T) {
	h := gitTestServer(t, http.NewServeMux())
	rec := doGit(t, h, "GET", "/v1/git/branches?owner=pblumer", "tok", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestGitCommits(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/pblumer/temis/commits", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("sha") != "main" {
			t.Errorf("sha = %q, want main", r.URL.Query().Get("sha"))
		}
		_, _ = fmt.Fprint(w, `[{"sha":"c1","commit":{"message":"init","author":{"name":"Pat","date":"2026-06-30T00:00:00Z"}}}]`)
	})
	h := gitTestServer(t, mux)

	rec := doGit(t, h, "GET", "/v1/git/commits?owner=pblumer&repo=temis&ref=main", "tok", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
	resp := decode[gitCommitsResponse](t, rec)
	if resp.Count != 1 || resp.Commits[0].Author != "Pat" {
		t.Errorf("commits = %+v", resp)
	}
}

func TestGitModels_filtersToDMN(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/pblumer/temis/contents/models", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `[
			{"name":"dish.dmn","path":"models/dish.dmn","sha":"b1","type":"file"},
			{"name":"readme.md","path":"models/readme.md","type":"file"},
			{"name":"sub","path":"models/sub","type":"dir"}
		]`)
	})
	h := gitTestServer(t, mux)

	rec := doGit(t, h, "GET", "/v1/git/models?owner=pblumer&repo=temis&dir=models", "tok", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
	resp := decode[gitModelsResponse](t, rec)
	if resp.Count != 1 || resp.Models[0].Path != "models/dish.dmn" {
		t.Errorf("models = %+v, want only dish.dmn", resp)
	}
}

func TestGitLoad_cachesAndIsEvaluable(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/pblumer/temis/contents/models/dish.dmn", dishContentsHandler(t))
	h := gitTestServer(t, mux)

	rec := doGit(t, h, "POST", "/v1/git/load", "tok",
		`{"owner":"pblumer","repo":"temis","ref":"main","path":"models/dish.dmn"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("load status = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
	resp := decode[gitLoadResponse](t, rec)
	if !strings.HasPrefix(resp.ModelID, "sha256:") {
		t.Errorf("modelId = %q", resp.ModelID)
	}
	if resp.SHA != "blobABC" {
		t.Errorf("sha = %q, want blobABC", resp.SHA)
	}
	if resp.Repo != "pblumer/temis" || resp.Path != "models/dish.dmn" {
		t.Errorf("provenance = %+v", resp)
	}

	// The loaded model is cached, so it evaluates by id through the normal route.
	ev := doGit(t, h, "POST", "/v1/models/"+resp.ModelID+"/evaluate", "",
		`{"decision":"Dish","input":{"Season":"Winter","Guest Count":8}}`)
	if ev.Code != http.StatusOK {
		t.Fatalf("evaluate status = %d, want 200 (body %s)", ev.Code, ev.Body)
	}
	out := decode[evaluateResponse](t, ev)
	if out.Outputs["Dish"] != "Roastbeef" {
		t.Errorf("Dish = %v, want Roastbeef", out.Outputs["Dish"])
	}
}

func TestGitLoad_notFoundMaps404(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/pblumer/temis/contents/models/missing.dmn", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"message":"Not Found"}`)
	})
	h := gitTestServer(t, mux)

	rec := doGit(t, h, "POST", "/v1/git/load", "tok",
		`{"owner":"pblumer","repo":"temis","ref":"main","path":"models/missing.dmn"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body %s)", rec.Code, rec.Body)
	}
}

func TestGitSave_commitsModel(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /repos/pblumer/temis/contents/models/dish.dmn", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer ghp_w" {
			t.Errorf("Authorization = %q", got)
		}
		_, _ = fmt.Fprint(w, `{"content":{"sha":"blob2"},"commit":{"sha":"commit2"}}`)
	})
	h := gitTestServer(t, mux)

	body := fmt.Sprintf(`{"owner":"pblumer","repo":"temis","branch":"edit","path":"models/dish.dmn","message":"edit","prevSha":"blobABC","xml":%s}`,
		jsonString(string(dishXML(t))))
	rec := doGit(t, h, "POST", "/v1/git/save", "ghp_w", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
	resp := decode[gitSaveResponse](t, rec)
	if resp.CommitSHA != "commit2" || resp.BlobSHA != "blob2" {
		t.Errorf("save response = %+v", resp)
	}
}

func TestGitSave_malformedIs400(t *testing.T) {
	h := gitTestServer(t, http.NewServeMux())
	rec := doGit(t, h, "POST", "/v1/git/save", "tok",
		`{"owner":"pblumer","repo":"temis","branch":"b","path":"x.dmn","xml":"not xml"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body)
	}
}

func TestGitSave_conflictMaps409(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /repos/pblumer/temis/contents/models/dish.dmn", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = fmt.Fprint(w, `{"message":"dish.dmn does not match abc"}`)
	})
	h := gitTestServer(t, mux)

	body := fmt.Sprintf(`{"owner":"pblumer","repo":"temis","branch":"edit","path":"models/dish.dmn","message":"edit","prevSha":"stale","xml":%s}`,
		jsonString(string(dishXML(t))))
	rec := doGit(t, h, "POST", "/v1/git/save", "tok", body)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (body %s)", rec.Code, rec.Body)
	}
}

func TestGitPropose_fullFlow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/pblumer/temis/contents/models/dish.dmn", dishContentsHandler(t)) // base sha resolution
	mux.HandleFunc("/repos/pblumer/temis/commits/main", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"sha":"basesha"}`)
	})
	mux.HandleFunc("POST /repos/pblumer/temis/git/refs", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprint(w, `{}`)
	})
	mux.HandleFunc("PUT /repos/pblumer/temis/contents/models/dish.dmn", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"content":{"sha":"b2"},"commit":{"sha":"c2"}}`)
	})
	mux.HandleFunc("POST /repos/pblumer/temis/pulls", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprint(w, `{"number":99,"html_url":"https://github.com/pblumer/temis/pull/99","state":"open"}`)
	})
	h := gitTestServer(t, mux)

	body := fmt.Sprintf(`{"owner":"pblumer","repo":"temis","base":"main","branch":"edit-dish","path":"models/dish.dmn","message":"tweak","title":"Update dish","xml":%s}`,
		jsonString(string(dishXML(t))))
	rec := doGit(t, h, "POST", "/v1/git/propose", "tok", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
	resp := decode[gitProposeResponse](t, rec)
	if resp.Number != 99 || resp.Base != "main" || resp.Branch != "edit-dish" {
		t.Errorf("propose response = %+v", resp)
	}
}

func TestGitBranches_upstreamErrorMaps502(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/pblumer/temis/branches", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, "boom")
	})
	h := gitTestServer(t, mux)

	rec := doGit(t, h, "GET", "/v1/git/branches?owner=pblumer&repo=temis", "tok", "")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 (body %s)", rec.Code, rec.Body)
	}
}

// jsonString returns s encoded as a JSON string literal (quotes and escaping).
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
