package mcp

import "testing"

// fakeCatalog is a minimal Catalog for the list_catalog tool test.
type fakeCatalog struct{ entries []CatalogEntry }

func (f *fakeCatalog) List() []CatalogEntry { return f.entries }

// TestListCatalogTool proves list_catalog answers from the catalog itself
// (ADR-0034, WP-143): it lists decisions whose revision is not loaded, carries
// coordinate/metadata, and honours the namespace/tag/status filters and paging.
func TestListCatalogTool(t *testing.T) {
	cat := &fakeCatalog{entries: []CatalogEntry{
		{Namespace: "domains/pricing", Name: "base-price", Model: "sha256:aaa", Owner: "@pricing", Layer: "L1", Tags: []string{"pii"}, Status: "active", Resolved: true},
		{Namespace: "domains/pricing", Name: "discount", Model: "sha256:bbb", Layer: "L1", Status: "deprecated", Resolved: false},
		{Namespace: "domains/risk", Name: "scoring", Model: "sha256:ccc", Status: "active", Resolved: false},
	}}
	s := NewServer(nil, WithCatalog(cat))
	list := func(args string) map[string]any { return run(t, s, call(1, "list_catalog", args))[0].payload(t) }
	decisions := func(p map[string]any) []any { return p["decisions"].([]any) }

	all := list(`{}`)
	if all["count"].(float64) != 3 || all["total"].(float64) != 3 {
		t.Fatalf("unfiltered = count %v total %v, want 3/3", all["count"], all["total"])
	}
	// Sorted by coordinate; the first entry carries its coordinate and resolved flag.
	first := decisions(all)[0].(map[string]any)
	if first["coordinate"] != "domains/pricing/base-price" || first["resolved"] != true || first["modelId"] != "sha256:aaa" {
		t.Errorf("first entry = %+v", first)
	}
	// An unloaded revision is still listed — the point of list_catalog vs list_models.
	second := decisions(all)[1].(map[string]any)
	if second["coordinate"] != "domains/pricing/discount" || second["resolved"] != false {
		t.Errorf("unloaded entry not listed correctly: %+v", second)
	}

	if p := list(`{"namespace":"domains/pricing"}`); p["count"].(float64) != 2 {
		t.Errorf("namespace=domains/pricing → %v, want 2", p["count"])
	}
	if p := list(`{"status":"deprecated"}`); p["count"].(float64) != 1 {
		t.Errorf("status=deprecated → %v, want 1", p["count"])
	}
	if p := list(`{"tags":["pii"]}`); p["count"].(float64) != 1 {
		t.Errorf("tags=[pii] → %v, want 1", p["count"])
	}
	if p := list(`{"limit":1,"offset":2}`); p["count"].(float64) != 1 || p["total"].(float64) != 3 {
		t.Errorf("limit=1 offset=2 → count %v total %v, want 1/3", p["count"], p["total"])
	}
}

// TestListCatalogToolNoCatalog proves the tool answers with an empty catalog when
// none is configured (the stdio default), rather than erroring.
func TestListCatalogToolNoCatalog(t *testing.T) {
	s := NewServer(nil)
	p := run(t, s, call(1, "list_catalog", `{}`))[0].payload(t)
	if p["count"].(float64) != 0 || p["total"].(float64) != 0 {
		t.Errorf("no catalog → %v/%v, want 0/0", p["count"], p["total"])
	}
}

// TestListModelsCatalogFilters proves list_models over MCP carries catalog
// metadata and honours the namespace/tag/status filters and limit/offset paging
// (ADR-0034, WP-141), agreeing with the HTTP surface.
func TestListModelsCatalogFilters(t *testing.T) {
	fs := &fakeStore{infos: []ModelInfo{
		{ID: "sha256:aaa", Decisions: []string{"Base Price"}, Namespace: "domains/pricing", Name: "base-price", Tags: []string{"pii"}, Status: "active"},
		{ID: "sha256:bbb", Decisions: []string{"Score"}, Namespace: "domains/risk", Name: "scoring", Status: "deprecated"},
		{ID: "sha256:ccc", Decisions: []string{"X"}}, // uncatalogued
	}}
	s := NewServer(nil, WithStore(fs))

	list := func(args string) map[string]any {
		return run(t, s, call(1, "list_models", args))[0].payload(t)
	}
	models := func(p map[string]any) []any { return p["models"].([]any) }
	firstNS := func(p map[string]any) string {
		m := models(p)[0].(map[string]any)
		ns, _ := m["namespace"].(string)
		return ns
	}

	// No filter: every model, enriched, catalogued grouped by coordinate first.
	all := list(`{}`)
	if all["count"].(float64) != 3 || all["total"].(float64) != 3 {
		t.Fatalf("unfiltered = count %v total %v, want 3/3", all["count"], all["total"])
	}
	if got := models(all)[0].(map[string]any)["catalogName"]; got != "base-price" {
		t.Errorf("first catalogName = %v, want base-price (sorted by coordinate)", got)
	}

	// Namespace prefix: exact child and parent; uncatalogued excluded.
	if p := list(`{"namespace":"domains/pricing"}`); p["count"].(float64) != 1 || firstNS(p) != "domains/pricing" {
		t.Errorf("namespace=domains/pricing → %v (%s)", p["count"], firstNS(p))
	}
	if p := list(`{"namespace":"domains"}`); p["count"].(float64) != 2 {
		t.Errorf("namespace=domains → %v, want 2", p["count"])
	}
	if p := list(`{"namespace":"domains-x"}`); p["count"].(float64) != 0 {
		t.Errorf("namespace=domains-x must not match domains/* → %v, want 0", p["count"])
	}

	// Status and tag filters.
	if p := list(`{"status":"deprecated"}`); p["count"].(float64) != 1 || firstNS(p) != "domains/risk" {
		t.Errorf("status=deprecated → %v (%s)", p["count"], firstNS(p))
	}
	if p := list(`{"tags":["pii"]}`); p["count"].(float64) != 1 || firstNS(p) != "domains/pricing" {
		t.Errorf("tags=[pii] → %v (%s)", p["count"], firstNS(p))
	}

	// Pagination over the unfiltered, sorted list: total stays the full match count.
	if p := list(`{"limit":1}`); p["count"].(float64) != 1 || p["total"].(float64) != 3 {
		t.Errorf("limit=1 → count %v total %v, want 1/3", p["count"], p["total"])
	}
	if p := list(`{"limit":1,"offset":2}`); p["count"].(float64) != 1 || models(p)[0].(map[string]any)["modelId"] != "sha256:ccc" {
		t.Errorf("limit=1 offset=2 → %+v, want the uncatalogued ccc", models(p))
	}
	if p := list(`{"offset":9}`); p["count"].(float64) != 0 || p["total"].(float64) != 3 {
		t.Errorf("offset past end → count %v total %v, want 0/3", p["count"], p["total"])
	}
}
