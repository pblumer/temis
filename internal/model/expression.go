package model

// LiteralExpression is a FEEL expression given as text. The text is stored
// verbatim and is not parsed until the FEEL compiler (WP-03ff).
type LiteralExpression struct {
	ID                 string `json:",omitempty"`
	TypeRef            string `json:",omitempty"`
	ExpressionLanguage string `json:",omitempty"`
	Text               string
}

// ItemDefinition is a (possibly structured) DMN type definition. Components
// model nested item components; AllowedValues holds the raw FEEL constraint
// text when present. Full type-system binding follows in WP-31.
type ItemDefinition struct {
	ID            string `json:",omitempty"`
	Name          string
	TypeRef       string            `json:",omitempty"`
	IsCollection  bool              `json:",omitempty"`
	AllowedValues string            `json:",omitempty"`
	Components    []*ItemDefinition `json:",omitempty"`
}
