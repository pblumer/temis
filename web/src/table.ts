import { getTable, saveTable, type TableView, type TableInput, type TableOutput, type TableRule, type TableEdit } from './api'
import { ensureFeel, validateExpr, validateUnary, validateName } from './feel'

// Hit policies offered in the editor (single-letter DMN codes) and the Collect
// aggregations.
const HIT_POLICIES: [string, string][] = [
  ['U', 'Unique'], ['A', 'Any'], ['P', 'Priority'], ['F', 'First'], ['R', 'Rule order'], ['C', 'Collect'],
]
const AGGREGATIONS = ['', 'SUM', 'COUNT', 'MIN', 'MAX']
const FEEL_TYPES = ['', 'string', 'number', 'boolean', 'date', 'time', 'date and time', 'days and time duration', 'years and months duration']

// openTableOverlay fetches a decision's decision-table and shows it in a fully
// editable modal (ADR-0016): hit policy, input/output columns (add/remove, edit
// expression/name/type) and rule cells, all FEEL-validated, then saved back into
// the model. onSaved gets the saved model's new id (its content hash changed).
export async function openTableOverlay(modelId: string, decisionId: string, onSaved?: (newModelId: string) => void): Promise<void> {
  let fetched: TableView | null
  try {
    fetched = await getTable(modelId, decisionId)
  } catch (e) {
    console.error(e)
    return
  }
  if (!fetched) return
  void ensureFeel()

  // Mutable working copy of the table; structural edits rebuild the grid from it.
  const state: TableView = {
    decisionId: fetched.decisionId,
    name: fetched.name,
    hitPolicy: fetched.hitPolicy || 'U',
    aggregation: fetched.aggregation ?? '',
    inputs: fetched.inputs.map((c) => ({ ...c })),
    outputs: fetched.outputs.length ? fetched.outputs.map((c) => ({ ...c })) : [{ name: fetched.name }],
    rules: fetched.rules.map((r) => ({ inputEntries: [...r.inputEntries], outputEntries: [...r.outputEntries], annotations: [...(r.annotations ?? [])] })),
  }

  const close = (): void => {
    overlay.remove()
    document.removeEventListener('keydown', onKey)
  }
  const onKey = (e: KeyboardEvent): void => {
    if (e.key === 'Escape' && !overlay.querySelector('.dt-cell:focus, .dt-head-field:focus')) close()
  }
  document.addEventListener('keydown', onKey)

  const overlay = el('div', { class: 'dt-overlay' })
  overlay.addEventListener('click', (e) => {
    if (e.target === overlay) close()
  })

  // Header: title + hit-policy controls + close.
  const policySel = el('select', { class: 'dt-policy-sel', title: 'Hit Policy' }) as HTMLSelectElement
  for (const [code, label] of HIT_POLICIES) policySel.append(option(code, code + ' · ' + label, code === state.hitPolicy))
  const aggSel = el('select', { class: 'dt-agg-sel', title: 'Aggregation (Collect)' }) as HTMLSelectElement
  for (const a of AGGREGATIONS) aggSel.append(option(a, a || '— Σ —', a === state.aggregation))
  const syncAgg = (): void => {
    aggSel.style.display = state.hitPolicy === 'C' ? '' : 'none'
  }
  policySel.addEventListener('change', () => {
    state.hitPolicy = policySel.value
    syncAgg()
  })
  aggSel.addEventListener('change', () => {
    state.aggregation = aggSel.value
  })
  syncAgg()

  const closeBtn = el('button', { class: 'dt-close', type: 'button', title: 'Schließen (Esc)' }, '✕') as HTMLButtonElement
  closeBtn.addEventListener('click', close)
  const header = el('div', { class: 'dt-head' }, el('span', { class: 'dt-title' }, state.name || decisionId), policySel, aggSel, closeBtn)

  const scroll = el('div', { class: 'dt-scroll' })
  const status = el('span', { class: 'dt-status' })

  // names that a cell/output expression may reference: the input column expressions.
  const inputNames = (): string[] => state.inputs.map((c) => c.expression).filter((s) => s.trim() !== '')

  const rebuild = (): void => {
    scroll.textContent = ''
    scroll.append(buildGrid(state, inputNames(), rebuild))
  }
  rebuild()

  const addRow = (): void => {
    state.rules.push({ inputEntries: state.inputs.map(() => ''), outputEntries: state.outputs.map(() => ''), annotations: [] })
    rebuild()
  }
  const addBtn = button('+ Regel', addRow)
  const addInBtn = button('+ Input', () => {
    state.inputs.push({ expression: '', typeRef: '' })
    state.rules.forEach((r) => r.inputEntries.push(''))
    rebuild()
  })
  const addOutBtn = button('+ Output', () => {
    state.outputs.push({ name: '', typeRef: '' })
    state.rules.forEach((r) => r.outputEntries.push(''))
    rebuild()
  })
  const saveBtn = el('button', { class: 'tbtn dt-save', type: 'button' }, 'Speichern') as HTMLButtonElement

  const save = async (): Promise<void> => {
    if (scroll.querySelector('.dt-cell-invalid, .dt-head-invalid')) {
      status.className = 'dt-status dt-error'
      status.textContent = 'Bitte zuerst die rot markierten Felder korrigieren.'
      return
    }
    saveBtn.disabled = true
    status.className = 'dt-status'
    status.textContent = 'speichert …'
    const edit: TableEdit = {
      replaceColumns: true,
      hitPolicy: state.hitPolicy,
      aggregation: state.aggregation,
      inputs: state.inputs,
      outputs: state.outputs,
      rules: state.rules,
    }
    try {
      const saved = await saveTable(modelId, decisionId, edit)
      const errs = (saved.diagnostics ?? []).filter((d) => d.severity === 'error')
      if (errs.length) {
        status.className = 'dt-status dt-error'
        status.textContent = errs.map((d) => d.message).join(' · ')
        saveBtn.disabled = false
        return
      }
      onSaved?.(saved.modelId)
      close()
    } catch (e) {
      status.className = 'dt-status dt-error'
      status.textContent = (e as Error).message
      saveBtn.disabled = false
    }
  }
  saveBtn.addEventListener('click', () => void save())

  const toolbar = el('div', { class: 'dt-toolbar' }, addInBtn, addOutBtn, addBtn, saveBtn, status)
  overlay.append(el('div', { class: 'dt-modal' }, header, scroll, toolbar))
  document.body.append(overlay)
}

