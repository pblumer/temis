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

// TableEdit is the editable payload for a decision table: its rule rows. The
// columns and hit policy are not changed by this edit (they keep the original
// table's structure), so the decision logic stays consistent — only the rows are
// rewritten (ADR-0016, table editing).
type TableEdit struct {
	Rules []TableRule `json:"rules"`
}

// ApplyTableEdit rewrites the rule rows of a decision's decision table in a DMN
// document and returns the updated XML. It patches the existing document, so the
// table's columns, hit policy, the DMNDI and every other decision are preserved.
// An empty input entry is stored as "-" (the DMN "any" match). It errors when the
// decision has no decision-table logic.
func ApplyTableEdit(src []byte, decisionID string, edit TableEdit) ([]byte, error) {
	def, err := dmnxml.Decode(src)
	if err != nil {
		return nil, err
	}
	rules := make([]dmnxml.Rule, len(edit.Rules))
	for i, r := range edit.Rules {
		rules[i] = dmnxml.Rule{
			InputEntries:  normalizeInputEntries(r.InputEntries),
			OutputEntries: trimEntries(r.OutputEntries),
			Annotations:   dropTrailingEmpty(trimEntries(r.Annotations)),
		}
	}
	if !def.SetDecisionTableRules(decisionID, rules) {
		return nil, fmt.Errorf("dmn: decision %q has no decision table", decisionID)
	}
	return dmnxml.Encode(def)
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
