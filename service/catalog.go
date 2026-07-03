package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// The decision catalog is the runtime identity/organisation plane over the flat,
// content-addressed model store (ADR-0034, WP-140). A hash is anonymous — no
// name, no place — so ordering lives on the *names*, not the blobs: an entry
// binds a human coordinate `namespace/name` to a pinned revision (`sha256:<hex>`)
// plus governance metadata (owner, layer, tags, status). The catalog is derived
// from git and loaded read-only, mirroring -flows-dir (ADR-0032): the directory
// is the source of truth, the server never writes it back, so there is no second
// source of truth and no drift.
//
// This work package is the format and the loader. Surfacing the catalog through
// list_models (prefix/tag/status filters, pagination) is WP-141; the modeler
// namespace tree and bookmarks are WP-142.

// catalogStatuses is the closed set of lifecycle states an entry may carry. An
// entry with any other status is malformed and skipped at load. Empty defaults
// to "active".
var catalogStatuses = map[string]bool{"active": true, "deprecated": true, "archived": true}

// catalogLayers is the closed set of layers from the federated-governance model
// (docs/90 §2, ADR-0025). Empty is allowed (unclassified); any other value is
// malformed and skipped.
var catalogLayers = map[string]bool{"L0": true, "L1": true, "L2a": true, "L2b": true, "L3": true}

