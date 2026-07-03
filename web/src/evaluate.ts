import { evaluateGraph, listTypes, InputValidationError, type ModelDetail, type InputField, type ItemType, type Trace, type TableTrace, type GraphEvalResult } from './api'
import { buildFieldControl, coerce } from './inputform'
import { attachJsonEditor } from './json-editor'

// coerce is re-exported here so its existing importers (the Import cockpit) keep
// their `from './evaluate'` path while the definition lives in inputform.ts, the
// one place the panel and the on-canvas input pills share.
export { coerce }

// EvalRun is one whole-graph evaluation: the inputs the user supplied and the
// result (per-decision values + traces). The Operate view keeps a session list
// of these to replay and highlight.
export type EvalRun = { inputs: Record<string, unknown>; result: GraphEvalResult }

// renderEvaluatePanel runs a whole-graph evaluation in the own modeler: the user
// fills the model's leaf inputs once and sees EVERY decision's result — the
// entire decision requirements graph computed from one set of inputs, not one
// decision at a time (ADR-0016). onRun, when given, receives each successful run
// (inputs + result) so the caller can overlay results on the canvas and record a
// session history (Operate).
export function renderEvaluatePanel(host: HTMLElement, model: ModelDetail, onRun?: (run: EvalRun) => void): void {
  host.textContent = ''
  const decisions = model.decisions ?? []
  if (!decisions.length) {
    host.append(el('p', { class: 'eval-empty' }, 'Dieses Modell hat keine auswertbare Decision.'))
    return
  }

  const runBtn = el('button', { id: 'evalRun', class: 'tbtn', type: 'button' }, 'Auswerten') as HTMLButtonElement
  const inputsHost = el('div', { id: 'evalInputs', class: 'eval-inputs' })
  const result = el('div', { id: 'evalResult', class: 'eval-result' })

  host.append(
    el('div', { class: 'eval-row' }, el('span', { class: 'eval-lead' }, 'Eingaben füllen und den ganzen Graphen auswerten'), runBtn),
    el('p', { class: 'eval-note' }, 'Werte werden als JSON eingegeben: Zahlen wie 12000, Text wie Nord, Listen und Objekte in [ … ] bzw. { … }. Jedes Feld zeigt sein erwartetes Format — mit „Beispiel einfügen" bekommst du ein passendes Gerüst zum Anpassen.'),
    inputsHost,
    result,
  )

  // The form asks for the model's leaf inputs — the union of every decision's
  // declared inputs, so a transitively-reached input (one only a downstream
  // decision names) is still offered. Types/constraints come along for the ride.
  const fields = leafInputs(model)
  const rows: { field: InputField; input: HTMLInputElement | HTMLSelectElement; wrap: HTMLElement }[] = fields.map((field, idx) => {
    // The panel and the on-canvas input pills build their widgets from one shared
    // factory (inputform.ts): a <select> for a closed enumeration, else a JSON-
    // coerced text box with a datalist of inferred values and the deluxe JSON editor.
    const { input, extras } = buildFieldControl(field, { idx, className: 'eval-field' })
    const label = el('label', { class: 'eval-field-label' }, field.name + (field.required ? ' *' : ''))
    const wrap = el('div', { class: 'eval-field-wrap' }, label, input, ...extras)
    inputsHost.append(wrap)
    // A free-text field coerces its value through JSON.parse, so offer the deluxe
    // JSON editor beside it (it wraps the input in place, hence after it is in the
    // DOM). Closed enumerations (a <select>) take only declared values, no opener.
    if (input instanceof HTMLInputElement) attachJsonEditor(input, { title: 'JSON — ' + field.name })
    return { field, input, wrap }
  })
  if (!rows.length) inputsHost.append(el('p', { class: 'eval-empty' }, 'Dieses Modell braucht keine Eingaben.'))

  // Enrich each field with a format hint (and, for structured/list inputs, a
  // one-click example skeleton) once the model's named types are known — so a
  // field typed „tDriverList" tells the user it wants a list of objects with
  // named fields, instead of leaving them to guess. Best-effort: the form is
  // fully usable even if the types can't be fetched.
  void addFieldGuidance(model.modelId, rows)

  // mark flags a field by name with a message (or clears all when name is null).
  const mark = (name: string | null, message?: string): void => {
    for (const { field, input: box } of rows) {
      const hit = name !== null && field.name === name
      box.classList.toggle('eval-field-invalid', hit)
      if (hit) box.title = message ?? 'ungültig'
      else if (name === null) box.title = field.constraint ? 'erlaubte Werte: ' + field.constraint : ''
    }
  }

  const run = async (): Promise<void> => {
    mark(null)
    const input: Record<string, unknown> = {}
    let missing: string | null = null
    for (const { field, input: box } of rows) {
      const v = coerce(box.value)
      if (v === undefined) {
        if (field.required && missing === null) missing = field.name
        continue
      }
      input[field.name] = v
    }
    if (missing !== null) {
      mark(missing, 'Pflichtfeld')
      result.className = 'eval-result eval-error'
      result.textContent = 'Bitte das Pflichtfeld „' + missing + '" ausfüllen.'
      return
    }
    runBtn.disabled = true
    result.textContent = 'wertet aus …'
    result.className = 'eval-result'
    try {
      // strict: validate inputs against the model's whole-graph schema; explain:
      // get each decision's trace (which rule hit).
      const res = await evaluateGraph(model.modelId, input, true, true)
      showResults(result, decisions, res.values, res.errors)
      showTraces(result, res.traces)
      onRun?.({ inputs: input, result: res })
    } catch (e) {
      result.className = 'eval-result eval-error'
      if (e instanceof InputValidationError) {
        const first = e.problems[0]
        mark(first.input, first.message)
        result.textContent = e.problems.map((p) => p.input + ': ' + p.message).join(' · ')
      } else {
        result.textContent = (e as Error).message
      }
    } finally {
      runBtn.disabled = false
    }
  }
  runBtn.addEventListener('click', () => void run())

  // Ctrl-Enter (Cmd-Enter on macOS) from any input field triggers the whole-graph
  // evaluation — the keyboard equivalent of clicking „Auswerten", so the user can
  // fill inputs and run without reaching for the mouse.
  inputsHost.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey) && !runBtn.disabled) {
      e.preventDefault()
      void run()
    }
  })
}

