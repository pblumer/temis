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
  hasContext?: boolean
  hasConditional?: boolean
  hasList?: boolean
  hasRelation?: boolean
  hasFilter?: boolean
  hasIterator?: boolean
  hasLogic?: boolean
  x?: number
  y?: number
  width?: number
  height?: number
}
export type GraphEdge = { type: string; source: string; target: string }
export type Graph = { nodes: GraphNode[]; edges: GraphEdge[] }

// seq is the model's server-side creation order (higher = newer), so same-named
// revisions can be shown newest-first as a history.
export type ModelSummary = { modelId: string; name?: string; decisions: string[]; inputs: string[]; seq?: number }

// InputField mirrors dmn.InputField: a decision's typed input, with its optional
// allowed-values constraint and discrete suggested values, for building the
// evaluation form. values lists the discrete values to offer (from a declared
// enumeration or the literals used in decision-table cells); valuesClosed is true
// when that set is exhaustive (offer a closed dropdown).
export type InputField = { name: string; type?: string; required: boolean; constraint?: string; values?: string[]; valuesClosed?: boolean }
// Diagnostic mirrors dmn.Diagnostic: a problem found while compiling/evaluating.
// decisionId ties it to a node (empty for model-level problems); line/col give the
// FEEL source position when applicable.
export type Diagnostic = { severity: string; code: string; message: string; decisionId?: string; line?: number; col?: number }

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

// renameModel sets a model's display name (the DMN definitions name), recompiles
// it server-side and returns the saved model's detail with its new id — the
// content hash changes, so the caller switches to the new revision. The original
// stays cached (the modeler cleans it up when it renames a whole named group).
export async function renameModel(modelId: string, name: string): Promise<ModelDetail> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/rename', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  })
  if (!r.ok) throw new Error(await problemMessage(r, 'Umbenennen fehlgeschlagen'))
  return (await r.json()) as ModelDetail
}

// deleteModel removes one cached model revision (DELETE /v1/models/{id}). The
// modeler deletes a whole named model by calling this once per revision. A 404
// (already gone) is treated as success so a group delete is idempotent.
export async function deleteModel(modelId: string): Promise<void> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId), { method: 'DELETE' })
  if (!r.ok && r.status !== 404) throw new Error(await problemMessage(r, 'Löschen fehlgeschlagen'))
}

// createModel uploads a DMN-XML document to the engine, which compiles and caches
// it (POST /v1/models), and returns its detail incl. the typed input schema. This
// is the own modeler's file/paste-deploy path — no dmn-js needed (ADR-0016).
export async function createModel(xml: string): Promise<ModelDetail> {
  const r = await fetch('/v1/models', { method: 'POST', headers: { 'Content-Type': 'application/xml' }, body: xml })
  if (!r.ok) throw new Error(await problemMessage(r, 'Deploy fehlgeschlagen'))
  return (await r.json()) as ModelDetail
}

// blankModelXML builds a minimal, valid DMN document for a brand-new model: an
// empty definitions carrying the given display name and a unique id/namespace, so
// two blank models never collide in temis's content-addressed cache. The modeler
// fills it in afterwards via the palette + structural save (ADR-0016) — this is
// the "create from scratch" counterpart to createModel's file/paste upload.
export function blankModelXML(name: string): string {
  const raw = typeof crypto !== 'undefined' && crypto.randomUUID ? crypto.randomUUID() : 'm' + Date.now().toString(36)
  const uid = raw.replace(/[^a-zA-Z0-9-]/g, '')
  const esc = (s: string): string => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
  return `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20191111/MODEL/"
             xmlns:dmndi="https://www.omg.org/spec/DMN/20191111/DMNDI/"
             xmlns:dc="http://www.omg.org/spec/DMN/20180521/DC/"
             xmlns:di="http://www.omg.org/spec/DMN/20180521/DI/"
             id="Definitions_${uid}"
             name="${esc(name)}"
             namespace="http://temis/models/${uid}">
</definitions>`
}

// createBlankModel deploys a fresh, empty model with the given name to the engine
// and returns its detail — the modeler's "new decision file" path (no upload).
export async function createBlankModel(name: string): Promise<ModelDetail> {
  return createModel(blankModelXML(name))
}

