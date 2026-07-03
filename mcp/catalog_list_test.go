package mcp

import "testing"

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
