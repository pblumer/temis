package dmn

import (
	"context"
	"errors"
	"fmt"

	"github.com/pblumer/temis/internal/boxed"
	"github.com/pblumer/temis/internal/feel"
	"github.com/pblumer/temis/internal/model"
	dmnxml "github.com/pblumer/temis/internal/xml"
)

// Engine compiles DMN models. It is re-entrant and holds no mutable state, so a
// single Engine may be shared across goroutines.
type Engine struct {
	cfg config
}

// config carries Engine options. Concrete fields are added alongside the
// features that consume them; the type and the Option signature stay stable.
type config struct {
	limits    Limits
	limitsSet bool
}

// Option configures an Engine passed to New (e.g. WithLimits).
type Option func(*config)

// New returns an Engine configured with the given options.
func New(opts ...Option) *Engine {
	var cfg config
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Engine{cfg: cfg}
}

// Compile decodes and compiles a complete DMN XML document. Malformed XML is a
// hard error; per-decision problems (unknown variables, unsupported constructs,
// unrecognised namespaces) are reported through the returned Diagnostics while
// the rest of the model still compiles. Decisions whose logic fails to compile
// are present in the result but not executable.
func (e *Engine) Compile(ctx context.Context, xml []byte) (*Definitions, Diagnostics, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	// A configured compile timeout bounds pathological models; it applies only
	// when the caller's context carries no earlier deadline of its own.
	if d := e.cfg.limits.CompileTimeout; d > 0 {
		if _, ok := ctx.Deadline(); !ok {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, d)
			defer cancel()
		}
	}
	lim := e.cfg.feelLimits()

	raw, err := dmnxml.Decode(xml)
	if err != nil {
		return nil, nil, err
	}

	m, modelDiags, err := model.FromXML(raw)
	if err != nil {
		return nil, nil, err
	}

	defs := &Definitions{
		model:  m,
		byID:   make(map[string]*CompiledDecision, len(m.Decisions)),
		byName: make(map[string]*CompiledDecision, len(m.Decisions)),
	}
	diags := fromModelDiagnostics(modelDiags)

	items := buildItemTypes(m)

	funcs, funcDiags := compileBKMs(m, items)
	diags = append(diags, funcDiags...)

	// Register each decision service as an invocable function so a decision's FEEL
	// can call it by name (DMN §10.4, TCK 0085). The closure resolves the compiled
	// service lazily, since services are compiled after the decisions here.
	registerServiceInvocables(m, defs, funcs, items, lim)

	for _, dec := range m.Decisions {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		cd, dd := compileDecision(m, dec, funcs, items)
		cd.limits = lim
		diags = append(diags, dd...)
		defs.order = append(defs.order, cd)
		if cd.id != "" {
			defs.byID[cd.id] = cd
		}
		// Resolve by the FEEL identifier (cd.name) and, for backward compatibility,
		// by the display name when it differs — so callers may still address a
		// decision by the label it shows in the diagram.
		if cd.name != "" {
			defs.byName[cd.name] = cd
		}
		if dec.Name != "" && dec.Name != cd.name {
			if _, taken := defs.byName[dec.Name]; !taken {
				defs.byName[dec.Name] = cd
			}
		}
	}

	diags = append(diags, wireRequirements(defs, m)...)
	diags = append(diags, compileServices(defs, m, items)...)
	for _, cs := range defs.serviceOrder {
		cs.limits = lim
	}
	diags = append(diags, typecheckModel(m, funcs, items)...)

	return defs, diags, nil
}

// compileBKMs compiles every business knowledge model into a named FEEL function
// available to all expressions. Functions are registered before their bodies
// compile, so a model may call itself (recursion) or a sibling (mutual
// recursion). A body that fails to compile yields a diagnostic and an
// uncallable (nil-body) function.
func compileBKMs(m *model.Definitions, items map[string]*feel.Type) (map[string]*feel.Func, Diagnostics) {
	funcs := make(map[string]*feel.Func)
	for _, b := range m.BKMs {
		if b.Name == "" || b.EncapsulatedLogic == nil {
			continue
		}
		fn := &feel.Func{Name: b.Name, ResultType: bkmResultType(b, items)}
		for _, p := range b.EncapsulatedLogic.Parameters {
			fn.Params = append(fn.Params, p.Name)
			fn.ParamTypes = append(fn.ParamTypes, resolveType(p.TypeRef, items))
		}
		funcs[b.Name] = fn
	}

	var diags Diagnostics
	for _, b := range m.BKMs {
		fn, ok := funcs[b.Name]
		if !ok {
			continue
		}
		bodyEnv := feel.NewEnv(fn.Params...).WithTypes(items)
		body, err := boxed.Compile(b.EncapsulatedLogic.Body, bodyEnv, funcs)
		if err != nil {
			diags = append(diags, Diagnostic{
				Severity: SevError,
				Code:     CodeFEELCompile,
				Message:  fmt.Sprintf("business knowledge model %q: %s", b.Name, err.Error()),
			})
			continue
		}
		fn.Body = body
	}
	return funcs, diags
}

