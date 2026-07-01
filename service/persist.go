package service

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// diskStore is an optional filesystem-backed store of raw DMN models, so the
// server's working set survives a restart (ADR-0027). Files are content-addressed
// — the file name is the model's SHA-256 hex with a ".dmn" suffix — so
// re-persisting an unchanged model is a no-op and an edited model lands under a
// new name (never overwriting the old revision). Only the raw XML is stored:
// everything else (compiled definitions, index, diagnostics, display name) is
// re-derived deterministically on load, so the store can never drift from what
// the engine would produce today.
type diskStore struct {
	dir string
}

// newDiskStore opens the model store rooted at dir, creating the directory (and
// any parents) if needed.
func newDiskStore(dir string) (*diskStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("model store: %w", err)
	}
	return &diskStore{dir: dir}, nil
}

// path maps a model id ("sha256:<hex>") to its on-disk file path. The scheme
// prefix and its colon are dropped so the name is portable across filesystems.
func (d *diskStore) path(id string) string {
	hex := strings.TrimPrefix(id, "sha256:")
	return filepath.Join(d.dir, hex+".dmn")
}

// put writes xml under id if it is not already stored. Because files are
// content-addressed, an existing file holds identical bytes, so put leaves it —
// and its modification time, which orders the listing — untouched. The write is
// atomic: a temp file in the same directory is renamed into place, so a crash
// mid-write never leaves a half-written model that would fail to compile on the
// next start.
func (d *diskStore) put(id string, xml []byte) error {
	p := d.path(id)
	if _, err := os.Stat(p); err == nil {
		return nil // already stored (identical content)
	}
	tmp, err := os.CreateTemp(d.dir, ".tmp-*.dmn")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(xml); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, p); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

// get returns the raw XML stored under id, if present. It backs the cache-miss
// fallback so a model evicted from the bounded in-memory cache is still durably
// retrievable.
func (d *diskStore) get(id string) ([]byte, bool) {
	xml, err := os.ReadFile(d.path(id))
	if err != nil {
		return nil, false
	}
	return xml, true
}

// load returns every stored model's raw XML, oldest first (by modification time,
// then file name as a stable tiebreaker), so the caller can repopulate the cache
// in a stable, roughly creation-ordered sequence. Unreadable entries are skipped
// rather than failing the whole load.
func (d *diskStore) load() ([][]byte, error) {
	entries, err := os.ReadDir(d.dir)
	if err != nil {
		return nil, err
	}
	type item struct {
		name string
		mod  int64
	}
	items := make([]item, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".dmn") || strings.HasPrefix(e.Name(), ".tmp-") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		items = append(items, item{name: e.Name(), mod: info.ModTime().UnixNano()})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].mod != items[j].mod {
			return items[i].mod < items[j].mod
		}
		return items[i].name < items[j].name
	})
	out := make([][]byte, 0, len(items))
	for _, it := range items {
		xml, err := os.ReadFile(filepath.Join(d.dir, it.name))
		if err != nil {
			continue
		}
		out = append(out, xml)
	}
	return out, nil
}
