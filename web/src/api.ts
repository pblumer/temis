// Thin client for the temis /v1 endpoints the modeler needs. temis stays the
// model authority (ADR-0016): the browser fetches the decision requirements
// graph rather than parsing DMN XML itself.

// dataType is the node's resolved FEEL type; varName a decision's output-variable
// name. x/y/width/height are present only when the model carries DMNDI (absent →
// the client auto-lays-out the graph).
export type GraphNode = {
  id: string
  type: string
  name: string
  dataType?: string
  varName?: string
  hasTable?: boolean
  hasLiteral?: boolean
  x?: number
  y?: number
  width?: number
  height?: number
}
export type GraphEdge = { type: string; source: string; target: string }
export type Graph = { nodes: GraphNode[]; edges: GraphEdge[] }

export type ModelSummary = { modelId: string; name?: string; decisions: string[]; inputs: string[] }

// InputField mirrors dmn.InputField: a decision's typed input, with its optional
// allowed-values constraint, for building the evaluation form.
export type InputField = { name: string; type?: string; required: boolean; constraint?: string }
export type Diagnostic = { severity: string; code: string; message: string }

// ModelDetail mirrors the service modelResponse: decisions/inputs plus the typed
// per-decision input schema used to drive the evaluate form.
export type ModelDetail = {
  modelId: string
  name?: string
  decisions: string[]
  inputs: string[]
  schema?: Record<string, InputField[]>
  diagnostics?: Diagnostic[]
}

export async function listModels(): Promise<ModelSummary[]> {
  const r = await fetch('/v1/models')
  if (!r.ok) throw new Error('Modelle laden fehlgeschlagen (HTTP ' + r.status + ')')
  const body = (await r.json()) as { models?: ModelSummary[] }
  return body.models ?? []
}

export async function getModel(modelId: string): Promise<ModelDetail> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId))
  if (!r.ok) throw new Error('Modell laden fehlgeschlagen (HTTP ' + r.status + ')')
  return (await r.json()) as ModelDetail
}

// createModel uploads a DMN-XML document to the engine, which compiles and caches
// it (POST /v1/models), and returns its detail incl. the typed input schema. This
// is the own modeler's file/paste-deploy path — no dmn-js needed (ADR-0016).
export async function createModel(xml: string): Promise<ModelDetail> {
  const r = await fetch('/v1/models', { method: 'POST', headers: { 'Content-Type': 'application/xml' }, body: xml })
  if (!r.ok) throw new Error(await problemMessage(r, 'Deploy fehlgeschlagen'))
  return (await r.json()) as ModelDetail
}

export async function getGraph(modelId: string): Promise<Graph> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/graph')
  if (!r.ok) throw new Error('Graph laden fehlgeschlagen (HTTP ' + r.status + ')')
  return (await r.json()) as Graph
}

// NodeEdit is one node's persistable edit; omitted fields stay unchanged. x/y are
// only honoured for models that carry DMNDI (otherwise positions auto-lay-out).
export type NodeEdit = { id: string; name?: string; dataType?: string; x?: number; y?: number }

// saveModel patches the model's DMN XML with the given node edits (positions,
// names, types), recompiles it server-side and returns the SAVED model's id —
// the content hash changes, so the caller switches to the new revision. Decision
// logic and untouched diagram interchange are preserved (ADR-0016, Edit→Save).
export async function saveModel(modelId: string, nodes: NodeEdit[]): Promise<string> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/save', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ nodes }),
  })
  if (!r.ok) throw new Error('Speichern fehlgeschlagen (HTTP ' + r.status + ')')
  const body = (await r.json()) as { modelId: string }
  return body.modelId
}

// GraphEdit is the desired full graph for a structural save: every node and edge
// currently on the canvas (not a delta — the server reconciles to this set).
export type GraphNodeEdit = { id: string; type: string; name?: string; dataType?: string; x: number; y: number; width: number; height: number }
export type GraphEdgeEdit = { type: string; source: string; target: string }
export type GraphEdit = { nodes: GraphNodeEdit[]; edges: GraphEdgeEdit[] }

// saveGraph persists the modeler's structural edits (added/removed/moved/renamed
// nodes and edges) by reconciling the model to the given graph (POST), recompiles
// it and returns the saved model's detail with its new id.
export async function saveGraph(modelId: string, edit: GraphEdit): Promise<ModelDetail> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/graph', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(edit),
  })
  if (!r.ok) throw new Error(await problemMessage(r, 'Speichern fehlgeschlagen'))
  return (await r.json()) as ModelDetail
}

// Trace mirrors dmn.Trace: the structured "why" of an evaluation — which decision
// tables ran, the values they tested and which rules matched.
export type TraceCondition = { input: string; entry: string; matched: boolean }
export type TraceRule = { index: number; id?: string; matched: boolean; conditions: TraceCondition[]; outputs?: unknown[] }
export type TraceInput = { expression: string; value: unknown }
export type TableTrace = { hitPolicy: string; aggregation?: string; inputs: TraceInput[]; rules: TraceRule[]; matched: number[] }
export type Trace = { tables: TableTrace[] }

