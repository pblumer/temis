package vcs

import "context"

// CommitRequest describes a single-file commit on an existing branch.
type CommitRequest struct {
	// Branch is the branch to commit on. It must already exist (create it with
	// Writer.CreateBranch).
	Branch string
	// Path is the slash-separated file path from the repository root.
	Path string
	// Content is the new file content.
	Content []byte
	// Message is the commit message.
	Message string
	// PrevSHA is the blob SHA the change is based on, for optimistic
	// concurrency: the write fails with ErrConflict if the file has since moved
	// off this SHA. An empty PrevSHA means "create a new file" — the write
	// fails with ErrConflict if a file already exists at Path.
	PrevSHA string
}

// CommitResult is the outcome of a successful Commit.
type CommitResult struct {
	// CommitSHA is the SHA of the new commit.
	CommitSHA string
	// BlobSHA is the SHA of the written file's new blob; pass it as the next
	// CommitRequest.PrevSHA to chain edits with optimistic concurrency.
	BlobSHA string
}

// PullRequestRequest describes a pull request to open.
type PullRequestRequest struct {
	// Title is the pull request title.
	Title string
	// Body is the pull request description.
	Body string
	// Head is the branch containing the changes.
	Head string
	// Base is the branch the changes should be merged into.
	Base string
}

// PullRequest identifies an opened pull request.
type PullRequest struct {
	// Number is the provider's pull request number.
	Number int
	// URL is the human-facing URL of the pull request.
	URL string
	// State is the pull request state (e.g. "open").
	State string
}

// Writer is the write surface a git provider implements to branch, commit and
// open pull requests. It is the counterpart to Reader; a provider may implement
// both. Implementations must be safe for concurrent use.
//
// Merging is deliberately not part of this interface: three-way and XML/diagram
// merges are left to the provider's pull-request machinery (e.g. a GitHub PR
// merge), not reimplemented in Temis (ADR-0022).
type Writer interface {
	// CreateBranch creates a new branch named name pointing at the commit that
	// fromRef (a branch, tag or commit SHA) resolves to. It returns ErrConflict
	// if a branch named name already exists.
	CreateBranch(ctx context.Context, repo RepoRef, name, fromRef string) (Branch, error)

	// Commit writes a single file on an existing branch and returns the new
	// commit and blob SHAs. See CommitRequest for the optimistic-concurrency
	// contract.
	Commit(ctx context.Context, repo RepoRef, req CommitRequest) (CommitResult, error)

	// OpenPullRequest opens a pull request from req.Head into req.Base.
	OpenPullRequest(ctx context.Context, repo RepoRef, req PullRequestRequest) (PullRequest, error)
}
