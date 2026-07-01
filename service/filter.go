package service

import (
	"net/http"

	"github.com/pblumer/temis/dmn"
)

// handleGetFilter returns a decision's boxed-filter view (the collection and
// predicate), or 404 when the decision's logic is not a boxed filter. It backs the
// modeler's filter editor (ADR-0016, WP-66).
func (s *Server) handleGetFilter(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	fv, ok := sm.defs.BoxedFilter(r.PathValue("decision"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "FILTER_NOT_FOUND", "no boxed filter for that decision")
		return
	}
	writeJSON(w, http.StatusOK, fv)
}

// handleSaveFilter rewrites a decision's filter branches, recompiles and caches
// the model, and returns the saved model's id with any compile diagnostics (so the
// client can surface a FEEL branch the engine rejects). It is a 404/400 when the
// model or the decision's filter is absent or the edit is invalid.
func (s *Server) handleSaveFilter(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	var edit dmn.FilterEdit
	if err := decodeJSON(w, r, &edit); err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	patched, err := dmn.SetBoxedFilter(sm.xml, r.PathValue("decision"), edit)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "FILTER_SAVE_FAILED", err.Error())
		return
	}
	s.respondSaved(w, r, patched)
}

// handleCreateFilter gives an undecided decision a fresh boxed filter (placeholder
// branches), recompiles and caches the model, and returns the new id. It is a
// 404/400 when the decision is unknown or already has logic.
func (s *Server) handleCreateFilter(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	patched, err := dmn.CreateBoxedFilter(sm.xml, r.PathValue("decision"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "FILTER_CREATE_FAILED", err.Error())
		return
	}
	s.respondSaved(w, r, patched)
}