// EvalResult mirrors the service evaluateResponse: the root decision's outputs
// plus every evaluated decision's result, any diagnostics, and (with explain) the
// trace.
export type EvalResult = {
  outputs: Record<string, unknown>
  decisions: Record<string, unknown>
  diagnostics?: Diagnostic[]
  trace?: Trace
}

// InputProblem mirrors dmn.InputProblem: one structured input-validation failure
// (strict mode), keyed by the input name.
export type InputProblem = { input: string; code: string; message: string; expected?: string; got?: string }

// InputValidationError carries the per-input problems from a strict evaluation, so
// the caller can flag the offending fields.
export class InputValidationError extends Error {
  problems: InputProblem[]
  constructor(problems: InputProblem[]) {
    super('Eingaben passen nicht zum Schema')
    this.name = 'InputValidationError'
    this.problems = problems
  }
}

// evaluate runs one decision of a cached model against the given input context
// (POST /v1/models/{id}/evaluate). With explain, the result carries the decision
// trace (which rules matched). With strict, the engine validates inputs against
// their declared types and allowed values, surfaced as an InputValidationError.
export async function evaluate(modelId: string, decision: string, input: Record<string, unknown>, explain = false, strict = false): Promise<EvalResult> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/evaluate', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ decision, input, explain, strict }),
  })
  if (!r.ok) {
    const problem = (await r.json().catch(() => ({}))) as { code?: string; problems?: InputProblem[]; detail?: string }
    if (problem.code === 'INVALID_INPUT' && problem.problems?.length) throw new InputValidationError(problem.problems)
    throw new Error(problem.detail || 'Auswertung fehlgeschlagen (HTTP ' + r.status + ')')
  }
  return (await r.json()) as EvalResult
}

// GraphEvalResult mirrors the service evaluateGraphResponse: every decision's
// value (keyed by name), per-decision traces (with explain), per-decision
// evaluation errors, and the leaf-input schema the whole graph consumes — so the
// form is built from one authoritative source.
export type GraphEvalResult = {
  values: Record<string, unknown>
  traces?: Record<string, Trace>
  errors?: Record<string, string>
  inputSchema: InputField[]
  diagnostics?: Diagnostic[]
}

// evaluateGraph fills the model's leaf inputs once and returns every decision's
// result (POST /v1/models/{id}/evaluate-graph), so the modeler can show the whole
// DRG computed from a single set of inputs. With explain, each decision carries
// its trace; with strict, inputs are validated against the model's whole-graph
// schema and a bad input surfaces as an InputValidationError.
export async function evaluateGraph(modelId: string, input: Record<string, unknown>, explain = false, strict = false): Promise<GraphEvalResult> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/evaluate-graph', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ input, explain, strict }),
  })
  if (!r.ok) {
    const problem = (await r.json().catch(() => ({}))) as { code?: string; problems?: InputProblem[]; detail?: string }
    if (problem.code === 'INVALID_INPUT' && problem.problems?.length) throw new InputValidationError(problem.problems)
    throw new Error(problem.detail || 'Auswertung fehlgeschlagen (HTTP ' + r.status + ')')
  }
  return (await r.json()) as GraphEvalResult
}

// TableView mirrors dmn.TableView: a decision's static decision-table logic for
// display in the modeler.
export type TableInput = { label?: string; expression: string; typeRef?: string }
export type TableOutput = { name?: string; label?: string; typeRef?: string }
export type TableRule = { inputEntries: string[]; outputEntries: string[]; annotations?: string[] }
export type TableView = {
  decisionId: string
  name: string
  hitPolicy: string
  aggregation?: string
  inputs: TableInput[]
  outputs: TableOutput[]
  rules: TableRule[]
}

// getTable fetches a decision's decision-table view, or null when the decision
// has no decision-table logic (HTTP 404).
export async function getTable(modelId: string, decision: string): Promise<TableView | null> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/decisions/' + encodeURIComponent(decision) + '/table')
  if (r.status === 404) return null
  if (!r.ok) throw new Error('Decision Table laden fehlgeschlagen (HTTP ' + r.status + ')')
  const tv = (await r.json()) as TableView
  // A freshly created table can have no columns/rules yet; Go serialises empty
  // slices as null, so normalise to arrays for the editor.
  tv.inputs = tv.inputs ?? []
  tv.outputs = tv.outputs ?? []
  tv.rules = tv.rules ?? []
  return tv
}

// createDecisionTable gives an undecided decision a fresh decision table (columns
// derived from its requirements server-side) and returns the saved model's
// detail with its new id.
export async function createDecisionTable(modelId: string, decision: string): Promise<ModelDetail> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/decisions/' + encodeURIComponent(decision) + '/create-table', { method: 'POST' })
  if (!r.ok) throw new Error(await problemMessage(r, 'Tabelle anlegen fehlgeschlagen'))
  return (await r.json()) as ModelDetail
}

