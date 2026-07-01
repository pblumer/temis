package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// readExample loads a bundled example so persistence tests exercise real models.
func readExample(t *testing.T, name string) []byte {
	t.Helper()
	xml, err := os.ReadFile(filepath.Join("examples", name))
	if err != nil {
		t.Fatalf("read example %s: %v", name, err)
	}
	return xml
}

func TestDiskStorePutIsIdempotentAndContentAddressed(t *testing.T) {
	dir := t.TempDir()
	st, err := newDiskStore(dir)
	if err != nil {
		t.Fatalf("newDiskStore: %v", err)
	}
	xml := readExample(t, "dish_15.dmn")
	id := modelID(xml)

	if err := st.put(id, xml); err != nil {
		t.Fatalf("put: %v", err)
	}
	// The file is named by the hex hash, not the scheme-prefixed id.
	if _, err := os.Stat(st.path(id)); err != nil {
		t.Fatalf("expected stored file: %v", err)
	}
	// Re-putting identical content is a no-op (no error, no duplicate).
	if err := st.put(id, xml); err != nil {
		t.Fatalf("re-put: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("want 1 stored file, got %d", len(entries))
	}

	got, ok := st.get(id)
	if !ok {
		t.Fatal("get: model not found")
	}
	if string(got) != string(xml) {
		t.Error("get returned different bytes than stored")
	}
}

func TestDiskStoreLoadReturnsAll(t *testing.T) {
	dir := t.TempDir()
	st, err := newDiskStore(dir)
	if err != nil {
		t.Fatalf("newDiskStore: %v", err)
	}
	a := readExample(t, "dish_15.dmn")
	b := readExample(t, "demo_greeting.dmn")
	if err := st.put(modelID(a), a); err != nil {
		t.Fatal(err)
	}
	if err := st.put(modelID(b), b); err != nil {
		t.Fatal(err)
	}

	loaded, err := st.load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("load returned %d models, want 2", len(loaded))
	}
}

// TestServerPersistsAcrossRestart is the core guarantee: a model uploaded to one
// server is present in a fresh server rooted at the same directory (ADR-0027).
func TestServerPersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	xml := readExample(t, "dish_15.dmn")

	first := NewServer(dmn.New(), WithModelStore(dir))
	sm, err := first.compileAndStore(context.Background(), xml)
	if err != nil {
		t.Fatalf("compileAndStore: %v", err)
	}
	id := sm.id

	// Simulate a restart: a brand-new server, same directory, no in-memory state.
	second := NewServer(dmn.New(), WithModelStore(dir))
	if got, ok := second.lookup(id); !ok {
		t.Fatal("model did not survive restart")
	} else if got.id != id {
		t.Errorf("restarted model id = %q, want %q", got.id, id)
	}
	if second.cache.len() != 1 {
		t.Errorf("restarted cache holds %d models, want 1", second.cache.len())
	}
}

// TestExamplesAreNotPersisted guards that the bundled examples never leak onto
// disk — they re-embed on every start, so persisting them would just pollute the
// store directory.
func TestExamplesAreNotPersisted(t *testing.T) {
	dir := t.TempDir()
	srv := NewServer(dmn.New(), WithExamples(), WithModelStore(dir))
	if srv.cache.len() == 0 {
		t.Fatal("examples should have loaded into the cache")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("store directory should be empty, holds %d files", len(entries))
	}
}

// TestLookupFallsBackToStoreAfterEviction verifies that a persisted model evicted
// from the bounded cache is still retrievable via the on-disk fallback.
func TestLookupFallsBackToStoreAfterEviction(t *testing.T) {
	dir := t.TempDir()
	a := readExample(t, "dish_15.dmn")
	b := readExample(t, "demo_greeting.dmn")

	// Cache capacity 1: storing b evicts a from memory, but it stays on disk.
	srv := NewServer(dmn.New(), WithCacheSize(1), WithModelStore(dir))
	smA, err := srv.compileAndStore(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srv.compileAndStore(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	if _, ok := srv.cache.get(smA.id); ok {
		t.Fatal("precondition: a should have been evicted from the cache")
	}

	// lookup recompiles a from the store and re-caches it.
	if got, ok := srv.lookup(smA.id); !ok {
		t.Fatal("evicted-but-persisted model not found via store fallback")
	} else if got.id != smA.id {
		t.Errorf("fallback returned id %q, want %q", got.id, smA.id)
	}
}

func TestWithoutStoreNothingIsPersisted(t *testing.T) {
	srv := NewServer(dmn.New())
	if srv.store != nil {
		t.Fatal("store should be nil without WithModelStore")
	}
	xml := readExample(t, "dish_15.dmn")
	sm, err := srv.compileAndStore(context.Background(), xml)
	if err != nil {
		t.Fatal(err)
	}
	// lookup still works purely from the in-memory cache.
	if _, ok := srv.lookup(sm.id); !ok {
		t.Error("in-memory lookup failed")
	}
}
