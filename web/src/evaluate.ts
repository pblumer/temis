import { evaluate, type ModelDetail, type InputField } from './api'

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
      const res = await evaluate(model.modelId, decisionSel.value, input)
      showResult(result, res.outputs)
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