// TableEdit is the editable payload for a decision table. Rules are always
// written; hitPolicy/aggregation set the policy; inputs/outputs replace the
// columns only when replaceColumns is set (the full editor sends everything).
export type TableEdit = {
  rules: TableRule[]
  hitPolicy?: string
  aggregation?: string
  inputs?: TableInput[]
  outputs?: TableOutput[]
  replaceColumns?: boolean
}

// saveTable rewrites a decision's table rules (POST), recompiles the model and
// returns the saved model's detail — incl. its new id and any compile
// diagnostics, so the caller can surface a cell the engine rejects.
export async function saveTable(modelId: string, decision: string, edit: TableEdit): Promise<ModelDetail> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/decisions/' + encodeURIComponent(decision) + '/table', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(edit),
  })
  if (!r.ok) throw new Error(await problemMessage(r, 'Tabelle speichern fehlgeschlagen'))
  return (await r.json()) as ModelDetail
}

// ItemType mirrors dmn.ItemType: a model's named type (a base FEEL type with an
// optional collection flag and allowed-values constraint). structured types
// (with components) are read-only in the simple editor.
export type ItemType = { name: string; typeRef?: string; isCollection?: boolean; allowedValues?: string; structured?: boolean }

export async function listTypes(modelId: string): Promise<ItemType[]> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/types')
  if (!r.ok) throw new Error('Typen laden fehlgeschlagen (HTTP ' + r.status + ')')
  return ((await r.json()) as { types?: ItemType[] }).types ?? []
}

// saveType creates or updates a custom type and returns the saved model's detail
// (with its new id).
export async function saveType(modelId: string, t: ItemType): Promise<ModelDetail> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/types', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(t),
  })
  if (!r.ok) throw new Error(await problemMessage(r, 'Typ speichern fehlgeschlagen'))
  return (await r.json()) as ModelDetail
}

// deleteType removes a custom type and returns the saved model's detail.
export async function deleteType(modelId: string, name: string): Promise<ModelDetail> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/types/' + encodeURIComponent(name), { method: 'DELETE' })
  if (!r.ok) throw new Error(await problemMessage(r, 'Typ löschen fehlgeschlagen'))
  return (await r.json()) as ModelDetail
}

// BKMView mirrors dmn.BKMView: a business knowledge model's function (formal
// parameters + literal body). simple=false means a boxed body (read-only here).
export type BKMParam = { name: string; typeRef?: string }
export type BKMView = { bkmId: string; name: string; params: BKMParam[]; bodyText: string; bodyTypeRef?: string; simple: boolean }
export type BKMFunctionEdit = { params: BKMParam[]; bodyText: string; bodyTypeRef: string }

export async function getBKM(modelId: string, bkm: string): Promise<BKMView | null> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/bkm/' + encodeURIComponent(bkm))
  if (r.status === 404) return null
  if (!r.ok) throw new Error('BKM laden fehlgeschlagen (HTTP ' + r.status + ')')
  return (await r.json()) as BKMView
}

// saveBKM sets a BKM's function (parameters + literal body), recompiles the model
// and returns the saved detail with its new id.
export async function saveBKM(modelId: string, bkm: string, edit: BKMFunctionEdit): Promise<ModelDetail> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/bkm/' + encodeURIComponent(bkm), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(edit),
  })
  if (!r.ok) throw new Error(await problemMessage(r, 'BKM speichern fehlgeschlagen'))
  return (await r.json()) as ModelDetail
}

// LiteralView mirrors dmn.LiteralView: a decision's literal FEEL expression.
export type LiteralView = { decisionId: string; name: string; text: string; typeRef?: string }

// getLiteral fetches a decision's literal expression, or null when the decision
// has no literal logic (HTTP 404 — e.g. it is undecided or a decision table).
export async function getLiteral(modelId: string, decision: string): Promise<LiteralView | null> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/decisions/' + encodeURIComponent(decision) + '/literal')
  if (r.status === 404) return null
  if (!r.ok) throw new Error('Ausdruck laden fehlgeschlagen (HTTP ' + r.status + ')')
  return (await r.json()) as LiteralView
}

// saveLiteral sets (or creates) a decision's literal expression (POST), recompiles
// the model and returns the saved model's detail with its new id.
export async function saveLiteral(modelId: string, decision: string, text: string, typeRef: string): Promise<ModelDetail> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/decisions/' + encodeURIComponent(decision) + '/literal', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ text, typeRef }),
  })
  if (!r.ok) throw new Error(await problemMessage(r, 'Ausdruck speichern fehlgeschlagen'))
  return (await r.json()) as ModelDetail
}

// problemMessage extracts a human-readable message from an RFC-7807 problem+json
// error body, including structured input-validation problems, falling back to the
// HTTP status.
async function problemMessage(r: Response, fallback: string): Promise<string> {
  try {
    const p = (await r.json()) as { detail?: string; problems?: { message: string }[] }
    let msg = p.detail || fallback
    if (p.problems?.length) msg += ': ' + p.problems.map((x) => x.message).join('; ')
    return msg
  } catch {
    return fallback + ' (HTTP ' + r.status + ')'
  }
}
