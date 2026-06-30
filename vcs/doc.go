// Package vcs reads DMN models from a version-controlled repository (git) and
// compiles them with the engine, so models can be browsed and evaluated per
// branch, tag or commit instead of being uploaded ad hoc.
//
// It accesses the engine only through the public dmn package, never through
// internal/ (architecture D5/ADR-0005), and is library-first: the HTTP service,
// the MCP server and the UI are thin wrappers over this package.
//
// # Providers
//
// The concrete way of talking to a git host is abstracted behind the Reader
// interface (and, later, a Writer for commits/branches). The first provider is
// GitHub over its REST API (subpackage vcs/github), implemented with the
// standard library alone — no new dependency. Because the contract is an
// interface, further backends (a pure-Go git library, the git CLI, GitLab,
// Bitbucket) can be added without touching callers. See ADR-0022.
//
// # Refs
//
// A ref names a point in history: a branch name, a tag, or a commit SHA. Every
// read is taken at an explicit ref, so evaluating "the dish model on branch
// release-2" is a first-class operation and reproducible.
package vcs
