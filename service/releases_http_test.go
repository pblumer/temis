package service

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// loadDish uploads the dish model and returns its id and display name.
func loadDish(t *testing.T, h http.Handler) (string, string) {
	t.Helper()
	resp := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", dishXML(t)))
	return resp.ModelID, resp.Name
}

func publishBody(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPublishAndListReleases(t *testing.T) {
	h := newTestServer(t)
	id, name := loadDish(t, h)

	// Publish 1.0.0 by id alone (name defaults to the model's display name).
	rec := do(t, h, "POST", "/v1/releases", "application/json", publishBody(t, publishReleaseRequest{ModelID: id, Version: "1.0.0", Notes: "first cut"}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("publish = %d, want 201 (body %s)", rec.Code, rec.Body)
	}
	got := decode[modelReleasesDTO](t, rec)
	if got.Name != name || len(got.Releases) != 1 || got.Releases[0].Version != "1.0.0" {
		t.Fatalf("publish response = %+v, want one 1.0.0 release named %q", got, name)
	}
	if got.Channels["latest"] != "1.0.0" {
		t.Errorf("latest channel = %q, want 1.0.0", got.Channels["latest"])
	}

	// The catalog lists it.
	list := decode[releasesListResponse](t, do(t, h, "GET", "/v1/releases", "", nil))
	if list.Count != 1 || list.Models[0].Name != name {
		t.Errorf("list = %+v, want one model %q", list, name)
	}

	// GET /v1/releases/{name} returns it.
	one := decode[modelReleasesDTO](t, do(t, h, "GET", "/v1/releases/"+name, "", nil))
	if len(one.Releases) != 1 || one.Releases[0].ModelID != id {
		t.Errorf("get releases = %+v, want the published id %q", one, id)
	}
}

func TestPublishReleaseImmutable(t *testing.T) {
	h := newTestServer(t)
	id, _ := loadDish(t, h)
	body := publishBody(t, publishReleaseRequest{ModelID: id, Name: "Dish", Version: "1.0.0"})
	if rec := do(t, h, "POST", "/v1/releases", "application/json", body); rec.Code != http.StatusCreated {
		t.Fatalf("first publish = %d, want 201", rec.Code)
	}
	rec := do(t, h, "POST", "/v1/releases", "application/json", body)
	if rec.Code != http.StatusConflict {
		t.Fatalf("re-publish = %d, want 409 (body %s)", rec.Code, rec.Body)
	}
	if p := decode[problem](t, rec); p.Code != "RELEASE_EXISTS" {
		t.Errorf("code = %q, want RELEASE_EXISTS", p.Code)
	}
}

func TestPublishReleaseErrors(t *testing.T) {
	h := newTestServer(t)
	id, _ := loadDish(t, h)

	// Unknown model.
	rec := do(t, h, "POST", "/v1/releases", "application/json", publishBody(t, publishReleaseRequest{ModelID: "sha256:" + "0000000000000000000000000000000000000000000000000000000000000000", Version: "1.0.0"}))
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown model publish = %d, want 404", rec.Code)
	}
	// Malformed version.
	rec = do(t, h, "POST", "/v1/releases", "application/json", publishBody(t, publishReleaseRequest{ModelID: id, Version: "nope"}))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad version publish = %d, want 400", rec.Code)
	}
	if p := decode[problem](t, rec); p.Code != "INVALID_VERSION" {
		t.Errorf("code = %q, want INVALID_VERSION", p.Code)
	}
}

func TestChannelAndEvaluateByReleaseRef(t *testing.T) {
	h := newTestServer(t)
	id, name := loadDish(t, h)
	for _, v := range []string{"1.0.0", "2.0.0"} {
		if rec := do(t, h, "POST", "/v1/releases", "application/json", publishBody(t, publishReleaseRequest{ModelID: id, Name: name, Version: v})); rec.Code != http.StatusCreated {
			t.Fatalf("publish %s = %d, want 201", v, rec.Code)
		}
	}
	// Point stable at 1.0.0.
	if rec := do(t, h, "POST", "/v1/releases/"+name+"/channels", "application/json", publishBody(t, setChannelRequest{Channel: "stable", Version: "1.0.0"})); rec.Code != http.StatusOK {
		t.Fatalf("setChannel = %d, want 200 (body %s)", rec.Code, rec.Body)
	}

	// Evaluate the dish decision through a release reference instead of a raw id.
	evalBody := publishBody(t, evaluateModelRequest{Decision: "Dish", Input: map[string]any{"Season": "Winter", "Guest Count": float64(8)}})
	rec := do(t, h, "POST", "/v1/models/"+name+"@stable/evaluate", "application/json", evalBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("evaluate by release ref = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
	out := decode[evaluateResponse](t, rec)
	if out.Outputs["Dish"] != "Roastbeef" {
		t.Errorf("Dish via %s@stable = %v, want Roastbeef", name, out.Outputs["Dish"])
	}
}

func TestReleasesSurviveRestart(t *testing.T) {
	dir := t.TempDir()
	first := NewServer(dmn.New(), WithModelStore(dir)).Handler()
	id, name := loadDish(t, first)
	if rec := do(t, first, "POST", "/v1/releases", "application/json", publishBody(t, publishReleaseRequest{ModelID: id, Name: name, Version: "1.0.0"})); rec.Code != http.StatusCreated {
		t.Fatalf("publish = %d, want 201 (body %s)", rec.Code, rec.Body)
	}

	// A fresh server over the same dir reloads both the model and its release, so
	// evaluating by release reference works after a "restart".
	second := NewServer(dmn.New(), WithModelStore(dir)).Handler()
	one := decode[modelReleasesDTO](t, do(t, second, "GET", "/v1/releases/"+name, "", nil))
	if len(one.Releases) != 1 || one.Releases[0].Version != "1.0.0" {
		t.Fatalf("reloaded releases = %+v, want the 1.0.0 release", one)
	}
	rec := do(t, second, "POST", "/v1/models/"+name+"@1.0.0/evaluate", "application/json",
		publishBody(t, evaluateModelRequest{Decision: "Dish", Input: map[string]any{"Season": "Winter", "Guest Count": float64(8)}}))
	if rec.Code != http.StatusOK {
		t.Fatalf("evaluate after restart = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
}