// A TypeShape is what a field ultimately expects, resolved through the model's
// named types: a base FEEL kind (or '' when unknown/structured), whether it is a
// list, and — for a structured type — its component fields. It's enough both to
// describe the field in words and to synthesise a matching example value.
type TypeShape = { base: string; collection: boolean; components?: { name: string; shape: TypeShape }[] }

type Row = { field: InputField; input: HTMLInputElement | HTMLSelectElement; wrap: HTMLElement }

// addFieldGuidance fetches the model's named types and, for each free-text field,
// makes the expected value concrete: a friendlier placeholder for scalars, and
// for a structured or list input a shape description plus a „Beispiel einfügen"
// button that drops a ready-to-edit JSON skeleton into the box. Best-effort — the
// form stays fully usable if the types can't be fetched. Closed-enum fields are
// rendered as a <select> and need no guidance.
async function addFieldGuidance(modelId: string, rows: Row[]): Promise<void> {
  let types: ItemType[]
  try {
    types = await listTypes(modelId)
  } catch {
    return
  }
  const byName = new Map<string, ItemType>()
  for (const t of types) byName.set(t.name, t)
  for (const { field, input, wrap } of rows) {
    if (!(input instanceof HTMLInputElement)) continue
    const shape = resolveShape(field.type, byName, 0)
    const structured = shape.collection || (shape.components?.length ?? 0) > 0
    if (structured) {
      const example = JSON.stringify(exampleValue(shape))
      const hint = el('div', { class: 'eval-field-hint' }, describeShape(shape))
      const btn = el('button', { class: 'eval-ex-btn', type: 'button', title: example }, 'Beispiel einfügen') as HTMLButtonElement
      btn.addEventListener('click', () => {
        input.value = example
        input.focus()
      })
      hint.append(btn)
      wrap.append(hint)
      if (isTypePlaceholder(input, field)) input.placeholder = example.length <= 40 ? example : 'JSON …'
    } else if (isTypePlaceholder(input, field)) {
      input.placeholder = scalarPlaceholder(shape.base)
    }
  }
}

// isTypePlaceholder is true while the box still shows its bare type-name (or
// FEEL) placeholder — i.e. we haven't replaced it with real guidance yet.
function isTypePlaceholder(input: HTMLInputElement, field: InputField): boolean {
  return input.placeholder === (field.type ?? '') || input.placeholder === 'FEEL' || input.placeholder === ''
}

// resolveShape resolves a type reference (a base FEEL type or a named model type)
// to its shape, following named types and collection flags. depth guards against
// a type that (directly or indirectly) refers to itself.
function resolveShape(typeRef: string | undefined, byName: Map<string, ItemType>, depth: number): TypeShape {
  const scalar = canonicalScalar(typeRef)
  if (scalar) return { base: scalar, collection: false }
  const t = typeRef ? byName.get(bareName(typeRef)) : undefined
  if (!t || depth > 5) return { base: '', collection: false }
  return resolveItem(t, byName, depth)
}

function resolveItem(t: ItemType, byName: Map<string, ItemType>, depth: number): TypeShape {
  const collection = t.isCollection === true
  if (t.components?.length) {
    return { base: '', collection, components: t.components.map((c) => ({ name: c.name, shape: resolveItem(c, byName, depth + 1) })) }
  }
  // No inline components → an alias to another (possibly named) type: inherit its
  // shape and OR in this level's collection flag (e.g. a list-of-numbers alias).
  const inner = resolveShape(t.typeRef, byName, depth + 1)
  return { base: inner.base, collection: collection || inner.collection, components: inner.components }
}

