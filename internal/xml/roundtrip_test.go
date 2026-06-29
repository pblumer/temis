package xml_test

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/pblumer/temis/internal/model"
	dmnxml "github.com/pblumer/temis/internal/xml"
)

func fixtures(t *testing.T) []string {
	t.Helper()
	dir := filepath.Join("testdata", "models")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read fixtures dir: %v", err)
	}
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".dmn") {
			names = append(names, filepath.Join(dir, e.Name()))
		}
	}
	return names
}

// TestRoundTrip decodes, re-encodes and decodes again, asserting that the
// semantic model is unchanged and that the DMNDI subtree is preserved.
func TestRoundTrip(t *testing.T) {
	for _, path := range fixtures(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			def1, err := dmnxml.Decode(data)
			if err != nil {
				t.Fatalf("decode #1: %v", err)
			}
			encoded, err := dmnxml.Encode(def1)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			def2, err := dmnxml.Decode(encoded)
			if err != nil {
				t.Fatalf("decode #2 of re-encoded XML:\n%s\nerror: %v", encoded, err)
			}

			m1, _, err := model.FromXML(def1)
			if err != nil {
				t.Fatal(err)
			}
			m2, _, err := model.FromXML(def2)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(m1, m2) {
				t.Errorf("semantic model changed across round-trip\nbefore: %+v\nafter:  %+v", m1, m2)
			}

			if got, want := summarizeDMNDI(def2.DMNDI), summarizeDMNDI(def1.DMNDI); got != want {
				t.Errorf("DMNDI not preserved across round-trip\nbefore: %s\nafter:  %s", want, got)
			}
		})
	}
}

// summarizeDMNDI projects a captured DMNDI token stream to a prefix-independent
// string: element local names plus their meaningful (non-xmlns) attributes and
// trimmed character data. This survives namespace-prefix rewrites while still
// detecting any loss of diagram content.
func summarizeDMNDI(r *dmnxml.Raw) string {
	if r == nil {
		return "<none>"
	}
	var b strings.Builder
	for _, tok := range r.Tokens {
		switch t := tok.(type) {
		case xml.StartElement:
			b.WriteString("<")
			b.WriteString(t.Name.Local)
			var attrs []string
			for _, a := range t.Attr {
				if a.Name.Local == "xmlns" || a.Name.Space == "xmlns" ||
					a.Name.Space == "http://www.w3.org/2000/xmlns/" {
					continue
				}
				attrs = append(attrs, a.Name.Local+"="+a.Value)
			}
			sort.Strings(attrs)
			for _, a := range attrs {
				b.WriteString(" ")
				b.WriteString(a)
			}
			b.WriteString(">")
		case xml.EndElement:
			b.WriteString("</")
			b.WriteString(t.Name.Local)
			b.WriteString(">")
		case xml.CharData:
			if s := strings.TrimSpace(string(t)); s != "" {
				b.WriteString(s)
			}
		}
	}
	return b.String()
}

// TestRoundTripXMLFidelity guards byte-structural round-trip fidelity, not just
// semantic-model equality: some encode bugs (boxed <list>/<relation> emitting Go
// field names; decision-table rules collapsing multiple <inputEntry> into one)
// produce structurally invalid DMN that nonetheless re-decodes to the same
// model — so TestRoundTrip alone misses them. A forked dmn-moddle editor
// (ADR-0016) consumes this XML directly, so element structure must be preserved.
func TestRoundTripXMLFidelity(t *testing.T) {
	for _, path := range fixtures(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			def, err := dmnxml.Decode(data)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			enc, err := dmnxml.Encode(def)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			before, after := elementCounts(t, data), elementCounts(t, enc)
			if !reflect.DeepEqual(before, after) {
				t.Errorf("element histogram changed across encode\n before: %v\n after:  %v", before, after)
			}
		})
	}
}

// elementCounts tallies start-element local names, ignoring prefixes, attributes
// and whitespace — robust to namespace-prefix rewrites and self-closing vs
// open/close differences, while detecting any added/dropped/renamed element.
func elementCounts(t *testing.T, b []byte) map[string]int {
	t.Helper()
	counts := map[string]int{}
	dec := xml.NewDecoder(strings.NewReader(string(b)))
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok {
			counts[se.Name.Local]++
		}
	}
	return counts
}

// TestNamespaceTolerance confirms that one decoder reads every DMN version.
func TestNamespaceTolerance(t *testing.T) {
	for _, path := range fixtures(t) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		def, err := dmnxml.Decode(data)
		if err != nil {
			t.Errorf("%s: decode failed: %v", filepath.Base(path), err)
			continue
		}
		if def.XMLName.Local != "definitions" {
			t.Errorf("%s: root = %q, want definitions", filepath.Base(path), def.XMLName.Local)
		}
	}
}

func TestDecodeMalformed(t *testing.T) {
	if _, err := dmnxml.Decode([]byte("<definitions><decision>")); err == nil {
		t.Error("Decode(malformed) = nil error, want error")
	}
}