// buildGrid renders the editable table from the working state. rebuild is called
// after a structural change (column/row add/remove) to redraw.
function buildGrid(state: TableView, names: string[], rebuild: () => void): HTMLElement {
  const table = el('table', { class: 'dt' })
  const head = el('thead')

  // Band row.
  const band = el('tr', { class: 'dt-band' }, el('th', { class: 'dt-idx' }, ''))
  if (state.inputs.length) band.append(el('th', { class: 'dt-in', colspan: String(state.inputs.length) }, 'Input'))
  if (state.outputs.length) band.append(el('th', { class: 'dt-out', colspan: String(state.outputs.length) }, 'Output'))
  band.append(el('th', { class: 'dt-ann' }, 'Annotation'), el('th', { class: 'dt-del' }, ''))
  head.append(band)

  // Column header row: editable expression/name + type + remove.
  const cols = el('tr', { class: 'dt-cols' }, el('th', { class: 'dt-idx' }, '#'))
  state.inputs.forEach((c, k) => cols.append(inputHeader(state, c, k, names, rebuild)))
  state.outputs.forEach((c, k) => cols.append(outputHeader(state, c, k, rebuild)))
  cols.append(el('th', { class: 'dt-ann' }, ''), el('th', { class: 'dt-del' }, ''))
  head.append(cols)
  table.append(head)

  // Rule rows.
  const body = el('tbody')
  state.rules.forEach((r, i) => body.append(ruleRow(state, r, i, names, rebuild)))
  table.append(body)
  return table
}

function inputHeader(state: TableView, col: TableInput, k: number, names: string[], rebuild: () => void): HTMLElement {
  const expr = el('input', { class: 'dt-head-field', value: col.expression ?? '', placeholder: 'FEEL' }) as HTMLInputElement
  const check = (): void => {
    const s = expr.value.trim()
    col.expression = expr.value
    mark(expr, s === '' ? { ok: false, message: 'Input-Ausdruck darf nicht leer sein' } : validateExpr(s, names))
  }
  expr.addEventListener('input', check)
  check()
  return el('th', { class: 'dt-in' }, el('div', { class: 'dt-colhead' }, expr, typeSelect(col), removeBtn(() => {
    state.inputs.splice(k, 1)
    state.rules.forEach((r) => r.inputEntries.splice(k, 1))
    rebuild()
  })))
}

