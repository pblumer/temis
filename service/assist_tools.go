package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pblumer/temis/assist"
	"github.com/pblumer/temis/dmn"
)

// defaultAssistSystem is the built-in system prompt for the modeling assistant.
// It teaches the model what temis is, how DMN/FEEL work, the tools it has and the
// verify-before-trust workflow that is temis's whole point (ADR-0013): the
// assistant checks its own proposals against the real engine with `evaluate`
// instead of guessing.
const defaultAssistSystem = `Du bist der eingebaute Modellierungs-Assistent von temis, einer DMN-1.5-Decision-Engine.
Du hilfst Anwendern, Entscheidungen (Decisions) zu verstehen und zu bauen — vor allem Decision-Tables mit FEEL-Ausdrücken.

Grundsätze:
- Antworte knapp und auf Deutsch, es sei denn der Nutzer schreibt in einer anderen Sprache.
- Du hast Werkzeuge, um echte Modelle im Server zu inspizieren, auszuwerten und zu ändern. Rate nicht — nutze die Werkzeuge.
- temis ist ein Verifikationswerkzeug: Wenn du eine Decision-Table oder Regel vorschlägst, prüfe sie mit dem Werkzeug "evaluate" an Beispiel-Eingaben, bevor du sie als korrekt bezeichnest.
- Bevor du ein Modell änderst, sieh es dir mit "list_models", "describe_decision" und "get_decision_table" an.
- Neue Modelle legst du mit "load_model" an (vollständiges DMN-1.5-XML). Bestehende Decision-Tables änderst du mit "save_decision_table"; eine leere Decision bekommt mit "create_decision_table" zuerst eine Tabelle.
- Wenn ein Werkzeug Diagnostics (Compile-Fehler mit Zeile/Spalte) zurückgibt, behebe sie und versuche es erneut.
- FEEL-Hinweise: Unary Tests in Eingabezellen (z. B. "< 18", "[1..10]", "\"Winter\"", "-" für beliebig); Hit Policies U/A/P/F/R/O/C; Ausgaben sind FEEL-Ausdrücke.
- Nenne dem Nutzer am Ende die modelId, wenn du ein Modell erstellt oder geändert hast, damit er es im Modeler laden kann.`

// assistExecutor implements assist.Executor over the server's model store and
// engine (ADR-0024). It is built fresh per chat request; lastModel records the id
// of the most recent model a build tool produced, so the handler can tell the
// modeler which revision to reload.
type assistExecutor struct {
	s         *Server
	lastModel string
}

func newAssistExecutor(s *Server) *assistExecutor { return &assistExecutor{s: s} }

