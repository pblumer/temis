package github

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/pblumer/temis/vcs"
)

var _ vcs.Writer = (*Client)(nil)

// CreateBranch creates a branch named name pointing at the commit fromRef
// resolves to (branch, tag or commit SHA). It returns ErrConflict if the branch
// already exists.
func (c *Client) CreateBranch(ctx context.Context, repo vcs.RepoRef, name, fromRef string) (vcs.Branch, error) {
	if name == "" {
		return vcs.Branch{}, fmt.Errorf("github: %w: empty branch name", vcs.ErrNotFound)
	}
	sha, err := c.resolveCommit(ctx, repo, fromRef)
	if err != nil {
		return vcs.Branch{}, err
	}
	u := fmt.Sprintf("%s/repos/%s/%s/git/refs", c.baseURL, esc(repo.Owner), esc(repo.Name))
	reqBody, err := json.Marshal(map[string]string{"ref": "refs/heads/" + name, "sha": sha})
	if err != nil {
		return vcs.Branch{}, fmt.Errorf("github: encode create-ref: %w", err)
	}
	if _, _, err := c.send(ctx, http.MethodPost, u, "application/vnd.github+json", reqBody); err != nil {
		return vcs.Branch{}, err
	}
	return vcs.Branch{Name: name, Commit: sha}, nil
}

// Commit writes a single file on an existing branch via the contents API and
// returns the new commit and blob SHAs.
func (c *Client) Commit(ctx context.Context, repo vcs.RepoRef, req vcs.CommitRequest) (vcs.CommitResult, error) {
	if req.Branch == "" {
		return vcs.CommitResult{}, fmt.Errorf("github: %w: empty branch", vcs.ErrNotFound)
	}
	if req.Path == "" {
		return vcs.CommitResult{}, fmt.Errorf("github: %w: empty path", vcs.ErrNotFound)
	}
	payload := map[string]string{
		"message": req.Message,
		"content": base64.StdEncoding.EncodeToString(req.Content),
		"branch":  req.Branch,
	}
	if req.PrevSHA != "" {
		payload["sha"] = req.PrevSHA
	}
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return vcs.CommitResult{}, fmt.Errorf("github: encode commit: %w", err)
	}
	u := fmt.Sprintf("%s/repos/%s/%s/contents/%s", c.baseURL, esc(repo.Owner), esc(repo.Name), escPath(req.Path))
	body, _, err := c.send(ctx, http.MethodPut, u, "application/vnd.github+json", reqBody)
	if err != nil {
		return vcs.CommitResult{}, err
	}
	var resp struct {
		Content struct {
			SHA string `json:"sha"`
		} `json:"content"`
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return vcs.CommitResult{}, fmt.Errorf("github: decode commit response: %w", err)
	}
	return vcs.CommitResult{CommitSHA: resp.Commit.SHA, BlobSHA: resp.Content.SHA}, nil
}

// OpenPullRequest opens a pull request from req.Head into req.Base.
func (c *Client) OpenPullRequest(ctx context.Context, repo vcs.RepoRef, req vcs.PullRequestRequest) (vcs.PullRequest, error) {
	u := fmt.Sprintf("%s/repos/%s/%s/pulls", c.baseURL, esc(repo.Owner), esc(repo.Name))
	reqBody, err := json.Marshal(map[string]string{
		"title": req.Title,
		"body":  req.Body,
		"head":  req.Head,
		"base":  req.Base,
	})
	if err != nil {
		return vcs.PullRequest{}, fmt.Errorf("github: encode pull request: %w", err)
	}
	body, _, err := c.send(ctx, http.MethodPost, u, "application/vnd.github+json", reqBody)
	if err != nil {
		return vcs.PullRequest{}, err
	}
	var resp struct {
		Number  int    `json:"number"`
		HTMLURL string `json:"html_url"`
		State   string `json:"state"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return vcs.PullRequest{}, fmt.Errorf("github: decode pull-request response: %w", err)
	}
	return vcs.PullRequest{Number: resp.Number, URL: resp.HTMLURL, State: resp.State}, nil
}

// resolveCommit resolves a ref (branch, tag or commit SHA) to a commit SHA via
// the commits API. An empty ref means the default branch.
func (c *Client) resolveCommit(ctx context.Context, repo vcs.RepoRef, ref string) (string, error) {
	if ref == "" {
		ref = "HEAD"
	}
	u := fmt.Sprintf("%s/repos/%s/%s/commits/%s", c.baseURL, esc(repo.Owner), esc(repo.Name), escPath(ref))
	body, _, err := c.do(ctx, u, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var resp struct {
		SHA string `json:"sha"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("github: decode commit ref %q: %w", ref, err)
	}
	if resp.SHA == "" {
		return "", fmt.Errorf("github: %w: ref %q has no commit", vcs.ErrNotFound, ref)
	}
	return resp.SHA, nil
}