function outputHeader(state: TableView, col: TableOutput, k: number, rebuild: () => void): HTMLElement {
  const name = el('input', { class: 'dt-head-field', value: col.name ?? '', placeholder: 'Name' }) as HTMLInputElement
  const check = (): void => {
    const s = name.value.trim()
    col.name = name.value
    // A name is optional for a single output, but if given it must be a FEEL name.
    mark(name, s === '' ? { ok: state.outputs.length === 1 } : validateName(s))
  }
  name.addEventListener('input', check)
  check()
  // The last output cannot be removed — a decision table needs at least one.
  const rm = state.outputs.length > 1 ? removeBtn(() => {
    state.outputs.splice(k, 1)
    state.rules.forEach((r) => r.outputEntries.splice(k, 1))
    rebuild()
  }) : el('span', { class: 'dt-rm-spacer' })
  return el('th', { class: 'dt-out' }, el('div', { class: 'dt-colhead' }, name, typeSelect(col), rm))
}

function ruleRow(state: TableView, r: TableRule, i: number, names: string[], rebuild: () => void): HTMLElement {
  const row = el('tr', { class: 'dt-rule' }, el('td', { class: 'dt-idx' }, String(i + 1)))
  state.inputs.forEach((_, k) => row.append(el('td', { class: 'dt-in' }, cell(r.inputEntries, k, 'in', names))))
  state.outputs.forEach((_, k) => row.append(el('td', { class: 'dt-out' }, cell(r.outputEntries, k, 'out', names))))
  const ann = el('input', { class: 'dt-cell dt-cell-ann', value: (r.annotations ?? [])[0] ?? '', placeholder: '—' }) as HTMLInputElement
  ann.addEventListener('input', () => {
    r.annotations = ann.value.trim() ? [ann.value] : []
  })
  row.append(el('td', { class: 'dt-ann' }, ann))
  row.append(el('td', { class: 'dt-del' }, removeBtn(() => {
    state.rules.splice(i, 1)
    rebuild()
  }, '🗑')))
  return row
}

// cell renders one editable rule cell, writing back to entries[k] and validating
// (input cells are unary tests with empty=any; output cells are FEEL expressions).
function cell(entries: string[], k: number, kind: 'in' | 'out', names: string[]): HTMLInputElement {
  const box = el('input', { class: 'dt-cell dt-cell-' + kind, value: entries[k] ?? '', placeholder: kind === 'in' ? '–' : '' }) as HTMLInputElement
  const check = (): void => {
    entries[k] = box.value
    const s = box.value.trim()
    if (kind === 'in') mark(box, s === '' || s === '-' ? { ok: true } : validateUnary(s, names))
    else mark(box, s === '' ? { ok: false, message: 'Output darf nicht leer sein' } : validateExpr(s, names))
  }
  box.addEventListener('input', check)
  check()
  return box
}

function typeSelect(col: { typeRef?: string }): HTMLSelectElement {
  const sel = el('select', { class: 'dt-type-sel', title: 'Typ' }) as HTMLSelectElement
  for (const t of FEEL_TYPES) sel.append(option(t, t || '— Typ —', (col.typeRef ?? '') === t))
  sel.addEventListener('change', () => {
    col.typeRef = sel.value
  })
  return sel
}

function removeBtn(onClick: () => void, glyph = '✕'): HTMLButtonElement {
  const b = el('button', { class: 'dt-rm', type: 'button', title: 'Spalte/Regel entfernen' }, glyph) as HTMLButtonElement
  b.addEventListener('click', onClick)
  return b
}

function button(label: string, onClick: () => void): HTMLButtonElement {
  const b = el('button', { class: 'tbtn', type: 'button' }, label) as HTMLButtonElement
  b.addEventListener('click', onClick)
  return b
}

function option(value: string, label: string, selected: boolean): HTMLOptionElement {
  const o = el('option', { value }, label) as HTMLOptionElement
  o.selected = selected
  return o
}

// mark toggles a field's invalid state and shows the engine's reason as a tooltip.
function mark(box: HTMLInputElement, res: { ok: boolean; message?: string }): void {
  const invalid = box.classList.contains('dt-cell') ? 'dt-cell-invalid' : 'dt-head-invalid'
  box.classList.toggle(invalid, !res.ok)
  box.title = res.ok ? '' : res.message ?? 'ungültig'
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
