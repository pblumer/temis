package service

import (
	"net/http"

	"github.com/pblumer/temis/dmn"
)

// handleGetInvocation returns a decision's boxed-invocation view (the called
// function/BKM and its parameter bindings), or 404 when the decision's logic is
// not a boxed invocation. It backs the modeler's invocation editor (ADR-0016).
func (s *Server) handleGetInvocation(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	iv, ok := sm.defs.BoxedInvocation(r.PathValue("decision"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "INVOCATION_NOT_FOUND", "no boxed invocation for that decision")
		return
	}
	writeJSON(w, http.StatusOK, iv)
}

// handleSaveInvocation rewrites a decision's invocation (called function and
// bindings), recompiles and caches the model, and returns the saved model's id
// with any compile diagnostics (e.g. an unknown called BKM). It is a 404/400 when
// the model or the decision's invocation is absent or the edit is invalid.
func (s *Server) handleSaveInvocation(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	var edit dmn.InvocationEdit
	if err := decodeJSON(w, r, &edit); err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	patched, err := dmn.SetBoxedInvocation(sm.xml, r.PathValue("decision"), edit)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "INVOCATION_SAVE_FAILED", err.Error())
		return
	}
	s.respondSaved(w, r, patched)
}

// handleCreateInvocation gives an undecided decision a fresh boxed invocation
// (placeholder called function and one binding), recompiles and caches the model,
// and returns the new id. It is a 404/400 when the decision is unknown or already
// has logic.
func (s *Server) handleCreateInvocation(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	patched, err := dmn.CreateBoxedInvocation(sm.xml, r.PathValue("decision"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "INVOCATION_CREATE_FAILED", err.Error())
		return
	}
	s.respondSaved(w, r, patched)
}
