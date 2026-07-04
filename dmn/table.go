package dmn

import (
	"fmt"
	"strings"

	"github.com/pblumer/temis/internal/model"
	dmnxml "github.com/pblumer/temis/internal/xml"
)

// TableView is a decision's decision-table logic, flattened for display by the
// modeler (ADR-0016): the static table — hit policy, input/output columns and
// rule rows — independent of any evaluation. It carries JSON tags as part of that
// wire contract.
type TableView struct {
	DecisionID  string        `json:"decisionId"`
	Name        string        `json:"name"`
	HitPolicy   string        `json:"hitPolicy"`
	Aggregation string        `json:"aggregation,omitempty"`
	Inputs      []TableInput  `json:"inputs"`
	Outputs     []TableOutput `json:"outputs"`
	Rules       []TableRule   `json:"rules"`
}

// TableInput is one input column: the FEEL expression whose value each rule tests,
// with an optional label and declared type.
type TableInput struct {
	Label      string `json:"label,omitempty"`
	Expression string `json:"expression"`
	TypeRef    string `json:"typeRef,omitempty"`
}

// TableOutput is one output column.
type TableOutput struct {
	Name    string `json:"name,omitempty"`
	Label   string `json:"label,omitempty"`
	TypeRef string `json:"typeRef,omitempty"`
}

// TableRule is one rule row: the unary-test input entries (aligned with Inputs),
// the result output entries (aligned with Outputs) and any annotations.
type TableRule struct {
	InputEntries  []string `json:"inputEntries"`
	OutputEntries []string `json:"outputEntries"`
	Annotations   []string `json:"annotations,omitempty"`
}

// DecisionTable returns the decision's decision-table view. ok is false when no
// such decision exists or its logic is not a decision table (e.g. a literal
// expression or context), so the modeler can fall back gracefully.
func (d *Definitions) DecisionTable(idOrName string) (TableView, bool) {
	dec := d.decisionModel(idOrName)
	if dec == nil || dec.DecisionTable == nil {
		return TableView{}, false
	}
	dt := dec.DecisionTable
	v := TableView{
		DecisionID:  dec.ID,
		Name:        dec.Name,
		HitPolicy:   string(dt.HitPolicy),
		Aggregation: string(dt.Aggregation),
	}
	for _, in := range dt.Inputs {
		v.Inputs = append(v.Inputs, TableInput{Label: in.Label, Expression: in.Expression, TypeRef: canonicalType(in.TypeRef)})
	}
	for _, out := range dt.Outputs {
		v.Outputs = append(v.Outputs, TableOutput{Name: out.Name, Label: out.Label, TypeRef: canonicalType(out.TypeRef)})
	}
	for _, r := range dt.Rules {
		v.Rules = append(v.Rules, TableRule{InputEntries: r.InputEntries, OutputEntries: r.OutputEntries, Annotations: r.Annotations})
	}
	return v, true
}

// TableEdit is the editable payload for a decision table. Rules are always
// rewritten. HitPolicy, when non-empty, sets the policy (Aggregation applies to
// Collect). Inputs/Outputs replace the columns only when ReplaceColumns is set —
// so a rules-only edit (ReplaceColumns false) keeps the existing columns, while
// the modeler's full editor sends the columns and sets the flag (ADR-0016).
type TableEdit struct {
	HitPolicy      string        `json:"hitPolicy,omitempty"`
	Aggregation    string        `json:"aggregation,omitempty"`
	Inputs         []TableInput  `json:"inputs,omitempty"`
	Outputs        []TableOutput `json:"outputs,omitempty"`
	Rules          []TableRule   `json:"rules"`
	ReplaceColumns bool          `json:"replaceColumns,omitempty"`
}

// ApplyTableEdit rewrites the rule rows of a decision's decision table in a DMN
// document and returns the updated XML. It patches the existing document, so the
// table's columns, hit policy, the DMNDI and every other decision are preserved.
// An empty input entry is stored as "-" (the DMN "any" match). It errors when the
// decision has no decision-table logic.
func ApplyTableEdit(src []byte, decisionID string, edit TableEdit) ([]byte, error) {
	return applyTableEditAt(src, "decision", decisionID, nil, edit)
}

// applyTableEditAt is ApplyTableEdit generalised over the logic location (a
// decision or BKM body, and a nested path within it, per exprSlotAt): it patches
// the decision table found at anchor+steps.
func applyTableEditAt(src []byte, anchorKind, anchorID string, steps []dmnxml.Step, edit TableEdit) ([]byte, error) {
	def, err := dmnxml.Decode(src)
	if err != nil {
		return nil, err
	}
	inputs, outputs, rules := buildTableParts(edit)
	if !def.UpdateTableAt(anchorKind, anchorID, steps, hitPolicyXML(edit.HitPolicy), aggregationFor(edit), inputs, outputs, rules, edit.ReplaceColumns) {
		return nil, fmt.Errorf("dmn: %s %q has no decision table at that location", anchorKind, anchorID)
	}
	return dmnxml.Encode(def)
}

