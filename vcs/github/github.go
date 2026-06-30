// Package github implements the vcs.Reader interface against the GitHub REST
// API using only the standard library — no new dependency (ADR-0022, Golden
// Rule 6). It is the first concrete git provider for Temis; because callers
// depend on the vcs.Reader interface, swapping or adding providers (GitHub
// Enterprise, a pure-Go git library, GitLab) does not touch them.
//
// Authentication is an optional bearer token (a GitHub personal access token or
// installation token). Public repositories work without one, subject to the
// stricter unauthenticated rate limit.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/pblumer/temis/vcs"
)

// defaultBaseURL is the public GitHub REST API root. For GitHub Enterprise
// Server pass WithBaseURL("https://HOST/api/v3").
const defaultBaseURL = "https://api.github.com"

// apiVersion pins the REST API version GitHub should serve, per its
// X-GitHub-Api-Version header convention.
const apiVersion = "2022-11-28"

// Client is a vcs.Reader backed by the GitHub REST API. It is safe for
// concurrent use. Construct one with New; the zero value is not usable.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// Option configures a Client at construction time.
type Option func(*Client)

// WithBaseURL overrides the API root (default https://api.github.com). Use it
// for GitHub Enterprise Server ("https://HOST/api/v3") or to point tests at a
// local server. A trailing slash is trimmed.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		if baseURL != "" {
			c.baseURL = strings.TrimRight(baseURL, "/")
		}
	}
}

// WithHTTPClient sets the underlying HTTP client (for custom timeouts,
// transports or proxies). A nil client is ignored.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			c.http = h
		}
	}
}