// getGraph returns the model's decision requirements graph. An empty model (a
// blank, freshly created one) has no nodes/edges, and the engine serialises
// those empty slices as JSON null — so coerce to arrays here, keeping the Graph
// contract (arrays, never null) that layout()/renderGraph rely on. Without this
// a brand-new model throws in layout() and the canvas keeps the old model.
export async function getGraph(modelId: string): Promise<Graph> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/graph')
  if (!r.ok) throw new Error('Graph laden fehlgeschlagen (HTTP ' + r.status + ')')
  const body = (await r.json()) as { nodes?: GraphNode[] | null; edges?: GraphEdge[] | null }
  return { nodes: body.nodes ?? [], edges: body.edges ?? [] }
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

// --- modeling assistant (ADR-0024) ---

// ChatMessage is one turn in the assistant conversation. Only user and assistant
// text turns are kept client-side; the server runs the tool-calling loop.
export type ChatMessage = { role: 'user' | 'assistant'; text: string }
// ChatStep records one tool the assistant ran on the way to its answer.
export type ChatStep = { tool: string; args?: unknown; result?: string; error?: boolean }
// ChatReply is the assistant's answer plus the tool steps it took and the id of
// any model it created or changed (so the modeler can reload it).
export type ChatReply = { reply: string; steps?: ChatStep[]; modelId?: string; provider?: string }

// chat sends the conversation so far to the assistant. An optional bring-your-own
// key is passed in the X-LLM-Token header (used only for that request, never
// stored server-side). provider/model override the server defaults when set.
export async function chat(
  messages: ChatMessage[],
  opts: { token?: string; provider?: string; model?: string } = {},
): Promise<ChatReply> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (opts.token) headers['X-LLM-Token'] = opts.token
  const body: Record<string, unknown> = { messages }
  if (opts.provider) body.provider = opts.provider
  if (opts.model) body.model = opts.model
  const r = await fetch('/v1/chat', { method: 'POST', headers, body: JSON.stringify(body) })
  if (!r.ok) throw new Error(await problemMessage(r, 'Assistent-Anfrage fehlgeschlagen'))
  return (await r.json()) as ChatReply
}

// ContextView mirrors dmn.ContextView: a decision's boxed-context logic — named
// literal entries plus an optional result-cell expression. simple=false when an
// entry is a nested boxed expression this text editor cannot represent.
export type ContextEntryView = { name: string; text: string; typeRef?: string }
export type ContextView = {
  decisionId: string
  name: string
  entries: ContextEntryView[]
  result?: string
  resultTypeRef?: string
  simple: boolean
}
export type ContextEdit = { entries: ContextEntryView[]; result?: string; resultTypeRef?: string }

// getContext fetches a decision's boxed-context view, or null when the decision
// has no boxed-context logic (HTTP 404).
export async function getContext(modelId: string, decision: string): Promise<ContextView | null> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/decisions/' + encodeURIComponent(decision) + '/context')
  if (r.status === 404) return null
  if (!r.ok) throw new Error('Boxed Context laden fehlgeschlagen (HTTP ' + r.status + ')')
  const cv = (await r.json()) as ContextView
  cv.entries = cv.entries ?? []
  return cv
}

// saveContext replaces a decision's boxed-context entries (POST), recompiles the
// model and returns the saved detail with its new id and any compile diagnostics.
export async function saveContext(modelId: string, decision: string, edit: ContextEdit): Promise<ModelDetail> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/decisions/' + encodeURIComponent(decision) + '/context', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(edit),
  })
  if (!r.ok) throw new Error(await problemMessage(r, 'Boxed Context speichern fehlgeschlagen'))
  return (await r.json()) as ModelDetail
}

// createBoxedContext gives an undecided decision a fresh boxed context and
// returns the saved model's detail with its new id.
export async function createBoxedContext(modelId: string, decision: string): Promise<ModelDetail> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/decisions/' + encodeURIComponent(decision) + '/create-context', { method: 'POST' })
  if (!r.ok) throw new Error(await problemMessage(r, 'Boxed Context anlegen fehlgeschlagen'))
  return (await r.json()) as ModelDetail
}

// ConditionalView mirrors dmn.ConditionalView: a decision's boxed-conditional
// logic — the three FEEL branches of an if/then/else. simple is false when a
// branch is a nested boxed expression, so the editor opens read-only.
export type ConditionalView = { decisionId: string; name: string; if: string; then: string; else: string; simple: boolean }
// ConditionalEdit is the editable payload: the three FEEL branches.
export type ConditionalEdit = { if: string; then: string; else: string }

