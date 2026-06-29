package dmn

import "github.com/pblumer/temis/internal/model"

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