// New returns a GitHub Client. An empty token accesses public repositories
// anonymously; a token (personal access or installation token) is sent as a
// bearer credential for private repositories and higher rate limits.
func New(token string, opts ...Option) *Client {
	c := &Client{
		baseURL: defaultBaseURL,
		token:   token,
		http:    http.DefaultClient,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

var _ vcs.Reader = (*Client)(nil)

// --- wire types (only the fields we use) ---

type ghBranch struct {
	Name   string `json:"name"`
	Commit struct {
		SHA string `json:"sha"`
	} `json:"commit"`
}

type ghCommit struct {
	SHA    string `json:"sha"`
	Commit struct {
		Message string `json:"message"`
		Author  struct {
			Name string `json:"name"`
			Date string `json:"date"`
		} `json:"author"`
	} `json:"commit"`
}

type ghContent struct {
	Name string `json:"name"`
	Path string `json:"path"`
	SHA  string `json:"sha"`
	Size int64  `json:"size"`
	Type string `json:"type"` // "file" or "dir"
}

// --- Reader implementation ---

// ListBranches returns every branch of the repository, following pagination so
// the list is complete.
func (c *Client) ListBranches(ctx context.Context, repo vcs.RepoRef) ([]vcs.Branch, error) {
	u := fmt.Sprintf("%s/repos/%s/%s/branches?per_page=100", c.baseURL, esc(repo.Owner), esc(repo.Name))
	raw, err := getPaged[ghBranch](ctx, c, u)
	if err != nil {
		return nil, err
	}
	out := make([]vcs.Branch, len(raw))
	for i, b := range raw {
		out[i] = vcs.Branch{Name: b.Name, Commit: b.Commit.SHA}
	}
	return out, nil
}

// ListCommits returns the commit history reachable from ref (newest first),
// following pagination. An empty ref means the default branch.
func (c *Client) ListCommits(ctx context.Context, repo vcs.RepoRef, ref string) ([]vcs.Commit, error) {
	q := url.Values{"per_page": {"100"}}
	if ref != "" {
		q.Set("sha", ref)
	}
	u := fmt.Sprintf("%s/repos/%s/%s/commits?%s", c.baseURL, esc(repo.Owner), esc(repo.Name), q.Encode())
	raw, err := getPaged[ghCommit](ctx, c, u)
	if err != nil {
		return nil, err
	}
	out := make([]vcs.Commit, len(raw))
	for i, cm := range raw {
		out[i] = vcs.Commit{
			SHA:     cm.SHA,
			Message: cm.Commit.Message,
			Author:  cm.Commit.Author.Name,
			Date:    cm.Commit.Author.Date,
		}
	}
	return out, nil
}

// ListFiles returns the entries directly under dir at ref via the contents
// API. The contents endpoint returns up to 1000 entries per directory; larger
// trees would need the Git Trees API, which is out of scope for now.
func (c *Client) ListFiles(ctx context.Context, repo vcs.RepoRef, ref, dir string) ([]vcs.File, error) {
	u := c.contentsURL(repo, ref, dir)
	body, _, err := c.do(ctx, u, "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	// A directory yields a JSON array; a file yields a single object.
	var entries []ghContent
	if err := json.Unmarshal(body, &entries); err != nil {
		var one ghContent
		if err2 := json.Unmarshal(body, &one); err2 != nil {
			return nil, fmt.Errorf("github: decode contents of %q: %w", dir, err)
		}
		entries = []ghContent{one}
	}
	out := make([]vcs.File, len(entries))
	for i, e := range entries {
		out[i] = vcs.File{
			Path:  e.Path,
			Name:  e.Name,
			Size:  e.Size,
			SHA:   e.SHA,
			IsDir: e.Type == "dir",
		}
	}
	return out, nil
}

// ReadFile returns the raw bytes of the file at path and ref using the raw
// media type, so the content arrives undecoded (no base64 round-trip).
func (c *Client) ReadFile(ctx context.Context, repo vcs.RepoRef, ref, path string) ([]byte, error) {
	if path == "" {
		return nil, fmt.Errorf("github: %w: empty path", vcs.ErrNotFound)
	}
	u := c.contentsURL(repo, ref, path)
	body, _, err := c.do(ctx, u, "application/vnd.github.raw")
	if err != nil {
		return nil, err
	}
	return body, nil
}

// contentsURL builds a /contents URL for a path at an optional ref.
func (c *Client) contentsURL(repo vcs.RepoRef, ref, path string) string {
	u := fmt.Sprintf("%s/repos/%s/%s/contents/%s", c.baseURL, esc(repo.Owner), esc(repo.Name), escPath(path))
	if ref != "" {
		u += "?ref=" + url.QueryEscape(ref)
	}
	return u
}

// --- HTTP plumbing ---

// do issues a GET to urlStr with the given Accept header. It is the read path;
// writes use send with an explicit method and body.
func (c *Client) do(ctx context.Context, urlStr, accept string) ([]byte, *http.Response, error) {
	return c.send(ctx, http.MethodGet, urlStr, accept, nil)
}

// send issues an HTTP request and returns the body and response. A non-nil
// reqBody is sent as JSON. Non-2xx responses are mapped to vcs sentinel errors
// where meaningful (404 → ErrNotFound, 401/403 → ErrUnauthorized, 409 →
// ErrConflict).
func (c *Client) send(ctx context.Context, method, urlStr, accept string, reqBody []byte) ([]byte, *http.Response, error) {
	var bodyReader io.Reader
	if reqBody != nil {
		bodyReader = bytes.NewReader(reqBody)
	}
	req, err := http.NewRequestWithContext(ctx, method, urlStr, bodyReader)
	if err != nil {
		return nil, nil, fmt.Errorf("github: build request: %w", err)
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("github: request %s: %w", urlStr, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp, fmt.Errorf("github: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp, statusError(resp.StatusCode, body)
	}
	return body, resp, nil
}

// statusError maps a GitHub error status to a vcs sentinel error, carrying the
// API's message when present.
func statusError(code int, body []byte) error {
	msg := apiMessage(body)
	switch code {
	case http.StatusNotFound:
		return fmt.Errorf("github: %w: %s", vcs.ErrNotFound, msg)
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("github: %w (status %d): %s", vcs.ErrUnauthorized, code, msg)
	case http.StatusConflict:
		return fmt.Errorf("github: %w: %s", vcs.ErrConflict, msg)
	case http.StatusUnprocessableEntity:
		// GitHub reports several optimistic-concurrency / already-exists
		// failures as 422 rather than 409 (creating an existing ref or file,
		// a stale or missing blob sha). Treat those as conflicts; anything
		// else 422 stays a generic error.
		if isConflictMessage(msg) {
			return fmt.Errorf("github: %w: %s", vcs.ErrConflict, msg)
		}
		return fmt.Errorf("github: unexpected status %d: %s", code, msg)
	default:
		return fmt.Errorf("github: unexpected status %d: %s", code, msg)
	}
}

// isConflictMessage reports whether a 422 message describes an
// optimistic-concurrency or already-exists conflict.
func isConflictMessage(msg string) bool {
	m := strings.ToLower(msg)
	for _, s := range []string{"already exists", "wasn't supplied", "does not match", "but expected"} {
		if strings.Contains(m, s) {
			return true
		}
	}
	return false
}

// apiMessage extracts GitHub's {"message": "..."} field, falling back to a
// trimmed snippet of the body.
func apiMessage(body []byte) string {
	var e struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &e); err == nil && e.Message != "" {
		return e.Message
	}
	s := strings.TrimSpace(string(body))
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

// getPaged fetches every page of a JSON-array endpoint, following the RFC-5988
// Link header's rel="next" until exhausted, and concatenates the results.
func getPaged[T any](ctx context.Context, c *Client, urlStr string) ([]T, error) {
	var all []T
	for urlStr != "" {
		body, resp, err := c.do(ctx, urlStr, "application/vnd.github+json")
		if err != nil {
			return nil, err
		}
		var page []T
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("github: decode page: %w", err)
		}
		all = append(all, page...)
		urlStr = nextLink(resp.Header.Get("Link"))
	}
	return all, nil
}

// nextLink returns the URL of the rel="next" entry in a GitHub Link header, or
// "" when there is none.
func nextLink(header string) string {
	if header == "" {
		return ""
	}
	for _, part := range strings.Split(header, ",") {
		segs := strings.Split(strings.TrimSpace(part), ";")
		if len(segs) < 2 {
			continue
		}
		isNext := false
		for _, s := range segs[1:] {
			if strings.Contains(s, `rel="next"`) {
				isNext = true
				break
			}
		}
		if !isNext {
			continue
		}
		u := strings.TrimSpace(segs[0])
		u = strings.TrimPrefix(u, "<")
		u = strings.TrimSuffix(u, ">")
		return u
	}
	return ""
}

// esc escapes a single path segment (owner or repo name).
func esc(s string) string { return url.PathEscape(s) }

// escPath escapes each segment of a slash-separated path while preserving the
// separators, so "models/dish.dmn" stays a two-segment path.
func escPath(p string) string {
	segs := strings.Split(p, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}