// buildTableParts turns a TableEdit into the XML input/output columns (only when
// ReplaceColumns is set) and the rule rows, aligning each rule's entries with the
// column counts so an added/removed column leaves a consistent table.
func buildTableParts(edit TableEdit) ([]dmnxml.Input, []dmnxml.Output, []dmnxml.Rule) {
	var inputs []dmnxml.Input
	var outputs []dmnxml.Output
	if edit.ReplaceColumns {
		for _, in := range edit.Inputs {
			inputs = append(inputs, dmnxml.Input{
				Label:           strings.TrimSpace(in.Label),
				InputExpression: dmnxml.InputExpression{Text: strings.TrimSpace(in.Expression), TypeRef: strings.TrimSpace(in.TypeRef)},
			})
		}
		for _, out := range edit.Outputs {
			outputs = append(outputs, dmnxml.Output{
				Name:    strings.TrimSpace(out.Name),
				Label:   strings.TrimSpace(out.Label),
				TypeRef: strings.TrimSpace(out.TypeRef),
			})
		}
	}
	nIn, nOut := -1, -1
	if edit.ReplaceColumns {
		nIn, nOut = len(inputs), len(outputs)
	}
	rules := make([]dmnxml.Rule, len(edit.Rules))
	for i, r := range edit.Rules {
		rules[i] = dmnxml.Rule{
			InputEntries:  fit(normalizeInputEntries(r.InputEntries), nIn, "-"),
			OutputEntries: fit(trimEntries(r.OutputEntries), nOut, ""),
			Annotations:   dropTrailingEmpty(trimEntries(r.Annotations)),
		}
	}
	return inputs, outputs, rules
}

// fit pads or truncates entries to n elements (filling with pad); n < 0 leaves
// the slice unchanged.
func fit(entries []string, n int, pad string) []string {
	if n < 0 {
		return entries
	}
	out := make([]string, n)
	for i := range out {
		if i < len(entries) {
			out[i] = entries[i]
		} else {
			out[i] = pad
		}
	}
	return out
}

// hitPolicyXML maps the single-letter (or word) hit policy to the DMN XML word,
// or "" to leave the policy unchanged.
func hitPolicyXML(p string) string {
	switch strings.ToUpper(strings.TrimSpace(p)) {
	case "U", "UNIQUE":
		return "UNIQUE"
	case "A", "ANY":
		return "ANY"
	case "P", "PRIORITY":
		return "PRIORITY"
	case "F", "FIRST":
		return "FIRST"
	case "R", "RULE ORDER", "RULE_ORDER":
		return "RULE ORDER"
	case "O", "OUTPUT ORDER", "OUTPUT_ORDER":
		return "OUTPUT ORDER"
	case "C", "COLLECT":
		return "COLLECT"
	default:
		return ""
	}
}

// aggregationFor returns the Collect aggregation, only when the policy is Collect
// (it is meaningless and invalid for the other policies).
func aggregationFor(edit TableEdit) string {
	if hitPolicyXML(edit.HitPolicy) != "COLLECT" {
		return ""
	}
	switch strings.ToUpper(strings.TrimSpace(edit.Aggregation)) {
	case "SUM", "COUNT", "MIN", "MAX":
		return strings.ToUpper(strings.TrimSpace(edit.Aggregation))
	default:
		return ""
	}
}

// normalizeInputEntries trims each input cell and maps an empty cell to "-", the
// DMN unary test that matches any value.
func normalizeInputEntries(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			s = "-"
		}
		out[i] = s
	}
	return out
}

func trimEntries(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strings.TrimSpace(s)
	}
	return out
}

// dropTrailingEmpty removes empty strings from the tail, so an all-empty
// annotation slice round-trips as no annotations rather than empty elements.
func dropTrailingEmpty(in []string) []string {
	end := len(in)
	for end > 0 && in[end-1] == "" {
		end--
	}
	return in[:end]
}

// CreateDecisionTable gives a logic-less decision a fresh decision table and
// returns the updated XML. The table's input columns are derived from the
// decision's information requirements (so a decision wired into the DRG gets its
// inputs for free), with a single output named after the decision; it starts
// with no rules, to be filled via the table editor. It errors when the decision
// is unknown or already has logic.
func CreateDecisionTable(src []byte, decisionID string) ([]byte, error) {
	def, err := dmnxml.Decode(src)
	if err != nil {
		return nil, err
	}
	if !def.CreateDecisionTable(decisionID) {
		return nil, fmt.Errorf("dmn: cannot create a table for decision %q (unknown or already has logic)", decisionID)
	}
	return dmnxml.Encode(def)
}

// decisionModel resolves a decision by id or name to the underlying model
// element, or nil when none matches.
func (d *Definitions) decisionModel(idOrName string) *model.Decision {
	for _, dec := range d.model.Decisions {
		if dec.ID == idOrName || dec.Name == idOrName {
			return dec
		}
	}
	return nil
}
