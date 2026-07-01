package service

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/pblumer/temis/vcs"
	"github.com/pblumer/temis/vcs/github"
)

// headerGitToken carries the caller's own git-provider token, per request
// (auth model A): Temis never stores it and acts on the caller's behalf. It is
// distinct from the optional temisd API bearer token (WithToken), which gates
// the endpoints themselves.
const headerGitToken = "X-Git-Token"

// WithGitHubBaseURL overrides the GitHub REST API root used by the /v1/git
// endpoints (default https://api.github.com). Use it for GitHub Enterprise
// Server ("https://HOST/api/v3") or to point tests at a local server.
func WithGitHubBaseURL(baseURL string) Option {
	return func(s *Server) { s.gitBaseURL = baseURL }
}

// gitClient builds a GitHub provider for one request from the per-request token
// (empty token = anonymous, public repos only).
func (s *Server) gitClient(token string) *github.Client {
	var opts []github.Option
	if s.gitBaseURL != "" {
		opts = append(opts, github.WithBaseURL(s.gitBaseURL))
	}
	return github.New(token, opts...)
}

// gitModels builds a read+write Models bound to this server's engine for one
// request.
func (s *Server) gitModels(token string) *vcs.Models {
	c := s.gitClient(token)
	return vcs.NewModelsWithWriter(c, c, s.engine)
}

// registerGitRoutes mounts the /v1/git endpoints on mux, gated by the git scope
// (ADR-0028 §2: git covers all /v1/git/*). The git-provider token stays per
// request (X-Git-Token), separate from the temis API key.
func (s *Server) registerGitRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/git/branches", s.requireScope(ScopeGit, s.handleGitBranches))
	mux.HandleFunc("GET /v1/git/commits", s.requireScope(ScopeGit, s.handleGitCommits))
	mux.HandleFunc("GET /v1/git/models", s.requireScope(ScopeGit, s.handleGitModels))
	mux.HandleFunc("GET /v1/git/flows", s.requireScope(ScopeGit, s.handleGitFlows))
	mux.HandleFunc("POST /v1/git/load", s.requireScope(ScopeGit, s.handleGitLoad))
	mux.HandleFunc("POST /v1/git/load-flow", s.requireScope(ScopeGit, s.handleGitLoadFlow))
	mux.HandleFunc("POST /v1/git/save", s.requireScope(ScopeGit, s.handleGitSave))
	mux.HandleFunc("POST /v1/git/propose", s.requireScope(ScopeGit, s.handleGitPropose))
}

// --- request/response DTOs ---

type gitBranchesResponse struct {
	Branches []vcs.Branch `json:"branches"`
	Count    int          `json:"count"`
}

type gitCommitsResponse struct {
	Commits []vcs.Commit `json:"commits"`
	Count   int          `json:"count"`
}

type gitModelsResponse struct {
	Models []vcs.File `json:"models"`
	Count  int        `json:"count"`
}

type gitLoadRequest struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
	Ref   string `json:"ref"`
	Path  string `json:"path"`
}

// gitLoadResponse is the compiled-model response plus the git provenance the
// model came from, so a client can edit it and later save it back with the
// blob SHA for optimistic concurrency.
type gitLoadResponse struct {
	modelResponse
	Repo string `json:"repo"`
	Ref  string `json:"ref"`
	Path string `json:"path"`
	SHA  string `json:"sha,omitempty"`
}

type gitSaveRequest struct {
	Owner   string `json:"owner"`
	Repo    string `json:"repo"`
	Branch  string `json:"branch"`
	Path    string `json:"path"`
	XML     string `json:"xml"`
	Message string `json:"message"`
	PrevSHA string `json:"prevSha"`
}

type gitSaveResponse struct {
	CommitSHA string `json:"commitSha"`
	BlobSHA   string `json:"blobSha"`
	Branch    string `json:"branch"`
	Path      string `json:"path"`
}

type gitProposeRequest struct {
	Owner   string `json:"owner"`
	Repo    string `json:"repo"`
	Base    string `json:"base"`
	Branch  string `json:"branch"`
	Path    string `json:"path"`
	XML     string `json:"xml"`
	Message string `json:"message"`
	Title   string `json:"title"`
	Body    string `json:"body"`
}

type gitProposeResponse struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
	State  string `json:"state"`
	Branch string `json:"branch"`
	Base   string `json:"base"`
}

// --- handlers ---

// handleGitBranches lists a repository's branches.
func (s *Server) handleGitBranches(w http.ResponseWriter, r *http.Request) {
	repo, ok := repoFromQuery(w, r)
	if !ok {
		return
	}
	branches, err := s.gitClient(gitToken(r)).ListBranches(r.Context(), repo)
	if err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, gitBranchesResponse{Branches: branches, Count: len(branches)})
}

// handleGitCommits lists a repository's commit history at the given ref.
func (s *Server) handleGitCommits(w http.ResponseWriter, r *http.Request) {
	repo, ok := repoFromQuery(w, r)
	if !ok {
		return
	}
	commits, err := s.gitClient(gitToken(r)).ListCommits(r.Context(), repo, r.URL.Query().Get("ref"))
	if err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, gitCommitsResponse{Commits: commits, Count: len(commits)})
}

// handleGitModels lists the DMN files under dir at ref.
func (s *Server) handleGitModels(w http.ResponseWriter, r *http.Request) {
	repo, ok := repoFromQuery(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	files, err := s.gitModels(gitToken(r)).List(r.Context(), repo, q.Get("ref"), q.Get("dir"))
	if err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, gitModelsResponse{Models: files, Count: len(files)})
}

