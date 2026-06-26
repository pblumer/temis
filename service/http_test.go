package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

const dishModelPath = "../dmn/testdata/models/dish_15.dmn"

func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	return NewServer(nil).Handler()
}

func dishXML(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(dishModelPath)
	if err != nil {
		t.Fatalf("read dish model: %v", err)
	}
	return b
}

func do(t *testing.T, h http.Handler, method, path, contentType string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode response %q: %v", rec.Body.String(), err)
	}
	return v
}

func TestCreateModelAndIndex(t *testing.T) {
	h := newTestServer(t)
	rec := do(t, h, "POST", "/v1/models", "application/xml", dishXML(t))
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /v1/models = %d, want 201 (body %s)", rec.Code, rec.Body)
	}
	resp := decode[modelResponse](t, rec)
	if !strings.HasPrefix(resp.ModelID, "sha256:") {
		t.Errorf("modelId = %q, want sha256: prefix", resp.ModelID)
	}
	if !contains(resp.Decisions, "Dish") {
		t.Errorf("decisions = %v, want to contain Dish", resp.Decisions)
	}

	// GET the same model back.
	rec = do(t, h, "GET", "/v1/models/"+resp.ModelID, "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET model = %d, want 200", rec.Code)
	}
	got := decode[modelResponse](t, rec)
	if got.ModelID != resp.ModelID {
		t.Errorf("GET modelId = %q, want %q", got.ModelID, resp.ModelID)
	}
}

func TestModelUploadIsIdempotent(t *testing.T) {
	h := newTestServer(t)
	xml := dishXML(t)
	id1 := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", xml)).ModelID
	id2 := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", xml)).ModelID
	if id1 != id2 {
		t.Errorf("re-upload gave different ids: %q vs %q", id1, id2)
	}
}

func TestEvaluateModel(t *testing.T) {
	h := newTestServer(t)
	id := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", dishXML(t))).ModelID

	body := mustJSON(t, evaluateModelRequest{
		Decision: "Dish",
		Input:    map[string]any{"Season": "Winter", "Guest Count": 4},
	})
	rec := do(t, h, "POST", "/v1/models/"+id+"/evaluate", "application/json", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("evaluate = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
	resp := decode[evaluateResponse](t, rec)
	if resp.Outputs["Dish"] != "Roastbeef" {
		t.Errorf("Dish = %v, want Roastbeef", resp.Outputs["Dish"])
	}
}

func TestEvaluateStateless(t *testing.T) {
	h := newTestServer(t)
	body := mustJSON(t, evaluateStatelessRequest{
		XML:      string(dishXML(t)),
		Decision: "Dish",
		Input:    map[string]any{"Season": "Fall", "Guest Count": 4},
	})
	rec := do(t, h, "POST", "/v1/evaluate", "application/json", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("stateless evaluate = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
	resp := decode[evaluateResponse](t, rec)
	if resp.Outputs["Dish"] != "Spareribs" {
		t.Errorf("Dish = %v, want Spareribs", resp.Outputs["Dish"])
	}
}

func TestErrorResponses(t *testing.T) {
	h := newTestServer(t)
	id := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", dishXML(t))).ModelID

	cases := []struct {
		name        string
		method, url string
		ct          string
		body        []byte
		wantStatus  int
		wantCode    string
	}{
		{"malformed xml", "POST", "/v1/models", "application/xml", []byte("<not-dmn>"), http.StatusBadRequest, "MALFORMED_XML"},
		{"empty body", "POST", "/v1/models", "application/xml", nil, http.StatusBadRequest, "INVALID_REQUEST"},
		{"unknown model", "GET", "/v1/models/sha256:deadbeef", "", nil, http.StatusNotFound, "MODEL_NOT_FOUND"},
		{"unknown decision", "POST", "/v1/models/" + id + "/evaluate", "application/json",
			mustJSON(t, evaluateModelRequest{Decision: "Nope"}), http.StatusNotFound, "DECISION_NOT_FOUND"},
		{"bad json", "POST", "/v1/models/" + id + "/evaluate", "application/json",
			[]byte("{not json"), http.StatusBadRequest, "INVALID_REQUEST"},
		{"missing decision", "POST", "/v1/models/" + id + "/evaluate", "application/json",
			mustJSON(t, evaluateModelRequest{}), http.StatusBadRequest, "INVALID_REQUEST"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := do(t, h, c.method, c.url, c.ct, c.body)
			if rec.Code != c.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, c.wantStatus, rec.Body)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
				t.Errorf("content-type = %q, want application/problem+json", ct)
			}
			p := decode[problem](t, rec)
			if p.Code != c.wantCode {
				t.Errorf("code = %q, want %q", p.Code, c.wantCode)
			}
		})
	}
}

func TestHealth(t *testing.T) {
	h := newTestServer(t)
	for _, path := range []string{"/healthz", "/readyz"} {
		rec := do(t, h, "GET", path, "", nil)
		if rec.Code != http.StatusOK {
			t.Errorf("%s = %d, want 200", path, rec.Code)
		}
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}
