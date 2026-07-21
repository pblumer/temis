package service

// Runtime-editable public-decision allowlist (ADR-0035, WP-107 follow-up). The
// per-model public set (option A) started life as immutable startup config
// (-public-models). This makes it toggleable at runtime through the admin API and
// the modeler's Zugriff UI, so an admin can open or close a single decision
// without a redeploy.
//
// Two sources under one lock, mirroring the keystore's static-vs-managed split:
//   - static entries from -public-models are always active and cannot be removed
//     at runtime (they are deployment config);
//   - managed entries are added/removed at runtime and persist to public.json in
//     the access-control dir (-keys-dir; ADR-0027 atomic write, pure stdlib) so a
//     toggle survives a restart. Without a dir they live only in memory.

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// errPublicStatic is returned when a caller tries to remove a static (startup)
// entry at runtime; the admin API maps it to 409.
var errPublicStatic = errors.New("entry is static (configured via -public-models); not removable at runtime")

// publicSet is the merged public-model allowlist: immutable static entries plus
// runtime-managed ones, guarded by one RWMutex. Reads (has) are on the evaluate
// hot path, so they take only the read lock.
type publicSet struct {
	mu      sync.RWMutex
	static  map[string]bool // -public-models; immutable at runtime, always active
	managed map[string]bool // runtime add/remove, persisted when persist != nil
	persist *publicPersist  // nil = in-memory only (no access-control dir)
}

// newPublicSet builds the set from the static seed (-public-models entries).
func newPublicSet(static []string) *publicSet {
	ps := &publicSet{static: map[string]bool{}, managed: map[string]bool{}}
	for _, m := range static {
		if m = strings.TrimSpace(m); m != "" {
			ps.static[m] = true
		}
	}
	return ps
}

// has reports whether entry (a modelId or display name) is public, static or
// managed. An empty entry is never public.
func (ps *publicSet) has(entry string) bool {
	if entry == "" {
		return false
	}
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.static[entry] || ps.managed[entry]
}

// empty reports whether no entries are configured at all (fast path guard).
func (ps *publicSet) empty() bool {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return len(ps.static) == 0 && len(ps.managed) == 0
}

// add marks entry public at runtime (managed) and persists. Adding a static or
// already-managed entry is an idempotent no-op. An empty entry is rejected.
func (ps *publicSet) add(entry string) error {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return errors.New("empty model")
	}
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if ps.static[entry] || ps.managed[entry] {
		return nil
	}
	ps.managed[entry] = true
	if err := ps.saveLocked(); err != nil {
		delete(ps.managed, entry) // roll back so memory matches disk
		return err
	}
	return nil
}

// remove drops a managed entry and persists. A static entry cannot be removed at
// runtime (errPublicStatic); an unknown managed entry is an idempotent no-op.
func (ps *publicSet) remove(entry string) error {
	entry = strings.TrimSpace(entry)
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if ps.static[entry] {
		return errPublicStatic
	}
	if !ps.managed[entry] {
		return nil
	}
	delete(ps.managed, entry)
	if err := ps.saveLocked(); err != nil {
		ps.managed[entry] = true // roll back
		return err
	}
	return nil
}

// staticList / managedList return the sorted entries of each source for the UI.
func (ps *publicSet) staticList() []string  { return sortedKeys(ps.mu.RLocker(), ps.static) }
func (ps *publicSet) managedList() []string { return sortedKeys(ps.mu.RLocker(), ps.managed) }

// persistent reports whether managed edits survive a restart (a store is attached).
func (ps *publicSet) persistent() bool {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.persist != nil
}

// saveLocked flushes managed entries to disk. No-op without a store. Caller holds
// the write lock.
func (ps *publicSet) saveLocked() error {
	if ps.persist == nil {
		return nil
	}
	out := make([]string, 0, len(ps.managed))
	for e := range ps.managed {
		out = append(out, e)
	}
	sort.Strings(out)
	return ps.persist.save(out)
}

// attach opens the persistent managed store at dir, loads its entries and wires
// future edits to flush there. Called at construction, before serving.
func (ps *publicSet) attach(dir string) (loaded int, err error) {
	p, err := newPublicPersist(dir)
	if err != nil {
		return 0, err
	}
	entries, err := p.load()
	if err != nil {
		return 0, err
	}
	ps.mu.Lock()
	defer ps.mu.Unlock()
	for _, e := range entries {
		if e = strings.TrimSpace(e); e != "" && !ps.static[e] {
			ps.managed[e] = true
			loaded++
		}
	}
	ps.persist = p
	return loaded, nil
}

// sortedKeys returns the keys of m sorted, taking lk while it reads.
func sortedKeys(lk sync.Locker, m map[string]bool) []string {
	lk.Lock()
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	lk.Unlock()
	sort.Strings(out)
	return out
}

// --- persistent managed store (ADR-0027) ---

// publicPersist is the optional filesystem home of the managed public entries: a
// single atomically-written JSON file next to the keystore in the access-control
// dir. Pure stdlib, no new dependency.
type publicPersist struct {
	path string
}

// publicFileName is the managed public set's file within the access-control dir.
const publicFileName = "public.json"

type publicDoc struct {
	Models []string `json:"models"`
}

// newPublicPersist opens (creating if needed) the store rooted at dir. The dir is
// 0700 because it is the access-control state directory (shared with the keystore).
func newPublicPersist(dir string) (*publicPersist, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &publicPersist{path: filepath.Join(dir, publicFileName)}, nil
}

// load reads the persisted managed entries. A missing file is an empty store.
func (p *publicPersist) load() ([]string, error) {
	data, err := os.ReadFile(p.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var doc publicDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return doc.Models, nil
}

// save writes the managed entries atomically (temp file + rename), so a crash
// mid-write never leaves a half-written file. The file is 0600.
func (p *publicPersist) save(models []string) error {
	data, err := json.MarshalIndent(publicDoc{Models: models}, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(p.path)
	tmp, err := os.CreateTemp(dir, ".tmp-public-*.json")
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
