package service

// Public decisions (ADR-0035). Access control (ADR-0028) is otherwise binary per
// server: with any key configured every route needs a scoped bearer. This lets an
// operator carve out an exception for *evaluation only*, so a decision can be
// exposed to anonymous callers while models:write, admin, assist, git and flow
// stay locked. Two independent openings, both opt-in and off by default:
//
//   - a global switch (WithPublicEvaluate / -public-evaluate / TEMIS_PUBLIC_EVALUATE)
//     opens every evaluation surface, including the stateless POST /v1/evaluate;
//   - a per-model allowlist (WithPublicModels / -public-models / TEMIS_PUBLIC_MODELS)
//     opens just the addressed model, matched by its modelId or its display name.
//
// Only the evaluate scope can be opened this way; the write/admin/assist/git/flow
// scopes are never affected. The gate lives in the adapter layer (requireScope,
// the gRPC interceptor and the MCP gate consult it); the engine core is untouched
// (ADR-0011).

// evaluateIsPublic reports whether an evaluate request addressed at model id
// should be served to an anonymous caller even when auth is configured. It is
// consulted only for the evaluate scope. An empty id (a resource-less route such
// as the stateless POST /v1/evaluate) is public only under the global switch.
func (s *Server) evaluateIsPublic(id string) bool {
	if s.publicEvaluate {
		return true
	}
	return s.isPublicModel(id)
}

// isPublicModel reports whether the model addressed by id is on the per-model
// public allowlist, matched by its content-addressed modelId or, for a currently
// cached model, its display name (so a re-saved model — which gets a new modelId
// — stays public when the operator listed it by name). An empty id is never
// public per-model. The name lookup reads only the in-memory cache, never a disk
// compile, so an anonymous request cannot force work merely by probing an id.
func (s *Server) isPublicModel(id string) bool {
	if id == "" || s.publicModels == nil || s.publicModels.empty() {
		return false
	}
	if s.publicModels.has(id) {
		return true // matched by modelId
	}
	if sm, ok := s.cache.get(id); ok && s.publicModels.has(sm.name) {
		return true // matched by display name
	}
	return false
}
