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
			Code:     "UNKNOWN_NAMESPACE",
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

	if di := dmnxml.ParseDI(def.DMNDI); di != nil {
		shapes := make(map[string]Bounds, len(di.Shapes))
		for _, s := range di.Shapes {
			shapes[s.Ref] = Bounds{X: s.X, Y: s.Y, Width: s.Width, Height: s.Height}
		}
		m.Diagram = &Diagram{Shapes: shapes}
	}

	for _, it := range def.ItemDefs {
		m.ItemDefinitions = append(m.ItemDefinitions, mapItemDef(it))
	}
	for _, in := range def.InputData {
		m.InputData = append(m.InputData, &InputData{ID: in.ID, Name: in.Name, TypeRef: variableTypeRef(in.Variable)})
	}
	for _, b := range def.BKMs {
		m.BKMs = append(m.BKMs, mapBKM(b))
	}
	for _, d := range def.Decisions {
		dec, dd := mapDecision(d)
		m.Decisions = append(m.Decisions, dec)
		diags = append(diags, dd...)
	}
	for _, ds := range def.Services {
		m.Services = append(m.Services, mapService(ds))
	}

	for _, u := range def.Unknown {
		diags = append(diags, Diagnostic{
			Severity: SeverityWarning,
			Code:     "UNKNOWN_ELEMENT",
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
		VariableName:    variableName(d.Variable),
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
	switch logic := mapExpression(d.Expression).(type) {
	case nil:
		diags = append(diags, Diagnostic{
			Severity:   SeverityWarning,
			Code:       "DECISION_NO_LOGIC",
			Message:    "decision has no executable logic",
			Source:     "decision",
			DecisionID: d.ID,
		})
	case *LiteralExpression:
		dec.LiteralExpression = logic
	case *DecisionTable:
		dec.DecisionTable = logic
	case *ContextExpr:
		dec.Context = logic
	case *Invocation:
		dec.Invocation = logic
	case *FunctionDef:
		dec.FunctionDef = logic
	case *ListExpr:
		dec.List = logic
	case *RelationExpr:
		dec.Relation = logic
	case *Conditional:
		dec.Conditional = logic
	case *ForExpr:
		dec.For = logic
	case *Quantified:
		dec.Quantified = logic
	case *FilterExpr:
		dec.Filter = logic
	}
	return dec, diags
}

// mapExpression maps a decoded boxed expression to the model. It returns nil when
// no expression child is present (an undecided position).
func mapExpression(x dmnxml.Expression) Expression {
	switch {
	case x.LiteralExpression != nil:
		le := x.LiteralExpression
		return &LiteralExpression{
			ID:                 le.ID,
			TypeRef:            le.TypeRef,
			ExpressionLanguage: le.ExprLang,
			Text:               strings.TrimSpace(le.Text),
		}
	case x.DecisionTable != nil:
		return mapTable(x.DecisionTable)
	case x.Context != nil:
		return mapContext(x.Context)
	case x.Invocation != nil:
		return mapInvocation(x.Invocation)
	case x.FunctionDefinition != nil:
		return mapFunctionDef(x.FunctionDefinition)
	case x.List != nil:
		return mapList(x.List)
	case x.Relation != nil:
		return mapRelation(x.Relation)
	case x.Conditional != nil:
		return mapConditional(x.Conditional)
	case x.For != nil:
		return mapFor(x.For)
	case x.Every != nil:
		return mapQuantified("every", x.Every)
	case x.Some != nil:
		return mapQuantified("some", x.Some)
	case x.Filter != nil:
		return mapFilter(x.Filter)
	default:
		return nil
	}
}

func mapList(l *dmnxml.List) *ListExpr {
	le := &ListExpr{ID: l.ID}
	for _, it := range l.Items {
		le.Items = append(le.Items, mapExpression(it))
	}
	return le
}

func mapRelation(r *dmnxml.Relation) *RelationExpr {
	re := &RelationExpr{ID: r.ID}
	for _, c := range r.Columns {
		re.Columns = append(re.Columns, c.Name)
	}
	for _, row := range r.Rows {
		mr := RelationRow{}
		for _, cell := range row.Cells {
			mr.Cells = append(mr.Cells, mapExpression(cell))
		}
		re.Rows = append(re.Rows, mr)
	}
	return re
}

func mapConditional(c *dmnxml.Conditional) *Conditional {
	return &Conditional{
		ID:   c.ID,
		If:   mapChild(c.If),
		Then: mapChild(c.Then),
		Else: mapChild(c.Else),
	}
}

func mapFor(it *dmnxml.Iterator) *ForExpr {
	return &ForExpr{
		ID:               it.ID,
		IteratorVariable: it.IteratorVariable,
		In:               mapChild(it.In),
		Return:           mapChild(it.Return),
	}
}

func mapQuantified(kind string, it *dmnxml.Iterator) *Quantified {
	return &Quantified{
		ID:               it.ID,
		Kind:             kind,
		IteratorVariable: it.IteratorVariable,
		In:               mapChild(it.In),
		Satisfies:        mapChild(it.Satisfies),
	}
}

func mapFilter(f *dmnxml.Filter) *FilterExpr {
	return &FilterExpr{ID: f.ID, In: mapChild(f.In), Match: mapChild(f.Match)}
}

// mapChild maps the single expression a holder element wraps, or nil when the
// holder is absent.
func mapChild(c *dmnxml.ChildExpr) Expression {
	if c == nil {
		return nil
	}
	return mapExpression(c.Expression)
}

func mapContext(c *dmnxml.Context) *ContextExpr {
	ctx := &ContextExpr{ID: c.ID}
	for _, e := range c.Entries {
		entry := ContextEntry{Value: mapExpression(e.Expression)}
		if e.Variable != nil {
			entry.Name = e.Variable.Name
			entry.TypeRef = e.Variable.TypeRef
		}
		ctx.Entries = append(ctx.Entries, entry)
	}
	return ctx
}

func mapInvocation(in *dmnxml.Invocation) *Invocation {
	inv := &Invocation{ID: in.ID, Called: mapExpression(in.Expression)}
	for _, b := range in.Bindings {
		bind := Binding{Value: mapExpression(b.Expression)}
		if b.Parameter != nil {
			bind.Parameter = b.Parameter.Name
		}
		inv.Bindings = append(inv.Bindings, bind)
	}
	return inv
}

func mapFunctionDef(fn *dmnxml.FunctionDefinition) *FunctionDef {
	fd := &FunctionDef{ID: fn.ID, Kind: strings.TrimSpace(fn.Kind), Body: mapExpression(fn.Expression)}
	for _, p := range fn.Parameters {
		fd.Parameters = append(fd.Parameters, FunctionParam{Name: p.Name, TypeRef: strings.TrimSpace(p.TypeRef)})
	}
	return fd
}

func mapService(ds dmnxml.DecisionService) *DecisionService {
	return &DecisionService{
		ID:                    ds.ID,
		Name:                  ds.Name,
		OutputDecisions:       localRefs(ds.OutputDecisions),
		EncapsulatedDecisions: localRefs(ds.EncapsulatedDecisions),
		InputDecisions:        localRefs(ds.InputDecisions),
		InputData:             localRefs(ds.InputData),
	}
}

// localRefs resolves a list of href references to their local identifiers,
// dropping empties.
func localRefs(refs []dmnxml.Ref) []string {
	var out []string
	for _, r := range refs {
		if id := localRef(r.Href); id != "" {
			out = append(out, id)
		}
	}
	return out
}

func mapBKM(b dmnxml.BKM) *BKM {
	m := &BKM{ID: b.ID, Name: b.Name, VariableTypeRef: variableTypeRef(b.Variable)}
	if b.EncapsulatedLogic != nil {
		m.EncapsulatedLogic = mapFunctionDef(b.EncapsulatedLogic)
	}
	for _, kr := range b.KnowledgeRequirts {
		if ref := localRef(hrefOf(kr.RequiredKnowledge)); ref != "" {
			m.RequiredKnowledge = append(m.RequiredKnowledge, ref)
		}
	}
	return m
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

// variableName returns the decision/BKM output variable's name (how downstream
// expressions reference its result), or "" when no <variable> is declared.
func variableName(v *dmnxml.Variable) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(v.Name)
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
