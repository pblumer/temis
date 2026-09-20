package service

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// The boxed-body BKM fixture lives with the round-trip fixtures; the service test
// reads it across packages so there is a single source of truth for it.
const bkmBoxedBodyPath = "../internal/xml/testdata/models/bkm_boxed_body_16.dmn"

// TestLogic_BKMBoxedBody reads a BKM's boxed body (a decision table and a context)
// through the anchored logic route — the case the simple BKM editor showed
// read-only (WP-66).
func TestLogic_BKMBoxedBody(t *testing.T) {
	h := newTestServer(t)
	id := deployFile(t, h, bkmBoxedBodyPath)

	// The BKM view reports the boxed body's kind so the modeler opens the right editor.
	rec := do(t, h, "GET", "/v1/models/"+id+"/bkm/id_rating", "", nil)
	bkm := decode[dmn.BKMView](t, rec)
	if bkm.Simple || bkm.BodyKind != "table" {
		t.Fatalf("BKM view = %+v, want simple=false bodyKind=table", bkm)
	}

	// The table body reads exactly like a decision's table.
	rec = do(t, h, "GET", "/v1/models/"+id+"/logic/bkm/id_rating/table", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET bkm table = %d (body %s)", rec.Code, rec.Body)
	}
	tv := decode[dmn.TableView](t, rec)
	if tv.HitPolicy != "F" || len(tv.Rules) != 2 || len(tv.Inputs) != 1 {
		t.Errorf("table view = %+v", tv)
	}

	// The context body reads through the same route with a different kind.
	rec = do(t, h, "GET", "/v1/models/"+id+"/logic/bkm/id_weight/context", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET bkm context = %d (body %s)", rec.Code, rec.Body)
	}
	cv := decode[dmn.ContextView](t, rec)
	if !cv.Simple || len(cv.Entries) != 1 || cv.Entries[0].Name != "factor" || cv.Result != "base * factor" {
		t.Errorf("context view = %+v", cv)
	}

	// Asking for the wrong kind of a real BKM body is a 404, like the decision routes.
	rec = do(t, h, "GET", "/v1/models/"+id+"/logic/bkm/id_rating/context", "", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET bkm table as context = %d, want 404", rec.Code)
	}
}

// TestSaveLogic_BKMTable_roundTrip edits a BKM's boxed decision-table body and
// evaluates a decision that calls the BKM — proving the edit applied, the
// function's formal parameters survived, and the model recompiled.
func TestSaveLogic_BKMTable_roundTrip(t *testing.T) {
	h := newTestServer(t)
	id := deployFile(t, h, bkmBoxedBodyPath)

	edit := dmn.TableEdit{
		Rules: []dmn.TableRule{
			{InputEntries: []string{">= 80"}, OutputEntries: []string{`"Top"`}},
			{InputEntries: []string{"< 80"}, OutputEntries: []string{`"B"`}},
		},
	}
	body, _ := json.Marshal(edit)
	rec := do(t, h, "POST", "/v1/models/"+id+"/logic/bkm/id_rating/table", "application/json", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("save bkm table = %d (body %s)", rec.Code, rec.Body)
	}
	newID := decode[modelResponse](t, rec).ModelID

	// Grade = Rating(Score); with the edited table, Score 85 (>= 80) now yields "Top".
	evBody, _ := json.Marshal(evaluateModelRequest{Decision: "Grade", Input: map[string]any{"Score": 85}})
	rec = do(t, h, "POST", "/v1/models/"+newID+"/evaluate", "application/json", evBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("evaluate = %d (body %s)", rec.Code, rec.Body)
	}
	if got := decode[evaluateResponse](t, rec).Outputs["Grade"]; got != "Top" {
		t.Errorf("Grade = %v, want \"Top\" after editing the BKM table body", got)
	}

	// The saved revision still reports a boxed table body (the parameters survived).
	rec = do(t, h, "GET", "/v1/models/"+newID+"/bkm/id_rating", "", nil)
	if bkm := decode[dmn.BKMView](t, rec); bkm.Simple || bkm.BodyKind != "table" || len(bkm.Params) != 1 {
		t.Errorf("saved BKM view = %+v, want boxed table body with 1 param", bkm)
	}
}

// TestBewertungExample_boxedBKMBody confirms the bundled Bewertung example (a BKM
// with a boxed decision-table body — the modeler read-only case) compiles and
// evaluates, and that its body is reachable and editable through the logic route.
func TestBewertungExample_boxedBKMBody(t *testing.T) {
	h := newTestServer(t)
	id := deployFile(t, h, "examples/bewertung.dmn")

	// Entscheidung_1 = Bewertung(Punkte); 85 (>= 80) → "sehr gut".
	evBody, _ := json.Marshal(evaluateModelRequest{Decision: "Entscheidung_1", Input: map[string]any{"Punkte": 85}})
	rec := do(t, h, "POST", "/v1/models/"+id+"/evaluate", "application/json", evBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("evaluate = %d (body %s)", rec.Code, rec.Body)
	}
	if got := decode[evaluateResponse](t, rec).Outputs["Entscheidung_1"]; got != "sehr gut" {
		t.Errorf("Entscheidung_1 = %v, want \"sehr gut\"", got)
	}

	// The BKM body is a boxed table, reachable through the logic route.
	rec = do(t, h, "GET", "/v1/models/"+id+"/logic/bkm/id_bewertung/table", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET bewertung table = %d (body %s)", rec.Code, rec.Body)
	}
	if tv := decode[dmn.TableView](t, rec); len(tv.Rules) != 3 {
		t.Errorf("table view = %+v, want 3 rules", tv)
	}
}

// TestLogic_decisionParity checks the anchored logic route returns byte-identical
// JSON to the decision-specific route for a decision, so the shared view builders
// never drift from the decision readers.
func TestLogic_decisionParity(t *testing.T) {
	h := newTestServer(t)
	id := deployFile(t, h, contextModelPath)

	viaDecision := do(t, h, "GET", "/v1/models/"+id+"/decisions/id_score/context", "", nil)
	viaLogic := do(t, h, "GET", "/v1/models/"+id+"/logic/decision/id_score/context", "", nil)
	if viaLogic.Code != http.StatusOK {
		t.Fatalf("GET logic decision context = %d (body %s)", viaLogic.Code, viaLogic.Body)
	}
	if viaDecision.Body.String() != viaLogic.Body.String() {
		t.Errorf("anchored view drifted from decision view:\n decision: %s\n logic:    %s", viaDecision.Body, viaLogic.Body)
	}
}
