package vcs

import "errors"

// ErrNotFound is returned (or wrapped) by a Reader when a repository, ref or
// path does not exist. Callers test for it with errors.Is.
var ErrNotFound = errors.New("vcs: not found")

// ErrUnauthorized is returned (or wrapped) by a Reader when the provider
// rejects the credentials (missing or invalid token, insufficient scope).
var ErrUnauthorized = errors.New("vcs: unauthorized")
