package service

import (
	"net/http"

	"github.com/pblumer/temis/flow"
	"github.com/pblumer/temis/vcs"
)

// gitFlowsResponse lists the decision-flow descriptors found in a repository.
type gitFlowsResponse struct {
	Flows []vcs.File `json:"flows"`
	Count int        `json:"count"`
}

// gitLoadFlowResponse is the registered-flow response plus the git provenance the
// descriptor came from.
type gitLoadFlowResponse struct {
	flowResponse
	Repo string `json:"repo"`
	Ref  string `json:"ref"`
	Path string `json:"path"`
	SHA  string `json:"sha,omitempty"`
}

// handleGitFlows lists the *.flow.json descriptors under dir at ref (WP-94).
func (s *Server) handleGitFlows(w http.ResponseWriter, r *http.Request) {
	repo, ok := repoFromQuery(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	files, err := s.gitModels(gitToken(r)).ListFlows(r.Context(), repo, q.Get("ref"), q.Get("dir"))
	if err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, gitFlowsResponse{Flows: files, Count: len(files)})
}

// handleGitLoadFlow reads a flow descriptor from a repository at a ref, compiles
// and registers it (so the returned flowId works with the /v1/flows endpoints),
// and reports the diagnostics from validating it against the loaded models. The
// referenced models must be loaded separately (e.g. via /v1/git/load).
func (s *Server) handleGitLoadFlow(w http.ResponseWriter, r *http.Request) {
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
	desc, err := client.ReadFile(r.Context(), repo, req.Ref, req.Path)
	if err != nil {
		writeGitError(w, err)
		return
	}
	f, _, err := flow.Compile(desc)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "FLOW_MALFORMED", err.Error())
		return
	}
	diags := f.Validate(r.Context(), cacheResolver{s})
	id := flowID(desc)
	s.flows.put(&storedFlow{id: id, flow: f, desc: desc, diags: diags})
	writeJSON(w, http.StatusOK, gitLoadFlowResponse{
		flowResponse: flowResponse{
			FlowID:      id,
			Name:        f.Name(),
			Diagnostics: toFlowDiagnosticDTOs(diags),
		},
		Repo: repo.Owner + "/" + repo.Name,
		Ref:  req.Ref,
		Path: req.Path,
		SHA:  blobSHA(r.Context(), client, repo, req.Ref, req.Path),
	})
}
