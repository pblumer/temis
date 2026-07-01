package service

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// TestGitCommits_missingRepoIs400 covers handleGitCommits's repoFromQuery miss.
func TestGitCommits_missingRepoIs400(t *testing.T) {
	h := gitTestServer(t, http.NewServeMux())
	if rec := doGit(t, h, "GET", "/v1/git/commits?owner=pblumer", "tok", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// TestGitCommits_upstreamError covers handleGitCommits's error branch.
func TestGitCommits_upstreamError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/pblumer/temis/commits", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	h := gitTestServer(t, mux)
	if rec := doGit(t, h, "GET", "/v1/git/commits?owner=pblumer&repo=temis", "tok", ""); rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
}

// TestGitModels_missingRepoIs400 covers handleGitModels's repoFromQuery miss.
func TestGitModels_missingRepoIs400(t *testing.T) {
	h := gitTestServer(t, http.NewServeMux())
	if rec := doGit(t, h, "GET", "/v1/git/models?repo=temis", "tok", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// TestGitModels_upstreamError covers handleGitModels's error branch.
func TestGitModels_upstreamError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/pblumer/temis/contents/models", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	h := gitTestServer(t, mux)
	if rec := doGit(t, h, "GET", "/v1/git/models?owner=pblumer&repo=temis&dir=models", "tok", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (unauthorized maps via writeGitError)", rec.Code)
	}
}

// TestGitLoad_badJSON covers handleGitLoad's decode error.
func TestGitLoad_badJSON(t *testing.T) {
	h := gitTestServer(t, http.NewServeMux())
	if rec := doGit(t, h, "POST", "/v1/git/load", "tok", "{not json"); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// TestGitLoad_missingOwner covers handleGitLoad's repoOrError miss.
func TestGitLoad_missingOwner(t *testing.T) {
	h := gitTestServer(t, http.NewServeMux())
	if rec := doGit(t, h, "POST", "/v1/git/load", "tok", `{"repo":"temis","path":"x.dmn"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// TestGitLoad_missingPath covers handleGitLoad's "missing path" branch.
func TestGitLoad_missingPath(t *testing.T) {
	h := gitTestServer(t, http.NewServeMux())
	if rec := doGit(t, h, "POST", "/v1/git/load", "tok", `{"owner":"pblumer","repo":"temis"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// TestGitLoad_malformedXML covers handleGitLoad's MALFORMED_XML branch: the file
// reads but does not compile.
func TestGitLoad_malformedXML(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/pblumer/temis/contents/models/bad.dmn", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<not-dmn>"))
	})
	h := gitTestServer(t, mux)
	rec := doGit(t, h, "POST", "/v1/git/load", "tok",
		`{"owner":"pblumer","repo":"temis","ref":"main","path":"models/bad.dmn"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body)
	}
	if p := decode[problem](t, rec); p.Code != "MALFORMED_XML" {
		t.Errorf("code = %q, want MALFORMED_XML", p.Code)
	}
}

// TestGitLoad_blobSHAUnresolved covers blobSHA's best-effort empty return: the
// raw read succeeds (so the model loads) but the contents lookup for the SHA
// fails, leaving SHA empty without failing the load.
func TestGitLoad_blobSHAUnresolved(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/pblumer/temis/contents/models/dish.dmn", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Accept"), "raw") {
			_, _ = w.Write(dishXML(t))
			return
		}
		// Non-raw (the SHA lookup): fail so blobSHA returns "".
		w.WriteHeader(http.StatusInternalServerError)
	})
	h := gitTestServer(t, mux)
	rec := doGit(t, h, "POST", "/v1/git/load", "tok",
		`{"owner":"pblumer","repo":"temis","ref":"main","path":"models/dish.dmn"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
	if resp := decode[gitLoadResponse](t, rec); resp.SHA != "" {
		t.Errorf("sha = %q, want empty (lookup failed, best-effort)", resp.SHA)
	}
}

// TestGitLoad_blobSHANoMatch covers blobSHA's loop falling through without a
// matching entry: the SHA lookup returns entries, but none has the requested
// path (so SHA stays empty though the load still succeeds).
func TestGitLoad_blobSHANoMatch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/pblumer/temis/contents/models/dish.dmn", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Accept"), "raw") {
			_, _ = w.Write(dishXML(t))
			return
		}
		// A directory listing whose only entry is a different path: no match.
		_, _ = fmt.Fprint(w, `[{"name":"other.dmn","path":"models/other.dmn","sha":"sX","type":"file"}]`)
	})
	h := gitTestServer(t, mux)
	rec := doGit(t, h, "POST", "/v1/git/load", "tok",
		`{"owner":"pblumer","repo":"temis","ref":"main","path":"models/dish.dmn"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
	if resp := decode[gitLoadResponse](t, rec); resp.SHA != "" {
		t.Errorf("sha = %q, want empty (no matching entry)", resp.SHA)
	}
}

// TestGitSave_badJSON covers handleGitSave's decode error.
func TestGitSave_badJSON(t *testing.T) {
	h := gitTestServer(t, http.NewServeMux())
	if rec := doGit(t, h, "POST", "/v1/git/save", "tok", "{not json"); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// TestGitSave_missingOwner covers handleGitSave's repoOrError miss.
func TestGitSave_missingOwner(t *testing.T) {
	h := gitTestServer(t, http.NewServeMux())
	if rec := doGit(t, h, "POST", "/v1/git/save", "tok", `{"repo":"temis","branch":"b","path":"x.dmn","xml":"<x/>"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// TestGitSave_missingFields covers handleGitSave's "missing branch/path/xml" branch.
func TestGitSave_missingFields(t *testing.T) {
	h := gitTestServer(t, http.NewServeMux())
	if rec := doGit(t, h, "POST", "/v1/git/save", "tok", `{"owner":"pblumer","repo":"temis"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// TestGitSave_upstreamError covers handleGitSave's Save error branch with a
// compile-valid model that the provider rejects.
func TestGitSave_upstreamError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /repos/pblumer/temis/contents/models/dish.dmn", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	h := gitTestServer(t, mux)
	body := fmt.Sprintf(`{"owner":"pblumer","repo":"temis","branch":"edit","path":"models/dish.dmn","message":"e","xml":%s}`,
		jsonString(string(dishXML(t))))
	if rec := doGit(t, h, "POST", "/v1/git/save", "tok", body); rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 (body %s)", rec.Code, rec.Body)
	}
}

// TestGitPropose_badJSON covers handleGitPropose's decode error.
func TestGitPropose_badJSON(t *testing.T) {
	h := gitTestServer(t, http.NewServeMux())
	if rec := doGit(t, h, "POST", "/v1/git/propose", "tok", "{not json"); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// TestGitPropose_missingOwner covers handleGitPropose's repoOrError miss.
func TestGitPropose_missingOwner(t *testing.T) {
	h := gitTestServer(t, http.NewServeMux())
	if rec := doGit(t, h, "POST", "/v1/git/propose", "tok", `{"repo":"temis","base":"main","branch":"b","path":"x.dmn","xml":"<x/>"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// TestGitPropose_missingFields covers handleGitPropose's "missing base/branch/path/xml" branch.
func TestGitPropose_missingFields(t *testing.T) {
	h := gitTestServer(t, http.NewServeMux())
	if rec := doGit(t, h, "POST", "/v1/git/propose", "tok", `{"owner":"pblumer","repo":"temis"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// TestGitPropose_malformedXML covers handleGitPropose's compiles() reject branch.
func TestGitPropose_malformedXML(t *testing.T) {
	h := gitTestServer(t, http.NewServeMux())
	body := `{"owner":"pblumer","repo":"temis","base":"main","branch":"b","path":"x.dmn","xml":"not xml"}`
	if rec := doGit(t, h, "POST", "/v1/git/propose", "tok", body); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// TestGitPropose_upstreamError covers handleGitPropose's Propose error branch.
func TestGitPropose_upstreamError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/pblumer/temis/commits/main", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	h := gitTestServer(t, mux)
	body := fmt.Sprintf(`{"owner":"pblumer","repo":"temis","base":"main","branch":"b","path":"models/dish.dmn","message":"m","xml":%s}`,
		jsonString(string(dishXML(t))))
	if rec := doGit(t, h, "POST", "/v1/git/propose", "tok", body); rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 (body %s)", rec.Code, rec.Body)
	}
}
