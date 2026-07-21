package service

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// WP-142: a flow step that references a release is frozen to the concrete modelId
// at registration, so a later channel move never changes what the flow evaluates.
func TestFreezeFlowReleaseRefs(t *testing.T) {
	h := newTestServer(t)
	id, name := loadDish(t, h)
	if rec := do(t, h, "POST", "/v1/releases", "application/json", publishBody(t, publishReleaseRequest{ModelID: id, Name: name, Version: "1.0.0"})); rec.Code != http.StatusCreated {
		t.Fatalf("publish = %d (body %s)", rec.Code, rec.Body)
	}

	// Register a flow whose step references the readable release "Dish@1.0.0".
	desc := []byte(`{"flow":"t","steps":[{"id":"d","model":"` + name + `@1.0.0","decision":"Dish","in":{"Season":"Season","Guest Count":"Guest Count"}}],"output":{"Dish":"d.Dish"}}`)
	reg := decode[flowResponse](t, do(t, h, "POST", "/v1/flows", "application/json", desc))
	if reg.FlowID == "" {
		t.Fatal("no flowId returned")
	}

	// The stored flow pins the concrete content-addressed id, not the release ref.
	detail := decode[flowDetail](t, do(t, h, "GET", "/v1/flows/"+reg.FlowID, "", nil))
	if len(detail.Steps) != 1 {
		t.Fatalf("steps = %d, want 1", len(detail.Steps))
	}
	if detail.Steps[0].Model != id {
		t.Errorf("step model = %q, want the frozen id %q", detail.Steps[0].Model, id)
	}
}

// WP-143: GC prunes only unreferenced drafts — a release and the newest cached
// revision (the head) survive; an older, unpublished, unreferenced draft is
// removed from both the cache and the on-disk store.
func TestGCPrunesUnreferencedDrafts(t *testing.T) {
	dir := t.TempDir()
	h := NewServer(dmn.New(), WithModelStore(dir)).Handler()
	base := dishXML(t)
	// Three same-named revisions (an XML comment changes the bytes, not the name).
	variant := func(tag string) []byte {
		return bytes.Replace(base, []byte("</definitions>"), []byte("<!-- "+tag+" --></definitions>"), 1)
	}
	id1 := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", variant("r1"))).ModelID
	id2 := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", variant("r2"))).ModelID
	id3 := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", variant("r3"))).ModelID
	if id1 == id2 || id2 == id3 {
		t.Fatalf("variants collided: %s %s %s", id1, id2, id3)
	}
	// Publish the oldest as a release; id3 is the newest (head). id2 is the only
	// prunable draft: not a release, not referenced, not the head.
	if rec := do(t, h, "POST", "/v1/releases", "application/json", publishBody(t, publishReleaseRequest{ModelID: id1, Name: "Dish", Version: "1.0.0"})); rec.Code != http.StatusCreated {
		t.Fatalf("publish = %d (body %s)", rec.Code, rec.Body)
	}

	resp := decode[gcResponse](t, do(t, h, "POST", "/v1/models/gc", "", nil))
	if !contains(resp.Deleted, id2) {
		t.Errorf("deleted = %v, want to contain the middle draft %s", resp.Deleted, id2)
	}
	if contains(resp.Deleted, id1) || contains(resp.Deleted, id3) {
		t.Errorf("deleted = %v, must not contain released %s or head %s", resp.Deleted, id1, id3)
	}

	// Released and head revisions still resolve; the pruned one is gone from both
	// cache and store.
	if rec := do(t, h, "GET", "/v1/models/"+id1, "", nil); rec.Code != http.StatusOK {
		t.Errorf("released id1 after GC = %d, want 200", rec.Code)
	}
	if rec := do(t, h, "GET", "/v1/models/"+id3, "", nil); rec.Code != http.StatusOK {
		t.Errorf("head id3 after GC = %d, want 200", rec.Code)
	}
	if rec := do(t, h, "GET", "/v1/models/"+id2, "", nil); rec.Code != http.StatusNotFound {
		t.Errorf("pruned id2 after GC = %d, want 404", rec.Code)
	}
	// The release still resolves by name after GC (its revision was kept).
	eval := publishBody(t, evaluateModelRequest{Decision: "Dish", Input: map[string]any{"Season": "Winter", "Guest Count": float64(8)}})
	if rec := do(t, h, "POST", "/v1/models/Dish@1.0.0/evaluate", "application/json", eval); rec.Code != http.StatusOK {
		t.Errorf("evaluate Dish@1.0.0 after GC = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
}
