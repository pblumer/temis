package service

import (
	"net/http"

	"github.com/pblumer/temis/dmn"
)

// handleGetConditional returns a decision's boxed-conditional view (the three
// FEEL branches of an if/then/else), or 404 when the decision's logic is not a
// boxed conditional. It backs the modeler's conditional editor (ADR-0016, WP-66).
func (s *Server) handleGetConditional(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	cv, ok := sm.defs.BoxedConditional(r.PathValue("decision"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "CONDITIONAL_NOT_FOUND", "no boxed conditional for that decision")
		return
	}
	writeJSON(w, http.StatusOK, cv)
}

// handleSaveConditional rewrites a decision's conditional branches, recompiles and
// caches the model, and returns the saved model's id with any compile diagnostics
// (so the client can surface a FEEL branch the engine rejects). It is a 404/400
// when the model or the decision's conditional is absent or the edit is invalid.
func (s *Server) handleSaveConditional(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	var edit dmn.ConditionalEdit
	if err := decodeJSON(w, r, &edit); err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	patched, err := dmn.SetBoxedConditional(sm.xml, r.PathValue("decision"), edit)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "CONDITIONAL_SAVE_FAILED", err.Error())
		return
	}
	s.respondSaved(w, r, patched)
}

// handleCreateConditional gives an undecided decision a fresh boxed conditional
// (placeholder branches), recompiles and caches the model, and returns the new id.
// It is a 404/400 when the decision is unknown or already has logic.
func (s *Server) handleCreateConditional(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	patched, err := dmn.CreateBoxedConditional(sm.xml, r.PathValue("decision"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "CONDITIONAL_CREATE_FAILED", err.Error())
		return
	}
	s.respondSaved(w, r, patched)
}
