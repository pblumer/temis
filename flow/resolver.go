package flow

import (
	"context"
	"fmt"

	"github.com/pblumer/temis/dmn"
)

// Resolver turns a content-addressed modelId into a compiled model. It is the
// seam through which a flow is backed by a model cache, a git source
// (package vcs) or an in-memory map. Implementations must be safe for concurrent
// use if the flow is evaluated concurrently.
type Resolver interface {
	Resolve(ctx context.Context, modelID string) (*dmn.Definitions, error)
}

// MapResolver is an in-memory Resolver over a fixed set of compiled models keyed
// by modelId. It suits inline composition and tests.
type MapResolver map[string]*dmn.Definitions

// Resolve returns the model registered under id, or an error when none is.
func (m MapResolver) Resolve(_ context.Context, id string) (*dmn.Definitions, error) {
	d, ok := m[id]
	if !ok {
		return nil, fmt.Errorf("no model %q", id)
	}
	return d, nil
}
