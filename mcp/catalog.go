package mcp

import (
	"encoding/json"
	"sort"
)

// The list_catalog tool exposes the decision catalog (ADR-0034, WP-143): the
// authoritative namespace/name index over the model store. Unlike list_models —
// which lists the models currently in the cache, enriched with catalog metadata —
// list_catalog answers from the catalog itself, so an agent can ask "which
// decisions exist under domains/pricing?" without every revision being loaded
// (and without a full scan). It shares list_models' namespace/tag/status filters
// and limit/offset paging.

// CatalogEntry mirrors one service catalog entry for the list_catalog tool: a
// decision's coordinate (namespace + name), the model revision it pins, its
// governance metadata and whether that revision is currently loaded.
type CatalogEntry struct {
	Namespace string
	Name      string
	Model     string
	Owner     string
	Layer     string
	Tags      []string
	Status    string
	Resolved  bool
}

// Catalog is the decision catalog the list_catalog tool queries. Splitting it out
// (like Store and FlowStore) lets the MCP server share the very catalog the HTTP
// service loaded from -catalog-dir, so both surfaces agree. Implementations must
// be safe for concurrent use.
type Catalog interface {
	// List returns every catalog entry, in any order (the tool sorts and filters).
	List() []CatalogEntry
}

// WithCatalog backs the server's list_catalog tool with the given catalog, so a
// co-located MCP endpoint queries the same catalog the HTTP service loaded. A nil
// catalog is ignored, leaving list_catalog to answer with an empty catalog.
func WithCatalog(c Catalog) Option {
	return func(s *Server) {
		if c != nil {
			s.catalog = c
		}
	}
}

// catalogTools is the catalog slice of the tool catalogue.
var catalogTools = []toolSpec{
	{
		Name: "list_catalog",
		Description: "List the decision catalog: the authoritative namespace/name index over " +
			"this server's models (ADR-0034). Unlike list_models it answers from the catalog " +
			"itself, so it reports decisions that exist even when their revision is not currently " +
			"loaded — use it to discover what decisions exist and where they live (e.g. \"what is " +
			"under domains/pricing?\") without a full scan. Each entry carries its coordinate, the " +
			"pinned modelId, owner, layer, tags, status and whether that revision is loaded. " +
			"Optional filters (namespace prefix, tags — all must match, status) and limit/offset " +
			"paging narrow the result.",
		InputSchema: obj(map[string]any{
			"namespace": str("Only decisions at or under this namespace (e.g. \"domains/pricing\")."),
			"status":    str("Only decisions in this lifecycle status (active, deprecated or archived)."),
			"tags":      arr("Only decisions carrying ALL of these catalog tags.", str("A catalog tag.")),
			"limit":     intProp("Maximum number of decisions to return (0 = no limit)."),
			"offset":    intProp("Number of matching decisions to skip before returning (for paging)."),
		}),
	},
}

func init() { tools = append(tools, catalogTools...) }

// catalogSummary is one decision in the list_catalog result. coordinate is
// "namespace/name" (or just "name" at the root), the decision's stable identity.
type catalogSummary struct {
	Coordinate string   `json:"coordinate"`
	Namespace  string   `json:"namespace,omitempty"`
	Name       string   `json:"name"`
	ModelID    string   `json:"modelId"`
	Owner      string   `json:"owner,omitempty"`
	Layer      string   `json:"layer,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Status     string   `json:"status,omitempty"`
	Resolved   bool     `json:"resolved"`
}

func coordOf(e CatalogEntry) string {
	if e.Namespace == "" {
		return e.Name
	}
	return e.Namespace + "/" + e.Name
}

func (s *Server) toolListCatalog(raw json.RawMessage) (any, *rpcError) {
	var a struct {
		Namespace string   `json:"namespace"`
		Status    string   `json:"status"`
		Tags      []string `json:"tags"`
		Limit     int      `json:"limit"`
		Offset    int      `json:"offset"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &a); err != nil {
			return toolError("invalid arguments: " + err.Error()), nil
		}
	}

	var entries []CatalogEntry
	if s.catalog != nil {
		entries = s.catalog.List()
	}
	out := make([]catalogSummary, 0, len(entries))
	for _, e := range entries {
		if !namespaceMatches(e.Namespace, a.Namespace) ||
			(a.Status != "" && e.Status != a.Status) || !hasAllTags(e.Tags, a.Tags) {
			continue
		}
		out = append(out, catalogSummary{
			Coordinate: coordOf(e), Namespace: e.Namespace, Name: e.Name, ModelID: e.Model,
			Owner: e.Owner, Layer: e.Layer, Tags: e.Tags, Status: e.Status, Resolved: e.Resolved,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Coordinate < out[j].Coordinate })
	total := len(out)
	out = pageCatalog(out, a.Offset, a.Limit)
	return toolText(map[string]any{"decisions": out, "count": len(out), "total": total})
}

// pageCatalog returns the window [offset, offset+limit) of xs; limit 0 means no
// bound, an offset past the end yields empty.
func pageCatalog(xs []catalogSummary, offset, limit int) []catalogSummary {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(xs) {
		return xs[:0]
	}
	xs = xs[offset:]
	if limit > 0 && limit < len(xs) {
		xs = xs[:limit]
	}
	return xs
}
