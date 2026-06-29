package vcs

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/pblumer/temis/dmn"
)

// dmnExt is the conventional file extension for DMN documents written by
// dmn-js and other tools.
const dmnExt = ".dmn"

// Models loads DMN documents from a version-controlled repository and compiles
// them with an engine. It is the library entry point that ties a Reader
// (how to reach the repository) to the dmn engine (how to compile what it
// finds), so a caller can list and evaluate the models on any branch, tag or
// commit.
//
// Models is safe for concurrent use when its Reader and Engine are (both are by
// contract). It holds no per-repository state; the ref is passed on every call.
type Models struct {
	reader Reader
	engine *dmn.Engine
}

// NewModels returns a Models backed by reader and engine. If engine is nil a
// default engine is used. reader must not be nil.
func NewModels(reader Reader, engine *dmn.Engine) *Models {
	if engine == nil {
		engine = dmn.New()
	}
	return &Models{reader: reader, engine: engine}
}

// List returns the DMN files (those whose name ends in ".dmn") directly under
// dir at ref, sorted by path. Directories and non-DMN files are omitted. An
// empty dir lists the repository root; an empty ref means the default branch.
func (m *Models) List(ctx context.Context, repo RepoRef, ref, dir string) ([]File, error) {
	entries, err := m.reader.ListFiles(ctx, repo, ref, dir)
	if err != nil {
		return nil, err
	}
	out := make([]File, 0, len(entries))
	for _, e := range entries {
		if e.IsDir || !strings.HasSuffix(strings.ToLower(e.Name), dmnExt) {
			continue
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// Load reads the DMN document at path and ref and compiles it. The returned
// Definitions and Diagnostics are exactly those of dmn.Engine.Compile; a
// malformed document (or a read failure) is a hard error, while per-decision
// compile problems come back through the diagnostics.
func (m *Models) Load(ctx context.Context, repo RepoRef, ref, path string) (*dmn.Definitions, dmn.Diagnostics, error) {
	xml, err := m.reader.ReadFile(ctx, repo, ref, path)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s@%s: %w", path, refLabel(ref), err)
	}
	return m.engine.Compile(ctx, xml)
}

// refLabel renders a ref for messages, naming the default branch when ref is
// empty.
func refLabel(ref string) string {
	if ref == "" {
		return "(default branch)"
	}
	return ref
}