// getConditional fetches a decision's boxed-conditional view, or null when the
// decision has no conditional logic (HTTP 404).
export async function getConditional(modelId: string, decision: string): Promise<ConditionalView | null> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/decisions/' + encodeURIComponent(decision) + '/conditional')
  if (r.status === 404) return null
  if (!r.ok) throw new Error('Conditional laden fehlgeschlagen (HTTP ' + r.status + ')')
  return (await r.json()) as ConditionalView
}

// saveConditional replaces a decision's if/then/else branches (POST), recompiles
// the model and returns the saved detail with its new id and any diagnostics.
export async function saveConditional(modelId: string, decision: string, edit: ConditionalEdit): Promise<ModelDetail> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/decisions/' + encodeURIComponent(decision) + '/conditional', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(edit),
  })
  if (!r.ok) throw new Error(await problemMessage(r, 'Conditional speichern fehlgeschlagen'))
  return (await r.json()) as ModelDetail
}

// createBoxedConditional gives an undecided decision a fresh boxed conditional and
// returns the saved model's detail with its new id.
export async function createBoxedConditional(modelId: string, decision: string): Promise<ModelDetail> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/decisions/' + encodeURIComponent(decision) + '/create-conditional', { method: 'POST' })
  if (!r.ok) throw new Error(await problemMessage(r, 'Conditional anlegen fehlgeschlagen'))
  return (await r.json()) as ModelDetail
}

// ListView mirrors dmn.ListView: a decision's boxed-list logic — its ordered FEEL
// items. simple is false when an item is a nested boxed expression, so the editor
// opens read-only.
export type ListView = { decisionId: string; name: string; items: string[]; simple: boolean }
// ListEdit is the editable payload: the ordered FEEL items.
export type ListEdit = { items: string[] }

// getList fetches a decision's boxed-list view, or null when the decision has no
// list logic (HTTP 404).
export async function getList(modelId: string, decision: string): Promise<ListView | null> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/decisions/' + encodeURIComponent(decision) + '/list')
  if (r.status === 404) return null
  if (!r.ok) throw new Error('Liste laden fehlgeschlagen (HTTP ' + r.status + ')')
  const lv = (await r.json()) as ListView
  lv.items = lv.items ?? []
  return lv
}

// saveList replaces a decision's list items (POST), recompiles the model and
// returns the saved detail with its new id and any diagnostics.
export async function saveList(modelId: string, decision: string, edit: ListEdit): Promise<ModelDetail> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/decisions/' + encodeURIComponent(decision) + '/list', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(edit),
  })
  if (!r.ok) throw new Error(await problemMessage(r, 'Liste speichern fehlgeschlagen'))
  return (await r.json()) as ModelDetail
}

// createBoxedList gives an undecided decision a fresh boxed list and returns the
// saved model's detail with its new id.
export async function createBoxedList(modelId: string, decision: string): Promise<ModelDetail> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/decisions/' + encodeURIComponent(decision) + '/create-list', { method: 'POST' })
  if (!r.ok) throw new Error(await problemMessage(r, 'Liste anlegen fehlgeschlagen'))
  return (await r.json()) as ModelDetail
}

// RelationView mirrors dmn.RelationView: a decision's boxed-relation logic — named
// columns and rows of FEEL cells. simple is false when a cell is a nested boxed
// expression, so the editor opens read-only.
export type RelationView = { decisionId: string; name: string; columns: string[]; rows: string[][]; simple: boolean }
// RelationEdit is the editable payload: the column names and rows of FEEL cells.
export type RelationEdit = { columns: string[]; rows: string[][] }

// getRelation fetches a decision's boxed-relation view, or null when the decision
// has no relation logic (HTTP 404).
export async function getRelation(modelId: string, decision: string): Promise<RelationView | null> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/decisions/' + encodeURIComponent(decision) + '/relation')
  if (r.status === 404) return null
  if (!r.ok) throw new Error('Relation laden fehlgeschlagen (HTTP ' + r.status + ')')
  const rv = (await r.json()) as RelationView
  rv.columns = rv.columns ?? []
  rv.rows = rv.rows ?? []
  return rv
}

// saveRelation replaces a decision's relation columns and rows (POST), recompiles
// the model and returns the saved detail with its new id and any diagnostics.
export async function saveRelation(modelId: string, decision: string, edit: RelationEdit): Promise<ModelDetail> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/decisions/' + encodeURIComponent(decision) + '/relation', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(edit),
  })
  if (!r.ok) throw new Error(await problemMessage(r, 'Relation speichern fehlgeschlagen'))
  return (await r.json()) as ModelDetail
}

