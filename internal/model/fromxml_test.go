package model_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/pblumer/temis/internal/model"
	dmnxml "github.com/pblumer/temis/internal/xml"
)

var update = flag.Bool("update", false, "update golden files")

// fixtures live with the xml package, which owns the .dmn samples.
func fixturePath(name string) string {
	return filepath.Join("..", "xml", "testdata", "models", name)
}

func loadModel(t *testing.T, name string) (*model.Definitions, []model.Diagnostic) {
	t.Helper()
	data, err := os.ReadFile(fixturePath(name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	def, err := dmnxml.Decode(data)
	if err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	m, diags, err := model.FromXML(def)
	if err != nil {
		t.Fatalf("FromXML %s: %v", name, err)
	}
	return m, diags
}

func TestFromXMLGolden(t *testing.T) {
	for _, name := range []string{"dish_15.dmn", "discount_14.dmn", "routing_13.dmn"} {
		t.Run(name, func(t *testing.T) {
			m, _ := loadModel(t, name)
			got, err := json.MarshalIndent(m, "", "  ")
			if err != nil {
				t.Fatalf("marshal model: %v", err)
			}
			got = append(got, '\n')

			golden := filepath.Join("testdata", "golden", name+".json")
			if *update {
				if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(golden, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}

			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("read golden (run with -update to create): %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("model mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
			}
		})
	}
}

func TestDetectVersion(t *testing.T) {
	for name, want := range map[string]model.Version{
		"dish_15.dmn":     model.Version15,
		"discount_14.dmn": model.Version14,
		"routing_13.dmn":  model.Version13,
	} {
		m, _ := loadModel(t, name)
		if m.DMNVersion != want {
			t.Errorf("%s: version = %s, want %s", name, m.DMNVersion, want)
		}
	}
}

func TestHasDMNDI(t *testing.T) {
	if m, _ := loadModel(t, "dish_15.dmn"); !m.HasDMNDI {
		t.Error("dish_15: HasDMNDI = false, want true")
	}
	if m, _ := loadModel(t, "discount_14.dmn"); m.HasDMNDI {
		t.Error("discount_14: HasDMNDI = true, want false")
	}
}

func TestUnknownElementDiagnostic(t *testing.T) {
	_, diags := loadModel(t, "routing_13.dmn")
	for _, d := range diags {
		if d.Severity == model.SeverityWarning && d.Source == "note" {
			return
		}
	}
	t.Errorf("expected a warning for the unknown <note> element, got %+v", diags)
}

func TestHitPolicyNormalisation(t *testing.T) {
	m, _ := loadModel(t, "dish_15.dmn")
	dt := m.Decisions[0].DecisionTable
	if dt == nil {
		t.Fatal("dish decision has no decision table")
	}
	if dt.HitPolicy != model.HitUnique {
		t.Errorf("hit policy = %q, want %q (normalised from UNIQUE)", dt.HitPolicy, model.HitUnique)
	}
}

func TestRequirementsResolved(t *testing.T) {
	m, _ := loadModel(t, "routing_13.dmn")
	var routing *model.Decision
	for _, d := range m.Decisions {
		if d.Name == "Routing" {
			routing = d
		}
	}
	if routing == nil {
		t.Fatal("Routing decision not found")
	}
	if len(routing.RequiredDecisions) != 1 || routing.RequiredDecisions[0] != "id_eligibility" {
		t.Errorf("Routing.RequiredDecisions = %v, want [id_eligibility]", routing.RequiredDecisions)
	}
}

func TestFromXMLNil(t *testing.T) {
	if _, _, err := model.FromXML(nil); err == nil {
		t.Error("FromXML(nil) = nil error, want error")
	}
}
