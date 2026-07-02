package service

import (
	"net/http"

	"github.com/pblumer/temis/dmn"
)

// handleGetIterator returns a decision's boxed-iteration view (a for/some/every),
// or 404 when the decision's logic is not an iteration. It backs the modeler's
// iterator editor (ADR-0016, WP-66).
func (s *Server) handleGetIterator(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	iv, ok := sm.defs.BoxedIterator(r.PathValue("decision"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "ITERATOR_NOT_FOUND", "no boxed iteration for that decision")
		return
	}
	writeJSON(w, http.StatusOK, iv)
}

// handleSaveIterator rewrites a decision's iteration (kind, variable, collection
// and body), recompiles and caches the model, and returns the saved model's id
// with any compile diagnostics. It is a 404/400 when the model or the decision's
// iteration is absent or the edit is invalid.
func (s *Server) handleSaveIterator(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	var edit dmn.IteratorEdit
	if err := decodeJSON(w, r, &edit); err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	patched, err := dmn.SetBoxedIterator(sm.xml, r.PathValue("decision"), edit)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "ITERATOR_SAVE_FAILED", err.Error())
		return
	}
	s.respondSaved(w, r, patched)
}

// handleCreateIterator gives an undecided decision a fresh boxed iteration (a
// placeholder for), recompiles and caches the model, and returns the new id. It is
// a 404/400 when the decision is unknown or already has logic.
func (s *Server) handleCreateIterator(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	patched, err := dmn.CreateBoxedIterator(sm.xml, r.PathValue("decision"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "ITERATOR_CREATE_FAILED", err.Error())
		return
	}
	s.respondSaved(w, r, patched)
}
