package service

import (
	"encoding/json"
	"net/http"

	"github.com/pblumer/temis/dmn"
)

// handleGetLogic returns the anchored element's boxed logic — a decision's logic
// (anchorKind "decision") or a business knowledge model's encapsulated body
// (anchorKind "bkm") — as the {kind} editor view, or 404 when the anchor or its
// logic kind is absent. It lets the modeler open the per-kind boxed editors on a
// BKM's boxed body, not just a decision's logic (ADR-0016, WP-66).
func (s *Server) handleGetLogic(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	a := dmn.Anchor{Kind: r.PathValue("anchorKind"), ID: r.PathValue("anchorId")}
	v, ok := sm.defs.LogicView(a, r.URL.Query().Get("at"), r.PathValue("kind"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "LOGIC_NOT_FOUND", "no boxed logic of that kind for that anchor")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// handleSaveLogic rewrites the anchored element's boxed logic of the given kind,
// recompiles and caches the model, and returns the saved model's id with any
// compile diagnostics (so the client can surface a FEEL cell the engine rejects).
// It is a 404/400 when the model, the anchor, or the edit is absent or invalid.
func (s *Server) handleSaveLogic(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.lookup(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "MODEL_NOT_FOUND", "no model with that id")
		return
	}
	var raw json.RawMessage
	if err := decodeJSON(w, r, &raw); err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	a := dmn.Anchor{Kind: r.PathValue("anchorKind"), ID: r.PathValue("anchorId")}
	patched, err := dmn.SetLogic(sm.xml, a, r.URL.Query().Get("at"), r.PathValue("kind"), raw)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "LOGIC_SAVE_FAILED", err.Error())
		return
	}
	s.respondSaved(w, r, patched)
}