// handleGitLoad reads a DMN file from a repository at a ref, compiles and caches
// it (so the returned modelId works with every other /v1 endpoint), and reports
// its blob SHA for a later optimistic save.
func (s *Server) handleGitLoad(w http.ResponseWriter, r *http.Request) {
	var req gitLoadRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	repo, ok := repoOrError(w, req.Owner, req.Repo)
	if !ok {
		return
	}
	if req.Path == "" {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", "missing path")
		return
	}
	client := s.gitClient(gitToken(r))
	xml, err := client.ReadFile(r.Context(), repo, req.Ref, req.Path)
	if err != nil {
		writeGitError(w, err)
		return
	}
	sm, err := s.compileAndStore(r.Context(), xml)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "MALFORMED_XML", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, gitLoadResponse{
		modelResponse: modelResponse{
			ModelID:     sm.id,
			Name:        sm.name,
			Decisions:   sm.index.Decisions,
			Inputs:      sm.index.Inputs,
			Schema:      schemaOf(sm.defs, sm.index.Decisions),
			Diagnostics: toDiagnosticDTOs(sm.diags),
		},
		Repo: repo.Owner + "/" + repo.Name,
		Ref:  req.Ref,
		Path: req.Path,
		SHA:  blobSHA(r.Context(), client, repo, req.Ref, req.Path),
	})
}

// handleGitSave commits a model to a branch. The model is compiled first, so a
// document that does not parse is a 400 and never reaches the repository.
func (s *Server) handleGitSave(w http.ResponseWriter, r *http.Request) {
	var req gitSaveRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	repo, ok := repoOrError(w, req.Owner, req.Repo)
	if !ok {
		return
	}
	if req.Branch == "" || req.Path == "" || req.XML == "" {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", "missing branch, path or xml")
		return
	}
	if !s.compiles(w, r.Context(), []byte(req.XML)) {
		return
	}
	res, err := s.gitModels(gitToken(r)).Save(r.Context(), repo, req.Branch, req.Path, []byte(req.XML), req.Message, req.PrevSHA)
	if err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, gitSaveResponse{
		CommitSHA: res.CommitSHA, BlobSHA: res.BlobSHA, Branch: req.Branch, Path: req.Path,
	})
}

// handleGitPropose runs the whole branch → commit → pull-request flow: it
// creates a branch off the base, commits the (compile-validated) model to it and
// opens a pull request back into the base.
func (s *Server) handleGitPropose(w http.ResponseWriter, r *http.Request) {
	var req gitProposeRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	repo, ok := repoOrError(w, req.Owner, req.Repo)
	if !ok {
		return
	}
	if req.Base == "" || req.Branch == "" || req.Path == "" || req.XML == "" {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", "missing base, branch, path or xml")
		return
	}
	if !s.compiles(w, r.Context(), []byte(req.XML)) {
		return
	}
	pr, err := s.gitModels(gitToken(r)).Propose(r.Context(), repo, vcs.Proposal{
		Path:    req.Path,
		XML:     []byte(req.XML),
		Base:    req.Base,
		Branch:  req.Branch,
		Message: req.Message,
		Title:   req.Title,
		Body:    req.Body,
	})
	if err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, gitProposeResponse{
		Number: pr.Number, URL: pr.URL, State: pr.State, Branch: req.Branch, Base: req.Base,
	})
}

// --- helpers ---

// gitToken returns the caller's per-request git token from the X-Git-Token
// header (empty when absent: anonymous access).
func gitToken(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get(headerGitToken))
}

// repoFromQuery parses owner and repo from the query string, writing a 400 and
// returning ok=false when either is missing.
func repoFromQuery(w http.ResponseWriter, r *http.Request) (vcs.RepoRef, bool) {
	q := r.URL.Query()
	return repoOrError(w, q.Get("owner"), q.Get("repo"))
}

// repoOrError builds a RepoRef, writing a 400 and returning ok=false when owner
// or repo is empty.
func repoOrError(w http.ResponseWriter, owner, name string) (vcs.RepoRef, bool) {
	if owner == "" || name == "" {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", "missing owner or repo")
		return vcs.RepoRef{}, false
	}
	return vcs.RepoRef{Owner: owner, Name: name}, true
}

// compiles reports whether xml compiles, writing a 400 MALFORMED_XML when it
// does not (so a malformed document never reaches a commit).
func (s *Server) compiles(w http.ResponseWriter, ctx context.Context, xml []byte) bool {
	if _, _, err := s.engine.Compile(ctx, xml); err != nil {
		writeProblem(w, http.StatusBadRequest, "MALFORMED_XML", err.Error())
		return false
	}
	return true
}

// blobSHA returns the blob SHA of path at ref, or "" when it cannot be resolved
// (best-effort: a missing SHA only costs the client optimistic concurrency on a
// later save, so a lookup failure must not fail the load).
func blobSHA(ctx context.Context, c *github.Client, repo vcs.RepoRef, ref, path string) string {
	entries, err := c.ListFiles(ctx, repo, ref, path)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.Path == path && !e.IsDir {
			return e.SHA
		}
	}
	return ""
}

// writeGitError maps a vcs/provider error to an RFC-7807 problem with a stable
// code.
func writeGitError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, vcs.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "GIT_NOT_FOUND", err.Error())
	case errors.Is(err, vcs.ErrUnauthorized):
		writeProblem(w, http.StatusUnauthorized, "GIT_UNAUTHORIZED", err.Error())
	case errors.Is(err, vcs.ErrConflict):
		writeProblem(w, http.StatusConflict, "GIT_CONFLICT", err.Error())
	default:
		writeProblem(w, http.StatusBadGateway, "GIT_UPSTREAM_ERROR", err.Error())
	}
}
