package service

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/pblumer/temis/flow"
)

// Draft garbage collection (ADR-0037, WP-143). Every save is a content-addressed
// draft that the store keeps append-only (ADR-0027), so revisions accumulate. GC
// is the deliberate, admin-scoped cleanup that prunes only *unreferenced* drafts:
// a revision is KEPT when it is a release, is referenced by a registered flow, or
// is the newest cached revision of its named model (the working head). Everything
// else — older, unpublished, unreferenced drafts — is removed from both the cache
// and the on-disk store. It is opt-in (a POST, never automatic), so the default
// stays the safe append-only behaviour.

type gcResponse struct {
	// Deleted lists the model ids that were pruned.
	Deleted []string `json:"deleted"`
	// DeletedCount and Remaining summarise the pass.
	DeletedCount int `json:"deletedCount"`
	Remaining    int `json:"remaining"`
}

// handleGCModels prunes unreferenced model drafts (ADR-0037). It responds 200 with
// the pruned ids and the remaining count. Releases, flow-referenced revisions and
// each named model's newest cached revision are always kept.
func (s *Server) handleGCModels(w http.ResponseWriter, _ *http.Request) {
	keep := s.gcKeepSet()

	// Candidates: every cached id plus every durably stored id.
	candidates := map[string]struct{}{}
	for _, sm := range s.cache.snapshot() {
		candidates[sm.id] = struct{}{}
	}
	if s.store != nil {
		for _, id := range s.store.listIDs() {
			candidates[id] = struct{}{}
		}
	}

	deleted := make([]string, 0)
	for id := range candidates {
		if keep[id] {
			continue
		}
		s.cache.delete(id)
		if s.store != nil {
			if _, err := s.store.delete(id); err != nil {
				writeProblem(w, http.StatusInternalServerError, "GC_FAILED", err.Error())
				return
			}
		}
		deleted = append(deleted, id)
	}
	sort.Strings(deleted)
	writeJSON(w, http.StatusOK, gcResponse{
		Deleted:      deleted,
		DeletedCount: len(deleted),
		Remaining:    len(candidates) - len(deleted),
	})
}

// gcKeepSet computes the set of model ids a GC pass must not prune: every released
// revision, every revision a registered flow references (frozen refs are raw ids;
// any unresolved release ref is resolved through the catalog), and the newest
// cached revision of each named model (its working head).
func (s *Server) gcKeepSet() map[string]bool {
	keep := map[string]bool{}
	if s.releases != nil {
		for id := range s.releases.releasedIDs() {
			keep[id] = true
		}
	}
	for _, sf := range s.flows.snapshot() {
		var d flow.Descriptor
		if json.Unmarshal(sf.desc, &d) != nil {
			continue
		}
		for _, st := range d.Steps {
			m := strings.TrimSpace(st.Model)
			if validModelID(m) {
				keep[m] = true
			} else if s.releases != nil {
				if mid, ok := s.releases.resolve(m); ok {
					keep[mid] = true
				}
			}
		}
	}
	// The newest cached revision of each named model is the working head — never
	// prune the user's current draft. Unnamed models share the "" bucket, so only
	// the newest unnamed draft survives (older ones are exactly the flood to prune).
	headSeq := map[string]uint64{}
	headID := map[string]string{}
	for _, sm := range s.cache.snapshot() {
		if sm.id == headID[sm.name] {
			continue
		}
		if sm.seq >= headSeq[sm.name] || headID[sm.name] == "" {
			headSeq[sm.name] = sm.seq
			headID[sm.name] = sm.id
		}
	}
	for _, id := range headID {
		keep[id] = true
	}
	return keep
}
