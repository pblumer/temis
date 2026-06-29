package vcs

import "context"

// RepoRef identifies a repository on a provider. For GitHub it is the
// owner/name pair (e.g. {Owner: "pblumer", Name: "temis"}); other providers map
// it to their own addressing.
type RepoRef struct {
	// Owner is the account or organisation that owns the repository.
	Owner string
	// Name is the repository name within the owner.
	Name string
}

// Branch is a named line of development together with the commit it currently
// points at.
type Branch struct {
	// Name is the branch name (e.g. "main").
	Name string
	// Commit is the SHA of the commit the branch points to.
	Commit string
}

// Commit is a single point in a repository's history.
type Commit struct {
	// SHA is the commit's content hash.
	SHA string
	// Message is the commit message (its full text).
	Message string
	// Author is the human-readable author name.
	Author string
	// Date is the author date in RFC-3339 form, or empty if the provider did
	// not supply one. It is kept as a string so this package stays free of time
	// semantics.
	Date string
}

// File is an entry in a repository tree at a given ref: either a regular file
// or a directory.
type File struct {
	// Path is the slash-separated path from the repository root.
	Path string
	// Name is the final path segment.
	Name string
	// Size is the file size in bytes; zero for directories.
	Size int64
	// SHA is the provider's blob/tree hash for the entry.
	SHA string
	// IsDir reports whether the entry is a directory.
	IsDir bool
}

// Reader is the read-only surface a git provider must implement to browse a
// repository and fetch file contents at an explicit ref. A ref is a branch
// name, tag or commit SHA; an empty ref means the repository's default branch.
//
// Implementations must be safe for concurrent use and must not mutate the
// repository. Writing (commits, branches, merges) is a separate, later
// interface (WP-61); keeping reads isolated lets read-only credentials stay
// read-only.
type Reader interface {
	// ListBranches returns every branch of the repository.
	ListBranches(ctx context.Context, repo RepoRef) ([]Branch, error)

	// ListCommits returns the commit history reachable from ref, newest first.
	// An empty ref means the default branch.
	ListCommits(ctx context.Context, repo RepoRef, ref string) ([]Commit, error)

	// ListFiles returns the entries directly under dir at ref (non-recursive).
	// An empty dir lists the repository root; an empty ref means the default
	// branch.
	ListFiles(ctx context.Context, repo RepoRef, ref, dir string) ([]File, error)

	// ReadFile returns the raw bytes of the file at path and ref. An empty ref
	// means the default branch. It returns an error matching ErrNotFound when
	// no such file exists.
	ReadFile(ctx context.Context, repo RepoRef, ref, path string) ([]byte, error)
}
