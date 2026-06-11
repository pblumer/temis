package model_test

import (
	"encoding/json"
	"testing"

	"github.com/pblumer/temis/internal/model"
	dmnxml "github.com/pblumer/temis/internal/xml"
)

// ns15 is a valid DMN 1.5 namespace used to make constructed fixtures resolve to
// a known version.
const ns15 = "https://www.omg.org/spec/DMN/20230324/MODEL/"

func defWithNS(ns string) *dmnxml.Definitions {
	d := &dmnxml.Definitions{}
	d.XMLName.Local = "definitions"
	d.XMLName.Space = ns
	return d
}

func TestMapItemDefinitions(t *testing.T) {
	def := defWithNS(ns15)
	def.ItemDefs = []dmnxml.ItemDef{{
		ID:            "it_address",
		Name:          "Address",
		IsCollection:  true,
		TypeRef:       "string",
		AllowedValues: &dmnxml.Text{Value: ` "A","B" `},
		Components: []dmnxml.ItemDef{
			{ID: "it_city", Name: "City", TypeRef: "string"},
		},
	}}

	m, _, err := model.FromXML(def)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.ItemDefinitions) != 1 {
		t.Fatalf("got %d item definitions, want 1", len(m.ItemDefinitions))
	}
	it := m.ItemDefinitions[0]
	if !it.IsCollection || it.TypeRef != "string" || it.AllowedValues != `"A","B"` {
		t.Errorf("item definition mapped wrong: %+v", it)
	}
	if len(it.Components) != 1 || it.Components[0].Name != "City" {
		t.Errorf("nested component not mapped: %+v", it.Components)
	}
}

func TestMapBKMAndKnowledgeRequirement(t *testing.T) {
	def := defWithNS(ns15)
	def.BKMs = []dmnxml.BKM{{ID: "bkm_rate", Name: "Rate Table"}}
	def.Decisions = []dmnxml.Decision{{
		ID:   "d1",
		Name: "Premium",
		KnowledgeRequirts: []dmnxml.KnowledgeRequirt{
			{RequiredKnowledge: &dmnxml.Ref{Href: "#bkm_rate"}},
		},
		LiteralExpression: &dmnxml.LiteralExpression{Text: "Rate Table(age)"},
	}}

	m, _, err := model.FromXML(def)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.BKMs) != 1 || m.BKMs[0].Name != "Rate Table" {
		t.Errorf("BKM not mapped: %+v", m.BKMs)
	}
	got := m.Decisions[0].RequiredKnowledge
	if len(got) != 1 || got[0] != "bkm_rate" {
		t.Errorf("RequiredKnowledge = %v, want [bkm_rate]", got)
	}
}

func TestMapCollectWithAggregation(t *testing.T) {
	def := defWithNS(ns15)
	def.Decisions = []dmnxml.Decision{{
		ID:   "d1",
		Name: "Totals",
		DecisionTable: &dmnxml.DecisionTable{
			HitPolicy:   "COLLECT",
			Aggregation: "SUM",
			Outputs:     []dmnxml.Output{{Name: "Amount", AllowedValues: &dmnxml.Text{Value: "[0..100]"}}},
			Rules:       []dmnxml.Rule{{InputEntries: []string{"-"}, OutputEntries: []string{"10"}}},
		},
	}}

	m, _, err := model.FromXML(def)
	if err != nil {
		t.Fatal(err)
	}
	dt := m.Decisions[0].DecisionTable
	if dt.HitPolicy != model.HitCollect || dt.Aggregation != model.AggSum {
		t.Errorf("collect/aggregation = %q/%q, want C/SUM", dt.HitPolicy, dt.Aggregation)
	}
	if dt.Outputs[0].AllowedValues != "[0..100]" {
		t.Errorf("output allowed values = %q", dt.Outputs[0].AllowedValues)
	}
}

func TestDecisionWithoutLogicDiagnostic(t *testing.T) {
	def := defWithNS(ns15)
	def.Decisions = []dmnxml.Decision{{ID: "d_empty", Name: "Empty"}}

	_, diags, err := model.FromXML(def)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range diags {
		if d.DecisionID == "d_empty" && d.Severity == model.SeverityWarning {
			return
		}
	}
	t.Errorf("expected a warning for the decision without logic, got %+v", diags)
}

func TestUnknownNamespaceDiagnostic(t *testing.T) {
	def := defWithNS("http://example.com/not-dmn")
	m, diags, err := model.FromXML(def)
	if err != nil {
		t.Fatal(err)
	}
	if m.DMNVersion != model.VersionUnknown {
		t.Errorf("version = %s, want unknown", m.DMNVersion)
	}
	if len(diags) == 0 || diags[0].Severity != model.SeverityWarning {
		t.Errorf("expected an unrecognised-namespace warning, got %+v", diags)
	}
}

func TestHitPolicyVariants(t *testing.T) {
	cases := map[string]model.HitPolicy{
		"":             model.HitUnique,
		"UNIQUE":       model.HitUnique,
		"any":          model.HitAny,
		"PRIORITY":     model.HitPriority,
		"FIRST":        model.HitFirst,
		"RULE ORDER":   model.HitRuleOrder,
		"OUTPUT ORDER": model.HitOutputOrder,
		"COLLECT":      model.HitCollect,
		"X":            model.HitPolicy("X"),
	}
	for raw, want := range cases {
		def := defWithNS(ns15)
		def.Decisions = []dmnxml.Decision{{
			ID:            "d",
			DecisionTable: &dmnxml.DecisionTable{HitPolicy: raw},
		}}
		m, _, err := model.FromXML(def)
		if err != nil {
			t.Fatal(err)
		}
		if got := m.Decisions[0].DecisionTable.HitPolicy; got != want {
			t.Errorf("hitPolicy %q -> %q, want %q", raw, got, want)
		}
	}
}

func TestDetectVersionAllVariants(t *testing.T) {
	cases := map[string]model.Version{
		"https://www.omg.org/spec/DMN/20191111/MODEL/": model.Version13,
		"http://www.omg.org/spec/DMN/20211108/MODEL/":  model.Version14,
		"https://www.omg.org/spec/DMN/20230324/MODEL/": model.Version15,
	}
	for ns, want := range cases {
		m, _, err := model.FromXML(defWithNS(ns))
		if err != nil {
			t.Fatal(err)
		}
		if m.DMNVersion != want {
			t.Errorf("ns %s -> %s, want %s", ns, m.DMNVersion, want)
		}
	}
}

func TestSeverityAndVersionJSON(t *testing.T) {
	for s, want := range map[model.Severity]string{
		model.SeverityError:   `"error"`,
		model.SeverityWarning: `"warning"`,
		model.SeverityInfo:    `"info"`,
		model.Severity(99):    `"unknown"`,
	} {
		if got, _ := json.Marshal(s); string(got) != want {
			t.Errorf("severity %d JSON = %s, want %s", s, got, want)
		}
		if s.String() == "" {
			t.Errorf("severity %d String() empty", s)
		}
	}
	if model.VersionUnknown.String() != "unknown" {
		t.Errorf("VersionUnknown.String() = %q", model.VersionUnknown.String())
	}
}
