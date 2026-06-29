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

// config carries Engine options. Concrete fields (limits, clock, locale) are
// added alongside the features that consume them (WP-22/WP-34); the type exists
// now so the New/Option signatures stay stable.
type config struct{}

// Option configures an Engine passed to New. No options are defined yet; the
// type is fixed so callers and the constructor signature remain stable as
// configuration lands in later work packages.
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

	for _, dec := range m.Decisions {
		cd, dd := compileDecision(m, dec)
		diags = append(diags, dd...)
		defs.order = append(defs.order, cd)
		if cd.id != "" {
			defs.byID[cd.id] = cd
		}
		if cd.name != "" {
			defs.byName[cd.name] = cd
		}
	}

	return defs, diags, nil
}

// compileDecision compiles one decision's logic into a CompiledDecision. A
// decision without a literal expression or decision table, or whose logic fails
// to compile, yields a CompiledDecision with a nil expr (not executable) plus a
// diagnostic for the failure.
func compileDecision(m *model.Definitions, dec *model.Decision) (*CompiledDecision, Diagnostics) {
	env := feel.NewEnv(envNames(m, dec)...)
	cd := &CompiledDecision{
		id:        dec.ID,
		name:      dec.Name,
		env:       env,
		inputs:    buildInputSchema(m, dec),
		reqInputs: reqInputNames(m, dec),
	}

	switch {
	case dec.LiteralExpression != nil:
		ce, err := feel.CompileString(dec.LiteralExpression.Text, env)
		if err != nil {
			return cd, Diagnostics{compileDiagnostic(dec, err)}
		}
		cd.expr = ce
	case dec.DecisionTable != nil:
		ce, err := boxed.CompileTable(dec.DecisionTable, env)
		if err != nil {
			return cd, Diagnostics{compileDiagnostic(dec, err)}
		}
		cd.expr = ce
	default:
		// No executable logic; FromXML already emitted a warning for this.
	}

	return cd, nil
}

// envNames returns the variable names visible to a decision's expressions: the
// names of its required input data and required decisions, resolved from their
// local identifiers. Duplicates and unresolved references are dropped. Wiring
// required-decision results automatically is the job of the DRG evaluator
// (WP-28); until then the caller supplies them as inputs.
func envNames(m *model.Definitions, dec *model.Decision) []string {
	byID := make(map[string]string, len(m.InputData)+len(m.Decisions))
	for _, in := range m.InputData {
		byID[in.ID] = in.Name
	}
	for _, d := range m.Decisions {
		byID[d.ID] = d.Name
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
		byID[in.ID] = in.Name
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
