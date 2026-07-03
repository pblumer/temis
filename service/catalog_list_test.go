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
