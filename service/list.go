package service

import (
	"net/http"

	"github.com/pblumer/temis/dmn"
)

// handleGetList returns a decision's boxed-list view (its ordered FEEL items), or
// 404 when the decision's logic is not a boxed list. It backs the modeler's
// boxed-list editor (ADR-0016, WP-66).
func (s *Server) handleGetList(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	lv, ok := sm.defs.BoxedList(r.PathValue("decision"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "LIST_NOT_FOUND", "no boxed list for that decision")
		return
	}
	writeJSON(w, http.StatusOK, lv)
}

// handleSaveList rewrites a decision's list items, recompiles and caches the
// model, and returns the saved model's id with any compile diagnostics (so the
// client can surface a FEEL item the engine rejects). It is a 404/400 when the
// model or the decision's list is absent or the edit is invalid.
func (s *Server) handleSaveList(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	var edit dmn.ListEdit
	if err := decodeJSON(w, r, &edit); err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	patched, err := dmn.SetBoxedList(sm.xml, r.PathValue("decision"), edit)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "LIST_SAVE_FAILED", err.Error())
		return
	}
	s.respondSaved(w, r, patched)
}

// handleCreateList gives an undecided decision a fresh boxed list (a single
// placeholder item), recompiles and caches the model, and returns the new id. It
// is a 404/400 when the decision is unknown or already has logic.
func (s *Server) handleCreateList(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	patched, err := dmn.CreateBoxedList(sm.xml, r.PathValue("decision"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "LIST_CREATE_FAILED", err.Error())
		return
	}
	s.respondSaved(w, r, patched)
}