// Tools is the catalog the assistant may drive. The schemas mirror temis's
// existing operations so the model can inspect, verify and build decisions.
func (e *assistExecutor) Tools() []assist.Tool {
	return []assist.Tool{
		{
			Name:        "list_models",
			Description: "List every model currently loaded in the server, with its id, name, decisions and inputs.",
			Schema:      json.RawMessage(`{"type":"object","properties":{}}`),
		},
		{
			Name:        "describe_decision",
			Description: "Describe a decision's typed input schema (each input's name, FEEL type and whether it is required).",
			Schema:      json.RawMessage(`{"type":"object","properties":{"modelId":{"type":"string"},"decision":{"type":"string","description":"decision id or name"}},"required":["modelId","decision"]}`),
		},
		{
			Name:        "get_decision_table",
			Description: "Return a decision's decision-table: hit policy, input/output columns and rule rows.",
			Schema:      json.RawMessage(`{"type":"object","properties":{"modelId":{"type":"string"},"decision":{"type":"string"}},"required":["modelId","decision"]}`),
		},
		{
			Name:        "evaluate",
			Description: "Evaluate a decision of a loaded model against a JSON input context and return its outputs. Set explain=true to also get the decision trace (which rules matched). Use this to verify a table or rule you propose.",
			Schema:      json.RawMessage(`{"type":"object","properties":{"modelId":{"type":"string"},"decision":{"type":"string"},"input":{"type":"object","description":"input values keyed by input name"},"explain":{"type":"boolean"}},"required":["modelId","decision"]}`),
		},
		{
			Name:        "load_model",
			Description: "Compile and load a complete DMN 1.5 XML document into the server. Returns the new modelId and any compile diagnostics. Use this to create a model from scratch.",
			Schema:      json.RawMessage(`{"type":"object","properties":{"xml":{"type":"string","description":"a complete DMN 1.5 XML document"}},"required":["xml"]}`),
		},
		{
			Name:        "create_decision_table",
			Description: "Give an undecided decision a fresh (empty) decision table, with columns derived from its requirements. Returns the new modelId.",
			Schema:      json.RawMessage(`{"type":"object","properties":{"modelId":{"type":"string"},"decision":{"type":"string"}},"required":["modelId","decision"]}`),
		},
		{
			Name: "save_decision_table",
			Description: "Rewrite a decision's decision-table rules (and optionally its hit policy and columns), recompile and cache the model. Returns the new modelId and any compile diagnostics. " +
				"Each rule's inputEntries are FEEL unary tests aligned with the input columns ('-' means any); outputEntries are FEEL expressions aligned with the output columns. Set replaceColumns=true to also replace inputs/outputs.",
			Schema: json.RawMessage(`{"type":"object","properties":{` +
				`"modelId":{"type":"string"},"decision":{"type":"string"},` +
				`"hitPolicy":{"type":"string","description":"U, A, P, F, R, O or C"},"aggregation":{"type":"string","description":"for Collect: SUM, MIN, MAX or COUNT"},` +
				`"replaceColumns":{"type":"boolean"},` +
				`"inputs":{"type":"array","items":{"type":"object","properties":{"label":{"type":"string"},"expression":{"type":"string"},"typeRef":{"type":"string"}}}},` +
				`"outputs":{"type":"array","items":{"type":"object","properties":{"name":{"type":"string"},"label":{"type":"string"},"typeRef":{"type":"string"}}}},` +
				`"rules":{"type":"array","items":{"type":"object","properties":{"inputEntries":{"type":"array","items":{"type":"string"}},"outputEntries":{"type":"array","items":{"type":"string"}},"annotations":{"type":"array","items":{"type":"string"}}}}}` +
				`},"required":["modelId","decision","rules"]}`),
		},
	}
}

// Execute dispatches a tool call to its handler. A returned error is reported
// back to the model as a failed tool result (so it can recover), not as a fatal
// error.
func (e *assistExecutor) Execute(ctx context.Context, name string, args json.RawMessage) (string, error) {
	switch name {
	case "list_models":
		return e.listModels()
	case "describe_decision":
		return e.describeDecision(args)
	case "get_decision_table":
		return e.getDecisionTable(args)
	case "evaluate":
		return e.evaluate(ctx, args)
	case "load_model":
		return e.loadModel(ctx, args)
	case "create_decision_table":
		return e.createDecisionTable(ctx, args)
	case "save_decision_table":
		return e.saveDecisionTable(ctx, args)
	default:
		return "", fmt.Errorf("unknown tool %q", name)
	}
}

// --- tool argument types ---

type decisionArgs struct {
	ModelID  string `json:"modelId"`
	Decision string `json:"decision"`
}

type evaluateArgs struct {
	ModelID  string         `json:"modelId"`
	Decision string         `json:"decision"`
	Input    map[string]any `json:"input"`
	Explain  bool           `json:"explain"`
}

type loadModelArgs struct {
	XML string `json:"xml"`
}

type saveTableArgs struct {
	ModelID  string `json:"modelId"`
	Decision string `json:"decision"`
	dmn.TableEdit
}

// --- tool implementations ---

func (e *assistExecutor) listModels() (string, error) {
	models := e.s.cache.snapshot()
	out := make([]modelSummary, 0, len(models))
	for _, sm := range models {
		out = append(out, modelSummary{
			ModelID:   sm.id,
			Name:      sm.name,
			Decisions: sm.index.Decisions,
			Inputs:    sm.index.Inputs,
			Seq:       sm.seq,
		})
	}
	return jsonResult(map[string]any{"models": out, "count": len(out)})
}

func (e *assistExecutor) describeDecision(args json.RawMessage) (string, error) {
	var a decisionArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	sm, ok := e.s.lookup(a.ModelID)
	if !ok {
		return "", fmt.Errorf("no model with id %q", a.ModelID)
	}
	fields, err := sm.defs.InputSchema(a.Decision)
	if err != nil {
		return "", err
	}
	res := map[string]any{"modelId": sm.id, "decision": a.Decision, "inputs": fields}
	if table, ok := sm.defs.DecisionTable(a.Decision); ok {
		res["hitPolicy"] = table.HitPolicy
		res["outputs"] = table.Outputs
	}
	return jsonResult(res)
}

