package service

import (
	"net/http"

	"github.com/pblumer/temis/dmn"
)

// handleGetContext returns a decision's boxed-context view (named literal entries
// plus an optional result cell), or 404 when the decision's logic is not a boxed
// context. It backs the modeler's boxed-context editor (ADR-0016, WP-66).
func (s *Server) handleGetContext(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	cv, ok := sm.defs.BoxedContext(r.PathValue("decision"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "CONTEXT_NOT_FOUND", "no boxed context for that decision")
		return
	}
	writeJSON(w, http.StatusOK, cv)
}

// handleSaveContext rewrites a decision's boxed-context entries, recompiles and
// caches the model, and returns the saved model's id with any compile
// diagnostics (so the client can surface a FEEL cell the engine rejects). It is a
// 404/400 when the model or the decision's context is absent or the edit is
// invalid.
func (s *Server) handleSaveContext(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	var edit dmn.ContextEdit
	if err := decodeJSON(w, r, &edit); err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	patched, err := dmn.SetBoxedContext(sm.xml, r.PathValue("decision"), edit)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "CONTEXT_SAVE_FAILED", err.Error())
		return
	}
	s.respondSaved(w, r, patched)
}

// handleCreateContext gives an undecided decision a fresh boxed context (a single
// named entry), recompiles and caches the model, and returns the new id. It is a
// 404/400 when the decision is unknown or already has logic (ADR-0016, WP-66).
func (s *Server) handleCreateContext(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	patched, err := dmn.CreateBoxedContext(sm.xml, r.PathValue("decision"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "CONTEXT_CREATE_FAILED", err.Error())
		return
	}
	s.respondSaved(w, r, patched)
}
