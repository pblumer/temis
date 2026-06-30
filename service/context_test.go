package service

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/pblumer/temis/dmn"
)

func deployFile(t *testing.T, h http.Handler, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	rec := do(t, h, "POST", "/v1/models", "application/xml", b)
	if rec.Code != http.StatusCreated {
		t.Fatalf("deploy %s = %d (body %s)", path, rec.Code, rec.Body)
	}
	return decode[modelResponse](t, rec).ModelID
}

const contextModelPath = "../dmn/testdata/models/boxed_context_15.dmn"

func TestGetContext(t *testing.T) {
	h := newTestServer(t)
	id := deployFile(t, h, contextModelPath)

	rec := do(t, h, "GET", "/v1/models/"+id+"/decisions/id_score/context", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET context = %d (body %s)", rec.Code, rec.Body)
	}
	cv := decode[dmn.ContextView](t, rec)
	if !cv.Simple || len(cv.Entries) != 2 || cv.Entries[0].Name != "Base" || cv.Result != "Bonus" {
		t.Errorf("context view = %+v", cv)
	}

	// A decision table is not a boxed context → 404.
	dishID := deployFile(t, h, dishModelPath)
	rec = do(t, h, "GET", "/v1/models/"+dishID+"/decisions/id_dish/context", "", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET context on a table = %d, want 404", rec.Code)
	}
}

func TestSaveContext_roundTripEvaluate(t *testing.T) {
	h := newTestServer(t)
	id := deployFile(t, h, contextModelPath)

	edit := dmn.ContextEdit{
		Entries: []dmn.ContextEntryView{
			{Name: "Base", Text: "Points * 3", TypeRef: "number"},
			{Name: "Bonus", Text: "Base + 10", TypeRef: "number"},
		},
		Result: "Bonus",
	}
	body, _ := json.Marshal(edit)
	rec := do(t, h, "POST", "/v1/models/"+id+"/decisions/id_score/context", "application/json", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("save context = %d (body %s)", rec.Code, rec.Body)
	}
	newID := decode[modelResponse](t, rec).ModelID

	// Evaluate the saved revision: (5*3)+10 = 25.
	evBody, _ := json.Marshal(evaluateModelRequest{Decision: "Score", Input: map[string]any{"Points": 5}})
	rec = do(t, h, "POST", "/v1/models/"+newID+"/evaluate", "application/json", evBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("evaluate = %d (body %s)", rec.Code, rec.Body)
	}
	if got := decode[evaluateResponse](t, rec).Outputs["Score"]; got != "25" {
		t.Errorf("Score = %v, want 25 after edit", got)
	}
}

func TestCreateContext_rejectsDecided(t *testing.T) {
	h := newTestServer(t)
	id := deployFile(t, h, contextModelPath)

	rec := do(t, h, "POST", "/v1/models/"+id+"/decisions/id_score/create-context", "application/json", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("create-context on a decided decision = %d, want 400 (body %s)", rec.Code, rec.Body)
	}
}