// exampleValue builds a minimal, correctly-shaped sample for a type: a list wraps
// one element, a structured type becomes an object with every field, and a scalar
// gets a neutral literal — a skeleton the user edits rather than invents.
function exampleValue(shape: TypeShape): unknown {
  if (shape.collection) return [exampleValue({ base: shape.base, collection: false, components: shape.components })]
  if (shape.components?.length) {
    const o: Record<string, unknown> = {}
    for (const c of shape.components) o[c.name] = exampleValue(c.shape)
    return o
  }
  switch (shape.base) {
    case 'number':
      return 0
    case 'boolean':
      return true
    case 'date':
      return '2026-01-01'
    case 'time':
      return '09:00:00'
    case 'date and time':
      return '2026-01-01T09:00:00'
    case 'duration':
      return 'P1D'
    default:
      return 'Text'
  }
}

// describeShape puts a field's expected shape into a short German phrase, e.g.
// „Liste von Objekten mit age (Zahl), points (Zahl)".
function describeShape(shape: TypeShape): string {
  if (shape.components?.length) {
    const fields = shape.components.map((c) => c.name + ' (' + shortType(c.shape) + ')').join(', ')
    return (shape.collection ? 'Liste von Objekten mit ' : 'Objekt mit ') + fields
  }
  if (shape.collection) return 'Liste von ' + pluralType(shape.base)
  return shortType(shape)
}

function shortType(shape: TypeShape): string {
  if (shape.components?.length) return 'Objekt'
  if (shape.collection) return 'Liste'
  switch (shape.base) {
    case 'number':
      return 'Zahl'
    case 'boolean':
      return 'true/false'
    case 'string':
      return 'Text'
    case 'date':
      return 'Datum'
    case 'time':
      return 'Uhrzeit'
    case 'date and time':
      return 'Zeitpunkt'
    case 'duration':
      return 'Dauer'
    default:
      return 'Text'
  }
}

function pluralType(base: string): string {
  switch (base) {
    case 'number':
      return 'Zahlen'
    case 'boolean':
      return 'true/false-Werten'
    case 'string':
      return 'Texten'
    case 'date':
      return 'Daten'
    default:
      return 'Werten'
  }
}

function scalarPlaceholder(base: string): string {
  switch (base) {
    case 'number':
      return 'Zahl, z. B. 12000'
    case 'boolean':
      return 'true / false'
    case 'date':
      return 'JJJJ-MM-TT'
    case 'time':
      return 'HH:MM:SS'
    case 'date and time':
      return 'JJJJ-MM-TTThh:mm:ss'
    case 'duration':
      return 'z. B. P1D'
    default:
      return 'Text'
  }
}

// bareName strips a namespace prefix (feel:number → number) and trims.
function bareName(ref: string): string {
  const i = ref.lastIndexOf(':')
  return (i >= 0 ? ref.slice(i + 1) : ref).trim()
}

// canonicalScalar maps a type reference to a canonical FEEL scalar name, or ''
// when it is not a built-in scalar (mirrors the engine's canonicalType).
function canonicalScalar(ref: string | undefined): string {
  if (!ref) return ''
  switch (bareName(ref).toLowerCase().replace(/\s+/g, '')) {
    case 'number':
      return 'number'
    case 'string':
      return 'string'
    case 'boolean':
      return 'boolean'
    case 'date':
      return 'date'
    case 'time':
      return 'time'
    case 'datetime':
    case 'dateandtime':
      return 'date and time'
    case 'duration':
    case 'daytimeduration':
    case 'yearmonthduration':
    case 'daysandtimeduration':
    case 'yearsandmonthsduration':
      return 'duration'
    default:
      return ''
  }
}

// leafInputs unions every decision's declared inputs into the model's leaf-input
// list (deduped by name; type/constraint from the first that declares one;
// required when any decision requires it). Falls back to the bare input names.
// Exported so the Import cockpit can build a matching CSV/JSON test template
// from the very same authoritative leaf-input set the evaluate form uses.
export function leafInputs(model: ModelDetail): InputField[] {
  const schema = model.schema
  if (schema) {
    const byName = new Map<string, InputField>()
    for (const fields of Object.values(schema)) {
      // A decision with no direct inputs serialises as null (Go nil slice).
      for (const f of fields ?? []) {
        const ex = byName.get(f.name)
        if (!ex) {
          byName.set(f.name, { ...f, values: f.values ? [...f.values] : undefined })
        } else {
          if (!ex.type && f.type) ex.type = f.type
          if (!ex.constraint && f.constraint) ex.constraint = f.constraint
          ex.required = ex.required || f.required
          // Values: a closed declared enumeration wins; else union the suggestions.
          if (!ex.valuesClosed) {
            if (f.valuesClosed) {
              ex.values = f.values ? [...f.values] : undefined
              ex.valuesClosed = true
            } else if (f.values?.length) {
              ex.values = [...new Set([...(ex.values ?? []), ...f.values])]
            }
          }
        }
      }
    }
    if (byName.size) return [...byName.values()]
  }
  return (model.inputs ?? []).map((name) => ({ name, required: false }))
}

