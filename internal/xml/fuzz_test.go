package xml_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	dmnxml "github.com/pblumer/temis/internal/xml"
)

// FuzzDecode asserts the invariant from docs/50-testing-strategy.md §3: no input
// makes the decoder (or a subsequent encode) panic. Seeded from the fixtures.
func FuzzDecode(f *testing.F) {
	entries, err := os.ReadDir(filepath.Join("testdata", "models"))
	if err != nil {
		f.Fatalf("read fixtures dir: %v", err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".dmn") {
			continue
		}
		if data, err := os.ReadFile(filepath.Join("testdata", "models", e.Name())); err == nil {
			f.Add(data)
		}
	}
	f.Fuzz(func(_ *testing.T, data []byte) {
		def, err := dmnxml.Decode(data)
		if err != nil {
			return
		}
		_, _ = dmnxml.Encode(def)
	})
}
