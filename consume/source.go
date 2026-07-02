package consume

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pblumer/temis/audit"
)

// DirSource is a Source backed by a directory of DMN files and flow descriptors.
// It indexes *.dmn/*.xml as models and *.flow.json as flows, each keyed by the
// content-addressed id (audit.ModelID: "sha256:"+hex), so an id matches the
// modelId/flowId recorded in events produced from the same bytes.
type DirSource struct {
	models map[string][]byte
	flows  map[string][]byte
}

// NewDirSource scans dir (non-recursively) and indexes models and flows by
// content-addressed id. It returns an error if the directory cannot be read.
func NewDirSource(dir string) (*DirSource, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read source dir: %w", err)
	}
	src := &DirSource{models: map[string][]byte{}, flows: map[string][]byte{}}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		switch {
		case strings.HasSuffix(name, ".flow.json"):
			src.flows[audit.ModelID(b)] = b
		case strings.HasSuffix(strings.ToLower(name), ".dmn"), strings.HasSuffix(strings.ToLower(name), ".xml"):
			src.models[audit.ModelID(b)] = b
		}
	}
	return src, nil
}

// Model implements Source.
func (s *DirSource) Model(modelID string) ([]byte, bool) {
	b, ok := s.models[modelID]
	return b, ok
}

// Flow implements Source.
func (s *DirSource) Flow(flowID string) ([]byte, bool) {
	b, ok := s.flows[flowID]
	return b, ok
}

// Models reports how many models were indexed.
func (s *DirSource) Models() int { return len(s.models) }

// Flows reports how many flow descriptors were indexed.
func (s *DirSource) Flows() int { return len(s.flows) }

// MapSource is an in-memory Source over fixed model and flow bytes keyed by id,
// handy for tests and callers that resolve artifacts another way.
type MapSource struct {
	ModelsByID map[string][]byte
	FlowsByID  map[string][]byte
}

// Model implements Source.
func (m MapSource) Model(id string) ([]byte, bool) { b, ok := m.ModelsByID[id]; return b, ok }

// Flow implements Source.
func (m MapSource) Flow(id string) ([]byte, bool) { b, ok := m.FlowsByID[id]; return b, ok }
