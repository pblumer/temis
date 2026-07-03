package service

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// TestListModelsCatalogFilters exercises the WP-141 catalog-aware GET /v1/models:
// each cached model is enriched with its catalog metadata, and the
// namespace/tag/status filters plus limit/offset paging narrow the listing so a
// server holding many decisions is navigable by asking rather than listing all.
func TestListModelsCatalogFilters(t *testing.T) {
	modelsDir := t.TempDir()
	writeModel := func(name, xml string) {
		if err := os.WriteFile(filepath.Join(modelsDir, name), []byte(xml), 0o600); err != nil {
			t.Fatalf("write model %s: %v", name, err)
		}
	}
	writeModel("discount.dmn", discountModelXML)
	writeModel("nologic.dmn", noLogicModelXML)
	discountID := modelID([]byte(discountModelXML))
	noLogicID := modelID([]byte(noLogicModelXML))

	catalogDir := t.TempDir()
	writeEntry := func(rel, body string) {
		p := filepath.Join(catalogDir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	writeEntry("domains/pricing/discount.catalog.json", `{"model":"`+discountID+`","owner":"@pricing","layer":"L1","tags":["pii"],"status":"active"}`)
	writeEntry("domains/risk/scoring.catalog.json", `{"model":"`+noLogicID+`","layer":"L1","status":"deprecated"}`)

	h := NewServer(nil, WithModelStore(modelsDir), WithCatalog(catalogDir)).Handler()

	list := func(query string) modelListResponse {
		rec := do(t, h, "GET", "/v1/models"+query, "", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /v1/models%s = %d: %s", query, rec.Code, rec.Body)
		}
		return decode[modelListResponse](t, rec)
	}

	// No filter: both models, enriched, catalogued grouped by coordinate first.
	all := list("")
	if all.Count != 2 || all.Total != 2 {
		t.Fatalf("unfiltered = count %d total %d, want 2/2", all.Count, all.Total)
	}
	if m := all.Models[0]; m.Namespace != "domains/pricing" || m.CatalogName != "discount" || m.Owner != "@pricing" || m.Status != "active" {
		t.Errorf("first summary not enriched/sorted: %+v", m)
	}
	if len(all.Models[0].Tags) != 1 || all.Models[0].Tags[0] != "pii" {
		t.Errorf("tags = %v, want [pii]", all.Models[0].Tags)
	}

	// Namespace prefix: exact child, parent, and a non-match that shares a prefix.
	if p := list("?namespace=domains/pricing"); p.Count != 1 || p.Models[0].ModelID != discountID {
		t.Errorf("namespace=domains/pricing → %d, want 1 (discount)", p.Count)
	}
	if p := list("?namespace=domains"); p.Count != 2 {
		t.Errorf("namespace=domains → %d, want 2", p.Count)
	}
	if p := list("?namespace=domains-x"); p.Count != 0 {
		t.Errorf("namespace=domains-x must not match domains/* → %d, want 0", p.Count)
	}

	// Status and tag filters (a catalog-scoped filter excludes uncatalogued models
	// too — here both models are catalogued).
	if p := list("?status=deprecated"); p.Count != 1 || p.Models[0].ModelID != noLogicID {
		t.Errorf("status=deprecated → %d, want 1 (scoring)", p.Count)
	}
	if p := list("?tag=pii"); p.Count != 1 || p.Models[0].ModelID != discountID {
		t.Errorf("tag=pii → %d, want 1 (discount)", p.Count)
	}

	// Pagination: Total is the full match count, Count the page size.
	if p := list("?limit=1"); p.Count != 1 || p.Total != 2 {
		t.Errorf("limit=1 → count %d total %d, want 1/2", p.Count, p.Total)
	}
	if p := list("?limit=1&offset=1"); p.Count != 1 || p.Models[0].ModelID != noLogicID {
		t.Errorf("limit=1&offset=1 → %+v, want the second (scoring)", p.Models)
	}
	if p := list("?offset=9"); p.Count != 0 || p.Total != 2 {
		t.Errorf("offset past end → count %d total %d, want 0/2", p.Count, p.Total)
	}
}

// TestCatalogEndpoint exercises GET /v1/catalog (WP-143): it answers from the
// catalog itself — so it lists a decision whose pinned revision is NOT loaded
// (resolved=false), the key difference from GET /v1/models — and honours the
// namespace/status filters.
func TestCatalogEndpoint(t *testing.T) {
	modelsDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(modelsDir, "discount.dmn"), []byte(discountModelXML), 0o600); err != nil {
		t.Fatalf("write model: %v", err)
	}
	loadedID := modelID([]byte(discountModelXML))
	unloadedID := "sha256:" + repeat64('b') // well-formed but never loaded

	catalogDir := t.TempDir()
	writeEntry := func(rel, body string) {
		p := filepath.Join(catalogDir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	writeEntry("domains/pricing/discount.catalog.json", `{"model":"`+loadedID+`","owner":"@pricing","layer":"L1","tags":["pii"],"status":"active"}`)
	writeEntry("domains/pricing/legacy.catalog.json", `{"model":"`+unloadedID+`","layer":"L1","status":"deprecated"}`)

	h := NewServer(nil, WithModelStore(modelsDir), WithCatalog(catalogDir)).Handler()
	get := func(query string) catalogListResponse {
		rec := do(t, h, "GET", "/v1/catalog"+query, "", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /v1/catalog%s = %d: %s", query, rec.Code, rec.Body)
		}
		return decode[catalogListResponse](t, rec)
	}

	// Both decisions are listed, sorted by coordinate; the unloaded one carries
	// resolved=false but is present — unlike GET /v1/models, which never shows it.
	all := get("")
	if all.Count != 2 || all.Total != 2 {
		t.Fatalf("catalog listed %d/%d, want 2/2", all.Count, all.Total)
	}
	if d := all.Decisions[0]; d.Coordinate != "domains/pricing/discount" || !d.Resolved || d.ModelID != loadedID || d.Owner != "@pricing" {
		t.Errorf("loaded entry = %+v", d)
	}
	if d := all.Decisions[1]; d.Coordinate != "domains/pricing/legacy" || d.Resolved || d.ModelID != unloadedID {
		t.Errorf("unloaded entry should be listed with resolved=false: %+v", d)
	}

	// The unloaded revision is absent from GET /v1/models (it is not cached), which
	// is exactly why list_catalog exists.
	models := decode[modelListResponse](t, do(t, h, "GET", "/v1/models", "", nil))
	for _, m := range models.Models {
		if m.ModelID == unloadedID {
			t.Errorf("unloaded revision must not appear in /v1/models: %+v", m)
		}
	}

	if p := get("?status=deprecated"); p.Count != 1 || p.Decisions[0].ModelID != unloadedID {
		t.Errorf("status=deprecated → %d, want 1 (legacy)", p.Count)
	}
	if p := get("?namespace=domains/risk"); p.Count != 0 {
		t.Errorf("namespace=domains/risk → %d, want 0", p.Count)
	}
}