// bkmResultType resolves a BKM's declared return type for output coercion: the
// bound variable's type if declared, otherwise the encapsulated body's own type
// (a literal expression may declare one). Any (nil) imposes no coercion.
func bkmResultType(b *model.BKM, items map[string]*feel.Type) *feel.Type {
	if b.VariableTypeRef != "" {
		return resolveType(b.VariableTypeRef, items)
	}
	if le, ok := b.EncapsulatedLogic.Body.(*model.LiteralExpression); ok && le.TypeRef != "" {
		return resolveType(le.TypeRef, items)
	}
	return nil
}

// compileDecision compiles one decision's logic into a CompiledDecision. A
// decision without a literal expression or decision table, or whose logic fails
// to compile, yields a CompiledDecision with a nil expr (not executable) plus a
// diagnostic for the failure.
func compileDecision(m *model.Definitions, dec *model.Decision, funcs map[string]*feel.Func, items map[string]*feel.Type) (*CompiledDecision, Diagnostics) {
	env := feel.NewEnv(envNames(m, dec)...).WithTypes(items)
	constraints := buildConstraints(m, dec, items)
	cd := &CompiledDecision{
		id: dec.ID,
		// The FEEL identifier the result is bound under and referenced by — the
		// decision's variable name, falling back to its (display) name. Name stays
		// the free-form label (used for diagnostics via decisionLabel).
		name:        dec.RefName(),
		env:         env,
		inputs:      buildInputSchema(m, dec, items, constraints),
		reqInputs:   reqInputNames(m, dec),
		constraints: constraints,
		outType:     resolveType(dec.VariableTypeRef, items),
	}

	logic := dec.Logic()
	if logic == nil {
		// No executable logic; FromXML already emitted a warning for this.
		return cd, nil
	}
	ce, err := boxed.Compile(logic, env, funcs)
	if err != nil {
		return cd, Diagnostics{compileDiagnostic(dec, err)}
	}
	cd.expr = ce
	return cd, nil
}

// envNames returns the variable names visible to a decision's expressions: the
// names of its required input data and required decisions, resolved from their
// local identifiers. Duplicates and unresolved references are dropped. The DRG
// evaluator fills the required-decision slots automatically by evaluating those
// decisions first (WP-28; see Evaluate).
func envNames(m *model.Definitions, dec *model.Decision) []string {
	byID := make(map[string]string, len(m.InputData)+len(m.Decisions))
	for _, in := range m.InputData {
		byID[in.ID] = in.RefName()
	}
	for _, d := range m.Decisions {
		byID[d.ID] = d.RefName()
	}

	var names []string
	seen := make(map[string]bool)
	add := func(id string) {
		name, ok := byID[id]
		if !ok || name == "" || seen[name] {
			return
		}
		seen[name] = true
		names = append(names, name)
	}
	for _, id := range dec.RequiredInputs {
		add(id)
	}
	for _, id := range dec.RequiredDecisions {
		add(id)
	}
	return names
}

// reqInputNames returns the resolved names of a decision's required input data
// (input data only, not required decisions). Names are looked up from the
// model's InputData; duplicates, empty names and unresolved references are
// dropped. These are the inputs Evaluate treats as mandatory.
func reqInputNames(m *model.Definitions, dec *model.Decision) []string {
	byID := make(map[string]string, len(m.InputData))
	for _, in := range m.InputData {
		byID[in.ID] = in.RefName()
	}

	var names []string
	seen := make(map[string]bool)
	for _, id := range dec.RequiredInputs {
		name, ok := byID[id]
		if !ok || name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names
}

// compileDiagnostic turns a FEEL/boxed compile error into a diagnostic, carrying
// the source position when the error exposes one.
func compileDiagnostic(dec *model.Decision, err error) Diagnostic {
	d := Diagnostic{
		Severity:   SevError,
		Code:       CodeFEELCompile,
		Message:    err.Error(),
		DecisionID: dec.ID,
	}
	var ce *feel.CompileError
	if errors.As(err, &ce) {
		d.Line, d.Col = ce.Line, ce.Col
		d.Message = fmt.Sprintf("decision %q: %s", decisionLabel(dec), ce.Msg)
	} else {
		d.Message = fmt.Sprintf("decision %q: %s", decisionLabel(dec), err.Error())
	}
	return d
}

// decisionLabel is the human-facing identifier of a decision: its name, or its
// ID when unnamed.
func decisionLabel(dec *model.Decision) string {
	if dec.Name != "" {
		return dec.Name
	}
	return dec.ID
}
