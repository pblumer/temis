package service

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCatalogLoadsFromDir exercises the WP-140 loader end to end: entries bind a
// namespace/name coordinate to a pinned model, the namespace defaults to the
// manifest's directory (git layout = namespace), a pinned model that is loaded
// resolves while an unloaded one still registers with a diagnostic, and malformed
// manifests are skipped without blocking startup.
func TestCatalogLoadsFromDir(t *testing.T) {
	// A real model in the cache, so a pinned revision can resolve. WithModelStore
	// loads *.dmn from a directory at startup, before the catalog, keying each by
	// its content hash — so the id the catalog must pin is modelID(xml).
	modelsDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(modelsDir, "m.dmn"), []byte(discountModelXML), 0o600); err != nil {
		t.Fatalf("write model: %v", err)
	}
	wantID := modelID([]byte(discountModelXML))

	catalogDir := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(catalogDir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	// Namespace + name derived from the path; pinned model is loaded → resolves.
	write("domains/pricing/base-price.catalog.json", `{
		"model": "`+wantID+`", "owner": "@pricing-team", "layer": "L1",
		"tags": ["pii", "pii", " jurisdiction=CH "], "status": "active"
	}`)
	// Explicit namespace/name override the path; pinned model is NOT loaded →
	// registers anyway, carrying a diagnostic, Resolved=false.
	write("foundation/vocab.catalog.json", `{
		"namespace": "foundation/vocabulary", "name": "currency",
		"model": "sha256:`+repeat64('a')+`"
	}`)
	// Malformed JSON, an unknown status, a missing model id, and a non-catalog
	// file — all skipped/ignored, none fatal.
	write("broken.catalog.json", `{ not json`)
	write("bad-status.catalog.json", `{"name":"x","model":"`+wantID+`","status":"retired"}`)
	write("no-model.catalog.json", `{"name":"y"}`)
	write("notes.txt", `ignore me`)

	s := NewServer(nil, WithModelStore(modelsDir), WithCatalog(catalogDir))

	entries := s.catalog.snapshot()
	if got := len(entries); got != 2 {
		t.Fatalf("catalog has %d entries, want 2: %+v", got, entries)
	}

	// snapshot is sorted by coordinate: "domains/pricing/base-price" < "foundation/vocabulary/currency".
	base := entries[0]
	if base.coord() != "domains/pricing/base-price" {
		t.Errorf("coord = %q, want domains/pricing/base-price", base.coord())
	}
	if base.Namespace != "domains/pricing" || base.Name != "base-price" {
		t.Errorf("namespace/name = %q/%q, want domains/pricing/base-price (derived from path)", base.Namespace, base.Name)
	}
	if base.Model != wantID || !base.Resolved {
		t.Errorf("base = %+v, want pinned+resolved model %s", base, wantID)
	}
	if base.Owner != "@pricing-team" || base.Layer != "L1" || base.Status != "active" {
		t.Errorf("base metadata = %+v", base)
	}
	// tags de-duplicated and trimmed, order preserved.
	if len(base.Tags) != 2 || base.Tags[0] != "pii" || base.Tags[1] != "jurisdiction=CH" {
		t.Errorf("tags = %v, want [pii jurisdiction=CH]", base.Tags)
	}

	vocab := entries[1]
	if vocab.Namespace != "foundation/vocabulary" || vocab.Name != "currency" {
		t.Errorf("explicit namespace/name not honoured: %+v", vocab)
	}
	if vocab.Resolved {
		t.Errorf("currency pins an unloaded model but reports Resolved=true")
	}
	if len(vocab.Diags) == 0 {
		t.Errorf("unresolved entry should carry a diagnostic, got none")
	}

	// A missing directory disables the catalog without blocking startup.
	s2 := NewServer(nil, WithCatalog(filepath.Join(catalogDir, "does-not-exist")))
	if got := s2.catalog.len(); got != 0 {
		t.Errorf("missing dir should yield empty catalog, got %d entries", got)
	}

	// No catalog configured is byte-identical to before: an empty, non-nil catalog.
	s3 := NewServer(nil)
	if s3.catalog == nil || s3.catalog.len() != 0 {
		t.Errorf("default server should have an empty catalog, got %+v", s3.catalog)
	}
}

// repeat64 builds a 64-char hex string of one rune, for a well-formed but
// deliberately-unloaded sha256 model id.
func repeat64(c byte) string {
	b := make([]byte, 64)
	for i := range b {
		b[i] = c
	}
	return string(b)
}
