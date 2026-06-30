package service

import (
	"net/http"
	"os"
	"testing"
)

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

// TestCacheEvictionOverHTTP covers WP-35 end to end: with a one-model cache,
// uploading a second model evicts the first, so a GET for it is a 404 — until it
// is re-uploaded (hot reload), which recompiles and re-caches it.
func TestCacheEvictionOverHTTP(t *testing.T) {
	h := NewServer(nil, WithCacheSize(1)).Handler()

	dish := readFile(t, "../dmn/testdata/models/dish_15.dmn")
	discount := readFile(t, "../dmn/testdata/models/discount_14.dmn")

	dishID := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", dish)).ModelID
	// Uploading a second, different model evicts the first.
	do(t, h, "POST", "/v1/models", "application/xml", discount)

	if rec := do(t, h, "GET", "/v1/models/"+dishID, "", nil); rec.Code != http.StatusNotFound {
		t.Errorf("evicted model GET = %d, want 404", rec.Code)
	}

	// Re-uploading recompiles and re-caches it (same content hash).
	reID := decode[modelResponse](t, do(t, h, "POST", "/v1/models", "application/xml", dish)).ModelID
	if reID != dishID {
		t.Errorf("re-upload id = %q, want stable %q", reID, dishID)
	}
	if rec := do(t, h, "GET", "/v1/models/"+dishID, "", nil); rec.Code != http.StatusOK {
		t.Errorf("re-uploaded model GET = %d, want 200", rec.Code)
	}
}
