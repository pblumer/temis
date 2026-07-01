package service

import (
	"net/http"

	"github.com/pblumer/temis/dmn"
)

// handleGetRelation returns a decision's boxed-relation view (named columns and
// rows of FEEL cells), or 404 when the decision's logic is not a boxed relation.
// It backs the modeler's relation editor (ADR-0016, WP-66).
func (s *Server) handleGetRelation(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	rv, ok := sm.defs.BoxedRelation(r.PathValue("decision"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "RELATION_NOT_FOUND", "no boxed relation for that decision")
		return
	}
	writeJSON(w, http.StatusOK, rv)
}

// handleSaveRelation rewrites a decision's relation columns and rows, recompiles
// and caches the model, and returns the saved model's id with any compile
// diagnostics (so the client can surface a FEEL cell the engine rejects). It is a
// 404/400 when the model or the decision's relation is absent or the edit is
// invalid.
func (s *Server) handleSaveRelation(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	var edit dmn.RelationEdit
	if err := decodeJSON(w, r, &edit); err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	patched, err := dmn.SetBoxedRelation(sm.xml, r.PathValue("decision"), edit)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "RELATION_SAVE_FAILED", err.Error())
		return
	}
	s.respondSaved(w, r, patched)
}

// handleCreateRelation gives an undecided decision a fresh boxed relation (one
// column, one placeholder cell), recompiles and caches the model, and returns the
// new id. It is a 404/400 when the decision is unknown or already has logic.
func (s *Server) handleCreateRelation(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	patched, err := dmn.CreateBoxedRelation(sm.xml, r.PathValue("decision"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "RELATION_CREATE_FAILED", err.Error())
		return
	}
	s.respondSaved(w, r, patched)
}