// showResults renders one row per decision (in the model's declared order) with
// its computed value — the whole graph's results — flagging any that errored.
function showResults(host: HTMLElement, decisions: string[], values: Record<string, unknown>, errors?: Record<string, string>): void {
  host.textContent = ''
  host.className = 'eval-result'
  host.append(el('span', { class: 'eval-ok' }, 'Ergebnisse (ganzer Graph)'))
  const table = el('table', { class: 'eval-out' })
  for (const name of decisions) {
    const errMsg = errors?.[name]
    const cell = errMsg
      ? el('td', { class: 'eval-cell-error' }, el('code', {}, 'Fehler'), document.createTextNode(' ' + errMsg))
      : el('td', {}, el('code', {}, fmt(values[name])))
    table.append(el('tr', {}, el('th', {}, name), cell))
  }
  host.append(table)
}

// showTraces renders, per decision that produced a decision-table trace, the
// tested input values and every rule — the matched rule(s) highlighted, so the
// user sees which rule hit and why, for the whole graph at once.
function showTraces(host: HTMLElement, traces?: Record<string, Trace>): void {
  if (!traces) return
  const names = Object.keys(traces).filter((n) => (traces[n].tables ?? []).length)
  if (!names.length) return
  const wrap = el('div', { class: 'trace' })
  wrap.append(el('div', { class: 'trace-title' }, 'Begründung (welche Regel hat gehittet)'))
  for (const name of names.sort()) {
    wrap.append(el('div', { class: 'trace-decision' }, name))
    const tables = traces[name].tables ?? []
    tables.forEach((tt, i) => wrap.append(traceTable(tt, tables.length > 1 ? i + 1 : 0)))
  }
  host.append(wrap)
}

function traceTable(tt: TableTrace, n: number): HTMLElement {
  const block = el('div', { class: 'trace-block' })
  const matched = tt.matched ?? []
  const policy = tt.hitPolicy + (tt.aggregation ? ' ' + tt.aggregation : '')
  const head = matched.length
    ? `Regel ${matched.map((m) => m + 1).join(', ')} hat gehittet`
    : 'keine Regel hat gehittet'
  block.append(el('div', { class: 'trace-head' }, (n ? `Tabelle ${n} · ` : '') + head, el('span', { class: 'trace-policy' }, policy)))

  const table = el('table', { class: 'trace-grid' })
  // Header: input columns (with the value each tested) + output.
  const hr = el('tr', {}, el('th', { class: 'trace-idx' }, '#'))
  for (const in_ of tt.inputs ?? []) hr.append(el('th', {}, el('div', { class: 'trace-col' }, el('span', {}, in_.expression), el('code', { class: 'trace-val' }, '= ' + fmt(in_.value)))))
  hr.append(el('th', {}, 'Output'))
  table.append(hr)

  const nIn = (tt.inputs ?? []).length
  for (const r of tt.rules ?? []) {
    const row = el('tr', { class: r.matched ? 'trace-rule trace-hit' : 'trace-rule' }, el('td', { class: 'trace-idx' }, String(r.index + 1)))
    for (let k = 0; k < nIn; k++) {
      const cond = r.conditions?.[k]
      const cls = cond ? (cond.matched ? 'trace-ok' : 'trace-no') : 'trace-skip'
      row.append(el('td', { class: cls }, cond ? cell(cond.entry) : ''))
    }
    row.append(el('td', { class: 'trace-out' }, r.matched && r.outputs ? r.outputs.map(fmt).join(', ') : ''))
    table.append(row)
  }
  block.append(table)
  return block
}

function cell(text: string | undefined): string {
  const s = (text ?? '').trim()
  return s === '' || s === '-' ? '–' : s
}

function fmt(v: unknown): string {
  if (v === null || v === undefined) return 'null'
  if (typeof v === 'string') return v
  return JSON.stringify(v)
}

// el is a tiny DOM builder: tag, attributes, then string/Node children.
function el(tag: string, attrs: Record<string, string> = {}, ...children: (string | Node)[]): HTMLElement {
  const node = document.createElement(tag)
  for (const [k, v] of Object.entries(attrs)) {
    if (v !== '') node.setAttribute(k, v)
  }
  node.append(...children)
  return node
}
