package model

import (
	"errors"
	"strconv"
	"strings"

	dmnxml "github.com/pblumer/temis/internal/xml"
)

// errNilDefinitions is returned when FromXML is given a nil input.
var errNilDefinitions = errors.New("model: nil definitions")

// DMN namespace date segments used to detect the authored version. Matching is
// scheme-agnostic so both http:// and https:// forms are recognised (ADR-0002).
const (
	nsDate13 = "/DMN/20191111/"
	nsDate14 = "/DMN/20211108/"
	nsDate15 = "/DMN/20230324/"
)

// FromXML maps decoded DMN XML into the version-neutral model. It never fails on
// content it does not understand: unknown elements and unrecognised versions are
// returned as diagnostics. A nil input yields an error.
func FromXML(def *dmnxml.Definitions) (*Definitions, []Diagnostic, error) {
	if def == nil {
		return nil, nil, errNilDefinitions
	}

	var diags []Diagnostic

	version, ok := detectVersion(def.XMLName.Space)
	if !ok {
		diags = append(diags, Diagnostic{
			Severity: SeverityWarning,
			Message:  "unrecognised DMN namespace " + strconv.Quote(def.XMLName.Space) + "; decoded leniently",
			Source:   "definitions",
		})
	}

	m := &Definitions{
		ID:         def.ID,
		Name:       def.Name,
		Namespace:  def.Namespace,
		DMNVersion: version,
		HasDMNDI:   def.DMNDI != nil,
	}

	for _, it := range def.ItemDefs {
		m.ItemDefinitions = append(m.ItemDefinitions, mapItemDef(it))
	}
	for _, in := range def.InputData {
		m.InputData = append(m.InputData, &InputData{ID: in.ID, Name: in.Name, TypeRef: variableTypeRef(in.Variable)})
	}
	for _, b := range def.BKMs {
		m.BKMs = append(m.BKMs, &BKM{ID: b.ID, Name: b.Name})
	}
	for _, d := range def.Decisions {
		dec, dd := mapDecision(d)
		m.Decisions = append(m.Decisions, dec)
		diags = append(diags, dd...)
	}

	for _, u := range def.Unknown {
		diags = append(diags, Diagnostic{
			Severity: SeverityWarning,
			Message:  "unknown element ignored",
			Source:   u.XMLName.Local,
		})
	}

	return m, diags, nil
}

func detectVersion(nsURI string) (Version, bool) {
	switch {
	case strings.Contains(nsURI, nsDate13):
		return Version13, true
	case strings.Contains(nsURI, nsDate14):
		return Version14, true
	case strings.Contains(nsURI, nsDate15):
		return Version15, true
	default:
		return VersionUnknown, false
	}
}

func mapDecision(d dmnxml.Decision) (*Decision, []Diagnostic) {
	dec := &Decision{
		ID:              d.ID,
		Name:            d.Name,
		VariableTypeRef: variableTypeRef(d.Variable),
	}

	for _, ir := range d.InformationRequirts {
		if ref := localRef(hrefOf(ir.RequiredDecision)); ref != "" {
			dec.RequiredDecisions = append(dec.RequiredDecisions, ref)
		}
		if ref := localRef(hrefOf(ir.RequiredInput)); ref != "" {
			dec.RequiredInputs = append(dec.RequiredInputs, ref)
		}
	}
	for _, kr := range d.KnowledgeRequirts {
		if ref := localRef(hrefOf(kr.RequiredKnowledge)); ref != "" {
			dec.RequiredKnowledge = append(dec.RequiredKnowledge, ref)
		}
	}

	var diags []Diagnostic
	switch {
	case d.LiteralExpression != nil:
		le := d.LiteralExpression
		dec.LiteralExpression = &LiteralExpression{
			ID:                 le.ID,
			TypeRef:            le.TypeRef,
			ExpressionLanguage: le.ExprLang,
			Text:               strings.TrimSpace(le.Text),
		}
	case d.DecisionTable != nil:
		dec.DecisionTable = mapTable(d.DecisionTable)
	default:
		diags = append(diags, Diagnostic{
			Severity:   SeverityWarning,
			Message:    "decision has no literal expression or decision table",
			Source:     "decision",
			DecisionID: d.ID,
		})
	}
	return dec, diags
}

func mapTable(t *dmnxml.DecisionTable) *DecisionTable {
	hp := normalizeHitPolicy(t.HitPolicy)
	dt := &DecisionTable{
		ID:          t.ID,
		HitPolicy:   hp,
		Aggregation: Aggregation(t.Aggregation),
	}
	for _, in := range t.Inputs {
		dt.Inputs = append(dt.Inputs, &InputClause{
			ID:            in.ID,
			Label:         in.Label,
			Expression:    strings.TrimSpace(in.InputExpression.Text),
			TypeRef:       strings.TrimSpace(in.InputExpression.TypeRef),
			AllowedValues: textValue(in.AllowedValues),
		})
	}
	for _, out := range t.Outputs {
		dt.Outputs = append(dt.Outputs, &OutputClause{
			ID:            out.ID,
			Name:          out.Name,
			Label:         out.Label,
			TypeRef:       out.TypeRef,
			AllowedValues: textValue(out.AllowedValues),
		})
	}
	for _, r := range t.Rules {
		dt.Rules = append(dt.Rules, &Rule{
			ID:            r.ID,
			InputEntries:  trimAll(r.InputEntries),
			OutputEntries: trimAll(r.OutputEntries),
			Annotations:   trimAll(r.Annotations),
		})
	}
	return dt
}

func mapItemDef(it dmnxml.ItemDef) *ItemDefinition {
	id := &ItemDefinition{
		ID:            it.ID,
		Name:          it.Name,
		TypeRef:       strings.TrimSpace(it.TypeRef),
		IsCollection:  it.IsCollection,
		AllowedValues: textValue(it.AllowedValues),
	}
	for _, c := range it.Components {
		id.Components = append(id.Components, mapItemDef(c))
	}
	return id
}

// normalizeHitPolicy maps the DMN XML hit-policy attribute, which uses full
// words ("UNIQUE", "RULE ORDER", ...) and defaults to UNIQUE when absent, to the
// canonical single-letter form used throughout the engine. The single-letter
// form is also accepted so hand-written models stay lenient.
func normalizeHitPolicy(raw string) HitPolicy {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "", "UNIQUE", "U":
		return HitUnique
	case "ANY", "A":
		return HitAny
	case "PRIORITY", "P":
		return HitPriority
	case "FIRST", "F":
		return HitFirst
	case "RULE ORDER", "RULE_ORDER", "R":
		return HitRuleOrder
	case "OUTPUT ORDER", "OUTPUT_ORDER", "O":
		return HitOutputOrder
	case "COLLECT", "C":
		return HitCollect
	default:
		return HitPolicy(strings.TrimSpace(raw))
	}
}

// localRef strips the leading fragment of an href, yielding the referenced
// element's local identifier (e.g. "#dec_1" or "model.dmn#dec_1" -> "dec_1").
func localRef(href string) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}
	if i := strings.LastIndex(href, "#"); i >= 0 {
		return href[i+1:]
	}
	return href
}

func hrefOf(r *dmnxml.Ref) string {
	if r == nil {
		return ""
	}
	return r.Href
}

func variableTypeRef(v *dmnxml.Variable) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(v.TypeRef)
}

func textValue(t *dmnxml.Text) string {
	if t == nil {
		return ""
	}
	return strings.TrimSpace(t.Value)
}

func trimAll(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strings.TrimSpace(s)
	}
	return out
}
