package mcp

import (
	"context"
	"encoding/json"

	"github.com/pblumer/temis/dmn"
	"github.com/pblumer/temis/vcs"
	"github.com/pblumer/temis/vcs/github"
)

// WithGitHubBaseURL overrides the GitHub REST API root used by the git_* tools
// (default https://api.github.com). Use it for GitHub Enterprise Server
// ("https://HOST/api/v3") or to point tests at a local server.
func WithGitHubBaseURL(baseURL string) Option {
	return func(s *Server) { s.gitBaseURL = baseURL }
}

// gitClient builds a GitHub provider for one tool call from the per-call token
// (empty token = anonymous, public repos only).
func (s *Server) gitClient(token string) *github.Client {
	var opts []github.Option
	if s.gitBaseURL != "" {
		opts = append(opts, github.WithBaseURL(s.gitBaseURL))
	}
	return github.New(token, opts...)
}

// gitTools is the git-backed slice of the tool catalogue, registered into the
// advertised tools at init so it appears in tools/list alongside the rest.
var gitTools = []toolSpec{
	{
		Name: "git_list_models",
		Description: "List the DMN models (*.dmn files) in a git repository at a ref " +
			"(branch, tag or commit). Use this to discover which decision models live " +
			"in a repo before loading one.",
		InputSchema: obj(map[string]any{
			"owner":    str("The repository owner (GitHub user or organisation)."),
			"repo":     str("The repository name."),
			"ref":      str("Branch, tag or commit SHA to read at. Empty = the default branch."),
			"dir":      str("Directory to list (slash-separated). Empty = repository root."),
			"gitToken": str("The caller's git-provider token, for private repos / higher rate limits. Optional for public repos."),
		}, "owner", "repo"),
	},
	{
		Name: "git_load_model",
		Description: "Read a DMN model from a git repository at a ref and compile+cache it, " +
			"returning a modelId you can then pass to describe_decision and evaluate. " +
			"This is how you evaluate a decision that lives in version control at a " +
			"specific branch or commit (reproducible).",
		InputSchema: obj(map[string]any{
			"owner":    str("The repository owner."),
			"repo":     str("The repository name."),
			"ref":      str("Branch, tag or commit SHA to read at. Empty = the default branch."),
			"path":     str("Path to the .dmn file in the repository (slash-separated)."),
			"gitToken": str("The caller's git-provider token. Optional for public repos."),
		}, "owner", "repo", "path"),
	},
	{
		Name: "git_propose",
		Description: "Propose an edited DMN model as a pull request: create a branch off " +
			"the base, commit the model to it and open a pull request back into the base. " +
			"The model is compiled first, so a document that does not parse is rejected " +
			"and never committed. Merging is left to the repository's review process.",
		InputSchema: obj(map[string]any{
			"owner":    str("The repository owner."),
			"repo":     str("The repository name."),
			"base":     str("The branch to branch off and open the pull request against (e.g. main)."),
			"branch":   str("The new branch name to create and commit on."),
			"path":     str("Path to the .dmn file to write (slash-separated)."),
			"xml":      str("The DMN 1.5 XML to commit."),
			"message":  str("Commit message."),
			"title":    str("Pull-request title."),
			"body":     str("Pull-request description. Optional."),
			"gitToken": str("The caller's git-provider token (needs write/PR scope)."),
		}, "owner", "repo", "base", "branch", "path", "xml", "title"),
	},
}

func init() { tools = append(tools, gitTools...) }

func (s *Server) toolGitListModels(ctx context.Context, raw json.RawMessage) (any, *rpcError) {
	var a struct {
		Owner, Repo, Ref, Dir, GitToken string
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return toolError("invalid arguments: " + err.Error()), nil
	}
	if a.Owner == "" || a.Repo == "" {
		return toolError("missing required argument: owner and repo"), nil
	}
	repo := vcs.RepoRef{Owner: a.Owner, Name: a.Repo}
	models := vcs.NewModels(s.gitClient(a.GitToken), nil)
	files, err := models.List(ctx, repo, a.Ref, a.Dir)
	if err != nil {
		return toolError("git: " + err.Error()), nil
	}
	return toolText(map[string]any{"models": files, "count": len(files)})
}

func (s *Server) toolGitLoadModel(ctx context.Context, raw json.RawMessage) (any, *rpcError) {
	var a struct {
		Owner, Repo, Ref, Path, GitToken string
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return toolError("invalid arguments: " + err.Error()), nil
	}
	if a.Owner == "" || a.Repo == "" {
		return toolError("missing required argument: owner and repo"), nil
	}
	if a.Path == "" {
		return toolError("missing required argument: path"), nil
	}
	repo := vcs.RepoRef{Owner: a.Owner, Name: a.Repo}
	xml, err := s.gitClient(a.GitToken).ReadFile(ctx, repo, a.Ref, a.Path)
	if err != nil {
		return toolError("git: " + err.Error()), nil
	}
	id, _, index, diags, err := s.store.Compile(ctx, xml)
	if err != nil {
		return toolError("could not compile model: " + err.Error()), nil
	}
	return toolText(map[string]any{
		"modelId":     id,
		"decisions":   index.Decisions,
		"inputs":      index.Inputs,
		"repo":        a.Owner + "/" + a.Repo,
		"ref":         a.Ref,
		"path":        a.Path,
		"diagnostics": toDiagnosticDTOs(diags),
	})
}

func (s *Server) toolGitPropose(ctx context.Context, raw json.RawMessage) (any, *rpcError) {
	var a struct {
		Owner, Repo, Base, Branch, Path, XML, Message, Title, Body, GitToken string
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return toolError("invalid arguments: " + err.Error()), nil
	}
	switch {
	case a.Owner == "" || a.Repo == "":
		return toolError("missing required argument: owner and repo"), nil
	case a.Base == "" || a.Branch == "" || a.Path == "" || a.XML == "":
		return toolError("missing required argument: base, branch, path and xml"), nil
	}
	c := s.gitClient(a.GitToken)
	models := vcs.NewModelsWithWriter(c, c, dmn.New())
	pr, err := models.Propose(ctx, vcs.RepoRef{Owner: a.Owner, Name: a.Repo}, vcs.Proposal{
		Path:    a.Path,
		XML:     []byte(a.XML),
		Base:    a.Base,
		Branch:  a.Branch,
		Message: a.Message,
		Title:   a.Title,
		Body:    a.Body,
	})
	if err != nil {
		return toolError("git: " + err.Error()), nil
	}
	return toolText(map[string]any{
		"number": pr.Number,
		"url":    pr.URL,
		"state":  pr.State,
		"branch": a.Branch,
		"base":   a.Base,
	})
}