func (e *assistExecutor) getDecisionTable(args json.RawMessage) (string, error) {
	var a decisionArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	sm, ok := e.s.lookup(a.ModelID)
	if !ok {
		return "", fmt.Errorf("no model with id %q", a.ModelID)
	}
	table, ok := sm.defs.DecisionTable(a.Decision)
	if !ok {
		return "", fmt.Errorf("decision %q has no decision table", a.Decision)
	}
	return jsonResult(table)
}

func (e *assistExecutor) evaluate(ctx context.Context, args json.RawMessage) (string, error) {
	var a evaluateArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	sm, ok := e.s.lookup(a.ModelID)
	if !ok {
		return "", fmt.Errorf("no model with id %q", a.ModelID)
	}
	dec, err := sm.defs.Decision(a.Decision)
	if err != nil {
		return "", err
	}
	var opts []dmn.EvalOption
	if a.Explain {
		opts = append(opts, dmn.WithTrace())
	}
	res, err := dec.Evaluate(ctx, dmn.Input(a.Input), opts...)
	if err != nil {
		return "", err
	}
	out := map[string]any{"outputs": res.Outputs, "decisions": res.Decisions}
	if res.Trace != nil {
		out["trace"] = res.Trace
	}
	return jsonResult(out)
}

func (e *assistExecutor) loadModel(ctx context.Context, args json.RawMessage) (string, error) {
	var a loadModelArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if a.XML == "" {
		return "", fmt.Errorf("missing xml")
	}
	sm, err := e.s.compileAndStore(ctx, []byte(a.XML))
	if err != nil {
		return "", fmt.Errorf("compile failed: %w", err)
	}
	e.lastModel = sm.id
	return e.savedResult(sm)
}

func (e *assistExecutor) createDecisionTable(ctx context.Context, args json.RawMessage) (string, error) {
	var a decisionArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	sm, ok := e.s.lookup(a.ModelID)
	if !ok {
		return "", fmt.Errorf("no model with id %q", a.ModelID)
	}
	id, err := resolveDecisionID(sm, a.Decision)
	if err != nil {
		return "", err
	}
	patched, err := dmn.CreateDecisionTable(sm.xml, id)
	if err != nil {
		return "", err
	}
	saved, err := e.s.compileAndStore(ctx, patched)
	if err != nil {
		return "", fmt.Errorf("compile failed: %w", err)
	}
	e.lastModel = saved.id
	return e.savedResult(saved)
}

func (e *assistExecutor) saveDecisionTable(ctx context.Context, args json.RawMessage) (string, error) {
	var a saveTableArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	sm, ok := e.s.lookup(a.ModelID)
	if !ok {
		return "", fmt.Errorf("no model with id %q", a.ModelID)
	}
	id, err := resolveDecisionID(sm, a.Decision)
	if err != nil {
		return "", err
	}
	patched, err := dmn.ApplyTableEdit(sm.xml, id, a.TableEdit)
	if err != nil {
		return "", err
	}
	saved, err := e.s.compileAndStore(ctx, patched)
	if err != nil {
		return "", fmt.Errorf("compile failed: %w", err)
	}
	e.lastModel = saved.id
	return e.savedResult(saved)
}

// savedResult renders the common "model saved" tool result: the new id, its
// decisions/inputs and any compile diagnostics (so the model can fix a rejected
// cell and retry).
func (e *assistExecutor) savedResult(sm *storedModel) (string, error) {
	return jsonResult(map[string]any{
		"modelId":     sm.id,
		"name":        sm.name,
		"decisions":   sm.index.Decisions,
		"inputs":      sm.index.Inputs,
		"diagnostics": toDiagnosticDTOs(sm.diags),
	})
}

// resolveDecisionID maps a decision id-or-name (as the assistant may use either)
// to its canonical id, which the XML-patching helpers require.
func resolveDecisionID(sm *storedModel, idOrName string) (string, error) {
	dec, err := sm.defs.Decision(idOrName)
	if err != nil {
		return "", err
	}
	return dec.ID(), nil
}

// jsonResult marshals a tool result to a compact JSON string for the model.
func jsonResult(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
