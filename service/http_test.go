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

func TestDocsAndSpec(t *testing.T) {
	h := newTestServer(t)

	rec := do(t, h, "GET", "/docs", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /docs = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("/docs content-type = %q, want text/html", ct)
	}
	if !strings.Contains(rec.Body.String(), "swagger-ui") {
		t.Errorf("/docs body does not look like a Swagger UI page")
	}

	rec = do(t, h, "GET", "/openapi.yaml", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /openapi.yaml = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "openapi:") {
		t.Errorf("/openapi.yaml body is not the OpenAPI document")
	}
}

func TestPlaygroundUI(t *testing.T) {
	h := newTestServer(t)

	for _, path := range []string{"/", "/ui"} {
		rec := do(t, h, "GET", path, "", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200", path, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Errorf("%s content-type = %q, want text/html", path, ct)
		}
		if !strings.Contains(rec.Body.String(), "DMN Playground") {
			t.Errorf("%s body does not look like the playground page", path)
		}
	}

	// The root pattern must not swallow unknown paths into a 200.
	if rec := do(t, h, "GET", "/does-not-exist", "", nil); rec.Code != http.StatusNotFound {
		t.Errorf("GET /does-not-exist = %d, want 404", rec.Code)
	}
}

func TestTokenAuth(t *testing.T) {
	const token = "s3cr3t-token"
	h := NewServer(nil, WithToken(token)).Handler()
	xml := dishXML(t)

	// No token → 401 with the stable code, on a gated endpoint.
	rec := do(t, h, "POST", "/v1/models", "application/xml", xml)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated POST = %d, want 401 (body %s)", rec.Code, rec.Body)
	}
	if p := decode[problem](t, rec); p.Code != "UNAUTHORIZED" {
		t.Errorf("code = %q, want UNAUTHORIZED", p.Code)
	}
	if wa := rec.Header().Get("WWW-Authenticate"); !strings.HasPrefix(wa, "Bearer") {
		t.Errorf("WWW-Authenticate = %q, want Bearer challenge", wa)
	}

	// Wrong token → 401.
	if rec := doAuth(t, h, "POST", "/v1/models", "application/xml", xml, "nope"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token POST = %d, want 401", rec.Code)
	}

	// Correct token → 201.
	rec = doAuth(t, h, "POST", "/v1/models", "application/xml", xml, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("authenticated POST = %d, want 201 (body %s)", rec.Code, rec.Body)
	}

	// Discovery and probes stay public even with a token configured.
	for _, path := range []string{"/", "/ui", "/docs", "/openapi.yaml", "/healthz"} {
		if rec := do(t, h, "GET", path, "", nil); rec.Code != http.StatusOK {
			t.Errorf("GET %s with token configured = %d, want 200", path, rec.Code)
		}
	}
}

func TestNoTokenLeavesAPIOpen(t *testing.T) {
	// An Authorization header on an open server is ignored, not rejected.
	h := newTestServer(t)
	rec := doAuth(t, h, "POST", "/v1/models", "application/xml", dishXML(t), "ignored")
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST on open server = %d, want 201", rec.Code)
	}
}

func doAuth(t *testing.T, h http.Handler, method, path, contentType string, body []byte, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
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
