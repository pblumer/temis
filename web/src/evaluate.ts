import { evaluate, type ModelDetail, type InputField, type Trace, type TableTrace } from './api'

// renderEvaluatePanel brings the legacy /ui evaluate workflow into the own
// modeler (feature parity before the root cutover, ADR-0016 WP-67): pick a
// decision, fill its typed inputs and run it against the engine. The panel is
// self-contained and re-rendered whenever the shown model changes.
export function renderEvaluatePanel(host: HTMLElement, model: ModelDetail): void {
  host.textContent = ''
  const decisions = model.decisions ?? []
  if (!decisions.length) {
    host.append(el('p', { class: 'eval-empty' }, 'Dieses Modell hat keine auswertbare Decision.'))
    return
  }

  const decisionSel = el('select', { id: 'evalDecision' }) as HTMLSelectElement
  for (const d of decisions) decisionSel.append(el('option', { value: d }, d))
  const runBtn = el('button', { id: 'evalRun', class: 'tbtn', type: 'button' }, 'Auswerten') as HTMLButtonElement

  const inputsHost = el('div', { id: 'evalInputs', class: 'eval-inputs' })
  const result = el('div', { id: 'evalResult', class: 'eval-result' })

  host.append(
    el('div', { class: 'eval-row' }, el('label', { for: 'evalDecision' }, 'Decision'), decisionSel, runBtn),
    inputsHost,
    result,
  )

  // fieldsFor returns the typed input fields the selected decision needs: its
  // schema entry when present, else the model's inputs as untyped fields.
  const fieldsFor = (decision: string): InputField[] => {
    const schema = model.schema?.[decision]
    if (schema && schema.length) return schema
    return (model.inputs ?? []).map((name) => ({ name, required: false }))
  }

  let rows: { field: InputField; input: HTMLInputElement }[] = []
  const renderInputs = (): void => {
    inputsHost.textContent = ''
    result.textContent = ''
    rows = fieldsFor(decisionSel.value).map((field) => {
      const input = el('input', {
        type: 'text',
        class: 'eval-field',
        placeholder: field.type || 'FEEL',
        title: field.constraint ? 'erlaubte Werte: ' + field.constraint : '',
      }) as HTMLInputElement
      const label = el('label', { class: 'eval-field-label' }, field.name + (field.required ? ' *' : ''))
      inputsHost.append(el('div', { class: 'eval-field-wrap' }, label, input))
      return { field, input }
    })
    if (!rows.length) inputsHost.append(el('p', { class: 'eval-empty' }, 'Diese Decision braucht keine Eingaben.'))
  }
  decisionSel.addEventListener('change', renderInputs)
  renderInputs()

  const run = async (): Promise<void> => {
    const input: Record<string, unknown> = {}
    for (const { field, input: box } of rows) {
      const v = coerce(box.value)
      if (v !== undefined) input[field.name] = v
    }
    runBtn.disabled = true
    result.textContent = 'wertet aus …'
    result.className = 'eval-result'
    try {
      const res = await evaluate(model.modelId, decisionSel.value, input, true)
      showResult(result, res.outputs)
      if (res.trace) showTrace(result, res.trace)
    } catch (e) {
      result.className = 'eval-result eval-error'
      result.textContent = (e as Error).message
    } finally {
      runBtn.disabled = false
    }
  }
  runBtn.addEventListener('click', () => void run())
}

// coerce turns a raw form value into a JSON value: an empty box contributes
// nothing (undefined); otherwise try JSON (numbers, booleans, null, lists,
// objects) and fall back to the raw string for bare FEEL text like "2024-01-01".
function coerce(raw: string): unknown {
  const s = raw.trim()
  if (s === '') return undefined
  try {
    return JSON.parse(s)
  } catch {
    return raw
  }
}

// showResult renders the decision outputs as a small key/value table, or the raw
// value when the output is scalar.
function showResult(host: HTMLElement, outputs: Record<string, unknown>): void {
  host.textContent = ''
  host.className = 'eval-result'
  const keys = Object.keys(outputs ?? {})
  if (!keys.length) {
    host.append(el('span', { class: 'eval-ok' }, 'Ergebnis: '), el('code', {}, 'null'))
    return
  }
  const table = el('table', { class: 'eval-out' })
  for (const k of keys) {
    table.append(el('tr', {}, el('th', {}, k), el('td', {}, el('code', {}, fmt(outputs[k])))))
  }
  host.append(el('span', { class: 'eval-ok' }, 'Ergebnis'), table)
}

// showTrace renders the evaluation trace below the outputs: one block per
// decision table, with the tested input values and every rule — the matched
// rule(s) highlighted, so the user sees which rule hit and why.
function showTrace(host: HTMLElement, trace: Trace): void {
  const tables = trace.tables ?? []
  if (!tables.length) return
  const wrap = el('div', { class: 'trace' })
  wrap.append(el('div', { class: 'trace-title' }, 'Begründung (welche Regel hat gehittet)'))
  tables.forEach((tt, i) => wrap.append(traceTable(tt, tables.length > 1 ? i + 1 : 0)))
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
