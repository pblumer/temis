package service

import (
	"net/http"
	"testing"
)

// TestRenameModel renames a cached model and checks a new revision is stored
// under a new id with the new name, while the original stays cached.
func TestRenameModel(t *testing.T) {
	h := newTestServer(t)
	created := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", dishXML(t)))

	rec := do(t, h, "POST", "/v1/models/"+created.ModelID+"/rename", "application/json", []byte(`{"name":"Speisekarte"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("rename = %d, want 201 (body %s)", rec.Code, rec.Body)
	}
	renamed := decode[modelResponse](t, rec)
	if renamed.Name != "Speisekarte" {
		t.Errorf("renamed name = %q, want Speisekarte", renamed.Name)
	}
	if renamed.ModelID == created.ModelID {
		t.Error("rename should produce a new content-addressed id")
	}
	// Decisions are preserved through the rename.
	if !contains(renamed.Decisions, "Dish") {
		t.Errorf("renamed decisions = %v, want to contain Dish", renamed.Decisions)
	}
	// The renamed revision is fetchable, and the original is still cached.
	if rec := do(t, h, "GET", "/v1/models/"+renamed.ModelID, "", nil); rec.Code != http.StatusOK {
		t.Errorf("GET renamed = %d, want 200", rec.Code)
	}
	if rec := do(t, h, "GET", "/v1/models/"+created.ModelID, "", nil); rec.Code != http.StatusOK {
		t.Errorf("GET original after rename = %d, want 200 (original stays cached)", rec.Code)
	}
}

func TestRenameModelEmptyName(t *testing.T) {
	h := newTestServer(t)
	created := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", dishXML(t)))
	for _, body := range []string{`{"name":""}`, `{"name":"   "}`, `{}`} {
		if rec := do(t, h, "POST", "/v1/models/"+created.ModelID+"/rename", "application/json", []byte(body)); rec.Code != http.StatusBadRequest {
			t.Errorf("rename body %s = %d, want 400", body, rec.Code)
		}
	}
}

func TestRenameModelNotFound(t *testing.T) {
	h := newTestServer(t)
	if rec := do(t, h, "POST", "/v1/models/sha256:deadbeef/rename", "application/json", []byte(`{"name":"X"}`)); rec.Code != http.StatusNotFound {
		t.Errorf("rename unknown = %d, want 404", rec.Code)
	}
}

// TestDeleteModel removes a cached model and checks it is gone and a repeat
// delete is a 404.
func TestDeleteModel(t *testing.T) {
	h := newTestServer(t)
	created := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", dishXML(t)))

	if rec := do(t, h, "DELETE", "/v1/models/"+created.ModelID, "", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d, want 204 (body %s)", rec.Code, rec.Body)
	}
	if rec := do(t, h, "GET", "/v1/models/"+created.ModelID, "", nil); rec.Code != http.StatusNotFound {
		t.Errorf("GET after delete = %d, want 404", rec.Code)
	}
	if rec := do(t, h, "DELETE", "/v1/models/"+created.ModelID, "", nil); rec.Code != http.StatusNotFound {
		t.Errorf("second delete = %d, want 404", rec.Code)
	}
}
