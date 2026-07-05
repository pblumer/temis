package model

// HitPolicy selects and aggregates matching rules of a decision table. WP-09
// implements U/A/F/R/C; the remaining policies arrive in WP-27. The DMN default
// when the attribute is absent is Unique.
type HitPolicy string

// Hit policies as they appear in DMN XML.
const (
	HitUnique      HitPolicy = "U"
	HitAny         HitPolicy = "A"
	HitPriority    HitPolicy = "P"
	HitFirst       HitPolicy = "F"
	HitRuleOrder   HitPolicy = "R"
	HitOutputOrder HitPolicy = "O"
	HitCollect     HitPolicy = "C"
)

// Aggregation is the function applied by the Collect hit policy. Empty means a
// plain list collect.
type Aggregation string

// Collect aggregations as they appear in DMN XML.
const (
	AggNone  Aggregation = ""
	AggSum   Aggregation = "SUM"
	AggCount Aggregation = "COUNT"
	AggMin   Aggregation = "MIN"
	AggMax   Aggregation = "MAX"
)

// DecisionTable is the boxed decision-table form of a decision's logic.
type DecisionTable struct {
	ID          string
	HitPolicy   HitPolicy
	Aggregation Aggregation `json:",omitempty"`
	Inputs      []*InputClause
	Outputs     []*OutputClause
	Rules       []*Rule
}

// InputClause is one input column. Expression is the FEEL text whose value is
// tested against each rule's matching input entry.
type InputClause struct {
	ID            string `json:",omitempty"`
	Label         string `json:",omitempty"`
	Expression    string
	TypeRef       string `json:",omitempty"`
	AllowedValues string `json:",omitempty"`
}

// OutputClause is one output column. DefaultOutput is the FEEL text of the
// column's <defaultOutputEntry>, applied when no rule matches (empty when none).
type OutputClause struct {
	ID            string `json:",omitempty"`
	Name          string `json:",omitempty"`
	Label         string `json:",omitempty"`
	TypeRef       string `json:",omitempty"`
	AllowedValues string `json:",omitempty"`
	DefaultOutput string `json:",omitempty"`
}

// Rule is one decision-table row. InputEntries are unary-test texts (aligned
// with Inputs); OutputEntries are FEEL result texts (aligned with Outputs).
type Rule struct {
	ID            string `json:",omitempty"`
	InputEntries  []string
	OutputEntries []string
	Annotations   []string `json:",omitempty"`
}
