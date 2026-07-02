package service

import (
	"net/http"
	"strconv"
)

// clioEventsResponse is the body of GET /v1/clio/events: whether the audit sink
// is configured at all, the subject subtree that was queried, the sink's default
// subject mapping (so the modeler can pre-fill it), the temis event types a
// reader can filter on, and the replay-relevant events read back from clio.
type clioEventsResponse struct {
	Enabled       bool        `json:"enabled"`
	Subject       string      `json:"subject,omitempty"`
	SubjectPrefix string      `json:"subjectPrefix,omitempty"`
	SubjectKey    string      `json:"subjectKey,omitempty"`
	Types         []string    `json:"types"`
	Events        []ClioEvent `json:"events"`
}

// clioReplayTypes are the temis CloudEvents types the Operate replay can read and
// map back into a run. decision/flow evaluated events carry a recorded input the
// modeler can re-evaluate; the command event is what an Umsystem writes to request
// one (ADR-0033). They are offered as the event-type filter in the mapping UI.
func clioReplayTypes() []string {
	return []string{DecisionEventType, FlowEventType, "com.temis.decision.requested.v1"}
}

// handleClioEvents reads decision/flow events back from clio for replay in the
// Operate view (ADR-0033 read side). The browser never holds the clio token: the
// server queries clio on its behalf over the sink's existing connection and
// returns only the replay-relevant fields. When no sink is configured it answers
// 200 with enabled:false (not an error), so the modeler shows "clio off" rather
// than a failure. A clio read failure is a 502. Guarded by the audit scope, like
// GET /v1/status.
func (s *Server) handleClioEvents(w http.ResponseWriter, r *http.Request) {
	if s.sink == nil {
		writeJSON(w, http.StatusOK, clioEventsResponse{Enabled: false, Types: clioReplayTypes(), Events: []ClioEvent{}})
		return
	}
	q := r.URL.Query()
	subject := q.Get("subject")
	eventType := q.Get("type")
	limit := 0
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}

	events, err := s.sink.Query(r.Context(), subject, eventType, limit)
	if err != nil {
		writeProblem(w, http.StatusBadGateway, "CLIO_UNAVAILABLE", "reading events from clio failed: "+err.Error())
		return
	}

	snap := s.sink.snapshot()
	resolved := subject
	if resolved == "" {
		resolved = snap.subjectPrefix
	}
	writeJSON(w, http.StatusOK, clioEventsResponse{
		Enabled:       true,
		Subject:       resolved,
		SubjectPrefix: snap.subjectPrefix,
		SubjectKey:    snap.subjectKey,
		Types:         clioReplayTypes(),
		Events:        events,
	})
}
