package vcs

import "errors"

// ErrNotFound is returned (or wrapped) by a Reader when a repository, ref or
// path does not exist. Callers test for it with errors.Is.
var ErrNotFound = errors.New("vcs: not found")

// ErrUnauthorized is returned (or wrapped) by a Reader when the provider
// rejects the credentials (missing or invalid token, insufficient scope).
var ErrUnauthorized = errors.New("vcs: unauthorized")

// ErrConflict is returned (or wrapped) by a Writer when a write loses an
// optimistic-concurrency check (the file or branch moved since the base SHA the
// change was built on) or when creating something that already exists. Callers
// test for it with errors.Is and typically re-read, re-apply and retry.
var ErrConflict = errors.New("vcs: conflict")