// createBoxedRelation gives an undecided decision a fresh boxed relation and
// returns the saved model's detail with its new id.
export async function createBoxedRelation(modelId: string, decision: string): Promise<ModelDetail> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/decisions/' + encodeURIComponent(decision) + '/create-relation', { method: 'POST' })
  if (!r.ok) throw new Error(await problemMessage(r, 'Relation anlegen fehlgeschlagen'))
  return (await r.json()) as ModelDetail
}

// FilterView mirrors dmn.FilterView: a decision's boxed-filter logic — the
// collection (in) and predicate (match, evaluated per element with `item`
// bound). simple is false when a branch is a nested boxed expression, so the
// editor opens read-only.
export type FilterView = { decisionId: string; name: string; in: string; match: string; simple: boolean }
// FilterEdit is the editable payload: the two FEEL branches.
export type FilterEdit = { in: string; match: string }

// getFilter fetches a decision's boxed-filter view, or null when the decision has
// no filter logic (HTTP 404).
export async function getFilter(modelId: string, decision: string): Promise<FilterView | null> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/decisions/' + encodeURIComponent(decision) + '/filter')
  if (r.status === 404) return null
  if (!r.ok) throw new Error('Filter laden fehlgeschlagen (HTTP ' + r.status + ')')
  return (await r.json()) as FilterView
}

// saveFilter replaces a decision's in/match branches (POST), recompiles the model
// and returns the saved detail with its new id and any diagnostics.
export async function saveFilter(modelId: string, decision: string, edit: FilterEdit): Promise<ModelDetail> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/decisions/' + encodeURIComponent(decision) + '/filter', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(edit),
  })
  if (!r.ok) throw new Error(await problemMessage(r, 'Filter speichern fehlgeschlagen'))
  return (await r.json()) as ModelDetail
}

// createBoxedFilter gives an undecided decision a fresh boxed filter and returns
// the saved model's detail with its new id.
export async function createBoxedFilter(modelId: string, decision: string): Promise<ModelDetail> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/decisions/' + encodeURIComponent(decision) + '/create-filter', { method: 'POST' })
  if (!r.ok) throw new Error(await problemMessage(r, 'Filter anlegen fehlgeschlagen'))
  return (await r.json()) as ModelDetail
}

// IteratorView mirrors dmn.IteratorView: a decision's boxed-iteration logic — a
// for (yields a list via return) or a some/every quantifier (yields a boolean via
// satisfies). The variable is bound in the body while `in` is iterated. simple is
// false when a branch is a nested boxed expression, so the editor opens read-only.
export type IteratorView = { decisionId: string; name: string; kind: 'for' | 'some' | 'every'; variable: string; in: string; body: string; simple: boolean }
// IteratorEdit is the editable payload: kind, iterator variable, collection, body.
export type IteratorEdit = { kind: string; variable: string; in: string; body: string }

// getIterator fetches a decision's boxed-iteration view, or null when the decision
// has no for/some/every logic (HTTP 404).
export async function getIterator(modelId: string, decision: string): Promise<IteratorView | null> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/decisions/' + encodeURIComponent(decision) + '/iterator')
  if (r.status === 404) return null
  if (!r.ok) throw new Error('Iteration laden fehlgeschlagen (HTTP ' + r.status + ')')
  return (await r.json()) as IteratorView
}

// saveIterator replaces a decision's iteration (POST), recompiles the model and
// returns the saved detail with its new id and any diagnostics.
export async function saveIterator(modelId: string, decision: string, edit: IteratorEdit): Promise<ModelDetail> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/decisions/' + encodeURIComponent(decision) + '/iterator', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(edit),
  })
  if (!r.ok) throw new Error(await problemMessage(r, 'Iteration speichern fehlgeschlagen'))
  return (await r.json()) as ModelDetail
}

// createBoxedIterator gives an undecided decision a fresh boxed iteration and
// returns the saved model's detail with its new id.
export async function createBoxedIterator(modelId: string, decision: string): Promise<ModelDetail> {
  const r = await fetch('/v1/models/' + encodeURIComponent(modelId) + '/decisions/' + encodeURIComponent(decision) + '/create-iterator', { method: 'POST' })
  if (!r.ok) throw new Error(await problemMessage(r, 'Iteration anlegen fehlgeschlagen'))
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
