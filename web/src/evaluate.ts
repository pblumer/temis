import { evaluateGraph, InputValidationError, type ModelDetail, type InputField, type Trace, type TableTrace, type GraphEvalResult } from './api'

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
    inputsHost,
    result,
  )

  // The form asks for the model's leaf inputs — the union of every decision's
  // declared inputs, so a transitively-reached input (one only a downstream
  // decision names) is still offered. Types/constraints come along for the ride.
  const fields = leafInputs(model)
  const rows: { field: InputField; input: HTMLInputElement | HTMLSelectElement }[] = fields.map((field, idx) => {
    const values = field.values ?? []
    let input: HTMLInputElement | HTMLSelectElement
    const extras: Node[] = []
    if (values.length && field.valuesClosed) {
      // Closed enumeration → a dropdown; only declared values are accepted.
      const sel = el('select', { class: 'eval-field' }) as HTMLSelectElement
      const blank = el('option', {}, '— wählen —') as HTMLOptionElement
      blank.value = ''
      sel.append(blank)
      for (const v of values) sel.append(el('option', { value: v }, v))
      input = sel
    } else {
      // Free text, with the inferred cell values offered as suggestions.
      const box = el('input', {
        type: 'text',
        class: 'eval-field',
        placeholder: field.type || 'FEEL',
        title: field.constraint ? 'erlaubte Werte: ' + field.constraint : '',
      }) as HTMLInputElement
      if (values.length) {
        const id = 'eval-dl-' + idx
        const dl = el('datalist', { id })
        for (const v of values) dl.append(el('option', { value: v }))
        box.setAttribute('list', id)
        extras.push(dl)
      }
      input = box
    }
    const label = el('label', { class: 'eval-field-label' }, field.name + (field.required ? ' *' : ''))
    inputsHost.append(el('div', { class: 'eval-field-wrap' }, label, input, ...extras))
    return { field, input }
  })
  if (!rows.length) inputsHost.append(el('p', { class: 'eval-empty' }, 'Dieses Modell braucht keine Eingaben.'))

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

// coerce turns a raw form value into a JSON value: an empty box contributes
// nothing (undefined); otherwise try JSON (numbers, booleans, null, lists,
// objects) and fall back to the raw string for bare FEEL text like "2024-01-01".
// Exported so the Import cockpit coerces imported CSV cells the same way the
// evaluate form coerces typed input, keeping one parsing rule across the UI.
export function coerce(raw: string): unknown {
  const s = raw.trim()
  if (s === '') return undefined
  try {
    return JSON.parse(s)
  } catch {
    return raw
  }
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