// catalogFile is the on-disk shape of a *.catalog.json manifest — one decision's
// catalog coordinate and metadata, authored next to the model in git. namespace
// and name are optional: when omitted they default to the file's directory path
// (relative to the catalog root) and its filename stem, so the git directory
// layout *is* the namespace with no repetition (ADR-0034).
type catalogFile struct {
	Namespace string   `json:"namespace,omitempty"`
	Name      string   `json:"name,omitempty"`
	Model     string   `json:"model"` // pinned revision, "sha256:<hex>" (required)
	Owner     string   `json:"owner,omitempty"`
	Layer     string   `json:"layer,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Status    string   `json:"status,omitempty"`
}

// catalogEntry is a loaded, validated catalog record. resolved reports whether
// Model is currently loaded in the cache; when false the entry still registers
// (fail-open, like a flow referencing an unloaded model) and carries a diagnostic
// so the gap is visible rather than silent.
type catalogEntry struct {
	Namespace string
	Name      string
	Model     string
	Owner     string
	Layer     string
	Tags      []string
	Status    string
	Resolved  bool
	Diags     []string
}

// coord is the entry's stable catalog coordinate, "namespace/name" (or just
// "name" at the root), used as the map key and for a stable listing order.
func (e catalogEntry) coord() string {
	if e.Namespace == "" {
		return e.Name
	}
	return e.Namespace + "/" + e.Name
}

// catalogStore holds loaded catalog entries keyed by their coordinate. Entries
// are few relative to evaluations and loaded once at startup, so a plain guarded
// map suffices (no LRU, mirroring flowStore).
type catalogStore struct {
	mu sync.Mutex
	m  map[string]*catalogEntry
}

func newCatalogStore() *catalogStore { return &catalogStore{m: map[string]*catalogEntry{}} }

// put registers e under its coordinate, reporting whether it was newly added
// (false means a duplicate coordinate that the caller should skip and log).
func (c *catalogStore) put(e *catalogEntry) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.m[e.coord()]; exists {
		return false
	}
	c.m[e.coord()] = e
	return true
}

// snapshot returns every registered entry, sorted by coordinate for a stable
// listing order. It backs the catalog-aware model listing (WP-141).
func (c *catalogStore) snapshot() []*catalogEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*catalogEntry, 0, len(c.m))
	for _, e := range c.m {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].coord() < out[j].coord() })
	return out
}

// len reports the number of registered entries.
func (c *catalogStore) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.m)
}

// byModel returns the catalog entry that pins model id, so a model listing can be
// enriched with its namespace/owner/layer/tags/status (WP-141). When more than one
// entry pins the same revision (unusual) the lexicographically smallest coordinate
// wins, so the join is deterministic.
func (c *catalogStore) byModel(id string) (*catalogEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var best *catalogEntry
	for _, e := range c.m {
		if e.Model == id && (best == nil || e.coord() < best.coord()) {
			best = e
		}
	}
	return best, best != nil
}

// namespaceMatches reports whether ns lies at or under the prefix namespace: an
// exact match or a proper descendant (so "domains" matches "domains/pricing" but
// not "domains-x"). An empty prefix matches everything. Leading/trailing slashes
// on the prefix are ignored, mirroring how a namespace is stored.
func namespaceMatches(ns, prefix string) bool {
	prefix = strings.Trim(strings.TrimSpace(prefix), "/")
	if prefix == "" {
		return true
	}
	return ns == prefix || strings.HasPrefix(ns, prefix+"/")
}

// hasAllTags reports whether tags contains every tag in want (AND semantics), so
// a listing can require several labels at once. An empty want matches everything.
func hasAllTags(tags, want []string) bool {
	for _, w := range want {
		found := false
		for _, t := range tags {
			if t == w {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// loadCatalog walks s.catalogDir for *.catalog.json manifests and registers each
// in the catalog (ADR-0034). It runs at construction, after the models it
// references are loaded, so validation against the cache is meaningful; it needs
// no locking beyond the store's own.
//
// Read-only: the directory is the source of truth (git + git_propose), never
// written back — so the catalog can never drift from git (contrast the model
// store, which persists uploads). Fail-open, mirroring loadFlows: a manifest that
// is unreadable, malformed, or names no/invalid model is logged and skipped,
// never blocking startup and left on disk so a later fix recovers it. An entry
// whose pinned model is not (yet) loaded still registers, carrying a diagnostic.
// A missing directory disables the catalog (logged) without blocking startup.
func (s *Server) loadCatalog(ctx context.Context) {
	if _, err := os.Stat(s.catalogDir); err != nil {
		log.Printf("temis: catalog disabled: %v", err)
		return
	}
	var loaded, skipped int
	err := filepath.WalkDir(s.catalogDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Printf("temis: catalog %q: %v", path, err)
			skipped++
			return nil //nolint:nilerr // skip the unreadable entry, keep walking
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".catalog.json") {
			return nil
		}
		e, ok := s.parseCatalogEntry(path)
		if !ok {
			skipped++
			return nil
		}
		if !s.catalog.put(e) {
			log.Printf("temis: catalog %q: duplicate coordinate %q, skipped", path, e.coord())
			skipped++
			return nil
		}
		loaded++
		return nil
	})
	if err != nil {
		log.Printf("temis: catalog: %v", err)
	}
	msg := fmt.Sprintf("temis: catalog at %s (%d entries loaded", s.catalogDir, loaded)
	if skipped > 0 {
		msg += fmt.Sprintf(", %d skipped", skipped)
	}
	log.Print(msg + ")")
}

// parseCatalogEntry reads and validates one *.catalog.json manifest at path,
// deriving the namespace from the file's directory (relative to the catalog root)
// and the name from its filename stem when the manifest omits them. It returns
// ok=false for a structurally invalid manifest (unreadable, malformed JSON, a
// missing/ill-formed model id, or an unknown status/layer) — those are logged and
// skipped. A well-formed entry whose pinned model is simply not loaded is valid
// (ok=true) but carries a diagnostic and Resolved=false.
func (s *Server) parseCatalogEntry(path string) (*catalogEntry, bool) {
	body, err := os.ReadFile(path)
	if err != nil {
		log.Printf("temis: catalog %q: %v", path, err)
		return nil, false
	}
	var f catalogFile
	if err := json.Unmarshal(body, &f); err != nil {
		log.Printf("temis: catalog %q: %v", path, err)
		return nil, false
	}

	namespace := strings.Trim(strings.TrimSpace(f.Namespace), "/")
	if namespace == "" {
		// Derive from the directory path relative to the catalog root, so the git
		// layout is the namespace (ADR-0034). filepath.Rel + ToSlash keeps it
		// portable; the root itself ("." ) means an unnamespaced, root-level entry.
		if rel, err := filepath.Rel(s.catalogDir, filepath.Dir(path)); err == nil && rel != "." {
			namespace = filepath.ToSlash(rel)
		}
	}
	name := strings.TrimSpace(f.Name)
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(path), ".catalog.json")
	}
	if name == "" {
		log.Printf("temis: catalog %q: entry has no name", path)
		return nil, false
	}

	model := strings.TrimSpace(f.Model)
	if !validModelID(model) {
		log.Printf("temis: catalog %q: missing or malformed model id %q", path, f.Model)
		return nil, false
	}

	status := strings.TrimSpace(f.Status)
	if status == "" {
		status = "active"
	}
	if !catalogStatuses[status] {
		log.Printf("temis: catalog %q: unknown status %q", path, f.Status)
		return nil, false
	}
	if f.Layer != "" && !catalogLayers[f.Layer] {
		log.Printf("temis: catalog %q: unknown layer %q", path, f.Layer)
		return nil, false
	}

	e := &catalogEntry{
		Namespace: namespace,
		Name:      name,
		Model:     model,
		Owner:     strings.TrimSpace(f.Owner),
		Layer:     f.Layer,
		Tags:      normalizeTags(f.Tags),
		Status:    status,
	}
	// Validate the pinned revision against the loaded models. A missing model is
	// not fatal — it may be deployed later — but it is surfaced as a diagnostic so
	// the gap is visible (mirrors a flow that references an unloaded model).
	if _, ok := s.lookup(model); ok {
		e.Resolved = true
	} else {
		e.Diags = append(e.Diags, "pinned model not loaded")
	}
	return e, true
}

// normalizeTags trims, drops empties and de-duplicates tags while preserving
// first-seen order, so a listing filter (WP-141) sees clean, stable labels.
func normalizeTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(tags))
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}
