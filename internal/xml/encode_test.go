package xml_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	dmnxml "github.com/pblumer/temis/internal/xml"
)

func TestEncodePreservesNamespaceAndDMNDI(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "models", "dish_15.dmn"))
	if err != nil {
		t.Fatal(err)
	}
	def, err := dmnxml.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	out, err := dmnxml.Encode(def)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "20230324/MODEL/") {
		t.Errorf("encoded XML lost the DMN model namespace:\n%s", s)
	}
	if !strings.Contains(s, "DMNShape") || !strings.Contains(s, "waypoint") {
		t.Errorf("encoded XML lost DMNDI content:\n%s", s)
	}
}

func TestEncodeKeepsExplicitXmlns(t *testing.T) {
	def := &dmnxml.Definitions{Xmlns: "http://temis.test/explicit"}
	def.XMLName.Local = "definitions"
	out, err := dmnxml.Encode(def)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "http://temis.test/explicit") {
		t.Errorf("explicit Xmlns not preserved:\n%s", out)
	}
}

func TestDecodeTruncatedDMNDI(t *testing.T) {
	// The DMNDI element is opened but never closed: Raw must surface the error
	// rather than panic.
	const broken = `<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/">` +
		`<DMNDI><DMNDiagram>`
	if _, err := dmnxml.Decode([]byte(broken)); err == nil {
		t.Error("Decode(truncated DMNDI) = nil error, want error")
	}
}
