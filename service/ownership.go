package service

// Owner visibility for models and flows (WP-106). This is the adapter layer's own
// concern; the engine core (package dmn) never imports it (ADR-0011).
//
// Models and flows are content-addressed (a resource id is the SHA-256 of its
// bytes), so ownership cannot live *inside* the artefact — that would change its
// hash and thus its id. Instead ownership is a side index: resource id → the set
// of key ids (kids) that created it. A caller may see a resource when they are
// among its owners, or when it has no owner at all (seed/example models and
// git-declared flows stay shared). An admin key bypasses the filter entirely
// (identityOf sets seeAll).
//
// The index only ever gains owners as authenticated keys create/edit artefacts;
// on the open API (no keys configured) nothing is ever claimed, so every resource
// stays unowned and the historical "everything is visible" default holds
// byte-for-byte. Pure stdlib.
//
// Grouping several keys/people into a shared view (a "team") is deliberately NOT
// modelled here — that is being designed separately.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// ownership records, per resource id, which keys own it. It backs both the access
// gate (may this caller reach id?) and the list filter (mine vs. shared). It is
// safe for concurrent use.
type ownership struct {
	mu sync.RWMutex
	// owners maps a resource id to the set of owning kids. A resource absent from
	// the map (or with an empty set) is unowned and visible to everyone.
	owners  map[string]map[string]bool
	persist *ownerPersist // nil = in-memory only (no store dir)
}

// newOwnership returns an in-memory ownership index. When persist is non-nil the
// index is seeded from disk and future claims are flushed there.
func newOwnership(persist *ownerPersist) (*ownership, error) {
	o := &ownership{owners: map[string]map[string]bool{}, persist: persist}
	if persist != nil {
		loaded, err := persist.load()
		if err != nil {
			return nil, err
		}
		o.owners = loaded
	}
	return o, nil
}

// claim records that the key identified by ident created (or edited into
// existence) the resource id, so the owner can later see it. It is a no-op when
// ident is unset or carries no kid — an unauthenticated write on the open API
// claims nothing, leaving the resource unowned (visible to all). A persistence
// failure is returned so the caller can log it; the in-memory claim still stands
// (visibility must not silently widen on a disk hiccup).
func (o *ownership) claim(id string, ident authIdentity) error {
	if id == "" || ident.kid == "" {
		return nil
	}
	o.mu.Lock()
	set, ok := o.owners[id]
	if !ok {
		set = map[string]bool{}
		o.owners[id] = set
	}
	if set[ident.kid] {
		o.mu.Unlock()
		return nil // already recorded — nothing to flush
	}
	set[ident.kid] = true
	snapshot := o.cloneLocked()
	o.mu.Unlock()
	if o.persist != nil {
		return o.persist.save(snapshot)
	}
	return nil
}

// visible reports whether the caller identified by ident may see resource id. An
// admin (seeAll) sees everything; an unowned resource is visible to all; otherwise
// the caller must be an owner (by kid).
func (o *ownership) visible(id string, ident authIdentity) bool {
	if ident.seeAll {
		return true
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	set := o.owners[id]
	if len(set) == 0 {
		return true // unowned → shared (examples, git-declared flows, pre-auth models)
	}
	return set[ident.kid]
}

// ownedByKid reports whether ident's own key (by kid) is an owner of id. It backs
// the "?owner=me" list filter (only artefacts I created), which is narrower than
// visible (it excludes shared/unowned artefacts).
func (o *ownership) ownedByKid(id string, ident authIdentity) bool {
	if ident.kid == "" {
		return false
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.owners[id][ident.kid]
}

// cloneLocked returns a deep copy of the owner map for persistence. Caller holds
// at least the read lock.
func (o *ownership) cloneLocked() map[string]map[string]bool {
	out := make(map[string]map[string]bool, len(o.owners))
	for id, set := range o.owners {
		cp := make(map[string]bool, len(set))
		for kid := range set {
			cp[kid] = true
		}
		out[id] = cp
	}
	return out
}

// --- persistence (WP-106) ---

// ownerPersist is the optional filesystem home of the ownership index: a single
// atomically-written JSON file alongside the model store, so owner isolation
// survives a restart (without it, persisted models would reload unowned and thus
// become visible to everyone — a silent isolation regression). Pure stdlib.
type ownerPersist struct {
	path string
}

// ownersFileName is the ownership index's file within the model store dir.
const ownersFileName = "owners.json"

// ownerDoc is the on-disk schema: resource id → list of owning kids.
type ownerDoc struct {
	Owners map[string][]string `json:"owners"`
}

// newOwnerPersist roots the ownership index in dir (the model store directory,
// already created by the disk store). It does not touch the filesystem until the
// first load/save.
func newOwnerPersist(dir string) *ownerPersist {
	return &ownerPersist{path: filepath.Join(dir, ownersFileName)}
}

// load reads the persisted ownership index. A missing file is an empty index
// (first run), not an error.
func (p *ownerPersist) load() (map[string]map[string]bool, error) {
	data, err := os.ReadFile(p.path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]map[string]bool{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read ownership index: %w", err)
	}
	var doc ownerDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("ownership index %s: %w", p.path, err)
	}
	out := make(map[string]map[string]bool, len(doc.Owners))
	for id, kids := range doc.Owners {
		set := make(map[string]bool, len(kids))
		for _, kid := range kids {
			if kid != "" {
				set[kid] = true
			}
		}
		if len(set) > 0 {
			out[id] = set
		}
	}
	return out, nil
}

// save writes the ownership index to disk atomically (temp file + rename in the
// same directory), so a crash mid-write never leaves a half-written index. The
// file is 0600 — it carries key ids, not secrets, but keeping it tight is cheap.
func (p *ownerPersist) save(owners map[string]map[string]bool) error {
	doc := ownerDoc{Owners: make(map[string][]string, len(owners))}
	for id, set := range owners {
		kids := make([]string, 0, len(set))
		for kid := range set {
			kids = append(kids, kid)
		}
		sort.Strings(kids)
		doc.Owners[id] = kids
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(p.path)
	tmp, err := os.CreateTemp(dir, ".tmp-owners-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, p.path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}
