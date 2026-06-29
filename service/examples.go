package service

import (
	"context"
	"embed"
	"path"
	"sort"
	"strings"
)

// exampleFS holds the bundled example DMN models, embedded so a freshly started
// temisd serves them without any external files. They populate the cache when
// the server is built with WithExamples, so the modeler's model picker is never
// empty on a new start.
//
//go:embed examples/*.dmn
var exampleFS embed.FS

// WithExamples preloads the bundled example DMN models into the cache at
// construction, so they appear in GET /v1/models (and the modeler) on a
// fresh start. Models that fail to compile are skipped silently — examples must
// never prevent the server from starting.
func WithExamples() Option {
	return func(s *Server) { s.loadExamplesOnInit = true }
}

// loadExamples compiles and caches every embedded example. Each example's
// display name is its file stem (the bundled files declare no DMN name), so the
// explorer lists them as e.g. "dish.dmn" rather than a content hash.
func (s *Server) loadExamples(ctx context.Context) {
	entries, err := exampleFS.ReadDir("examples")
	if err != nil {
		return
	}
	// Deterministic order so the explorer list is stable across starts.
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".dmn") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, file := range names {
		xml, err := exampleFS.ReadFile(path.Join("examples", file))
		if err != nil {
			continue
		}
		sm, err := s.compileAndStore(ctx, xml)
		if err != nil {
			continue
		}
		if sm.name == "" {
			// Set at construction time, before the server serves, so no lock is
			// needed; the cache holds this same pointer.
			sm.name = strings.TrimSuffix(file, ".dmn")
		}
	}
}
