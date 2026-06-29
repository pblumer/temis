import { getTable, saveTable, type TableView, type TableRule } from './api'
import { ensureFeel, validateExpr, validateUnary } from './feel'

// openTableOverlay fetches a decision's decision-table and shows it in an
// editable modal overlay (ADR-0016): edit cells, add/delete rules, with live FEEL
// validation, then save back into the model. Columns and hit policy are kept; the
// table's rows are rewritten. onSaved is called with the saved model's new id
// (its content hash changed) so the caller can switch to it.
export async function openTableOverlay(modelId: string, decisionId: string, onSaved?: (newModelId: string) => void): Promise<void> {
  let table: TableView | null
  try {
    table = await getTable(modelId, decisionId)
  } catch (e) {
    console.error(e)
    return
  }
  if (!table) return
  const t = table
  void ensureFeel() // load the validator in the background

  // Input variable names a cell may reference (the input column expressions).
  const names = t.inputs.map((c) => c.expression).filter((s) => s.trim() !== '')

  const close = (): void => {
    overlay.remove()
    document.removeEventListener('keydown', onKey)
  }
  const onKey = (e: KeyboardEvent): void => {
    if (e.key === 'Escape') close()
  }
  document.addEventListener('keydown', onKey)

  const overlay = el('div', { class: 'dt-overlay' })
  overlay.addEventListener('click', (e) => {
    if (e.target === overlay) close()
  })

  const policy = t.hitPolicy + (t.aggregation ? ' ' + t.aggregation : '')
  const closeBtn = el('button', { class: 'dt-close', type: 'button', title: 'Schließen (Esc)' }, '✕') as HTMLButtonElement
  closeBtn.addEventListener('click', close)
  const header = el(
    'div',
    { class: 'dt-head' },
    el('span', { class: 'dt-title' }, t.name || decisionId),
    el('span', { class: 'dt-policy', title: 'Hit Policy' }, policy),
    closeBtn,
  )

  const tbody = el('tbody')
  const grid = el('table', { class: 'dt' }, buildHead(t), tbody)
  for (const r of t.rules) tbody.append(buildRow(t, names, r))

  const status = el('span', { class: 'dt-status' })
  const addBtn = el('button', { class: 'tbtn', type: 'button' }, '+ Regel') as HTMLButtonElement
  addBtn.addEventListener('click', () => {
    tbody.append(buildRow(t, names, emptyRule(t)))
    renumber(tbody)
  })
  const saveBtn = el('button', { class: 'tbtn dt-save', type: 'button' }, 'Speichern') as HTMLButtonElement

  const save = async (): Promise<void> => {
    const rules = collectRules(tbody, t)
    // Block on any client-side FEEL error (empty inputs are the "any" match).
    if (tbody.querySelector('.dt-cell-invalid')) {
      status.className = 'dt-status dt-error'
      status.textContent = 'Bitte zuerst die rot markierten Zellen korrigieren.'
      return
    }
    saveBtn.disabled = true
    status.className = 'dt-status'
    status.textContent = 'speichert …'
    try {
      const saved = await saveTable(modelId, decisionId, { rules })
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

  const toolbar = el('div', { class: 'dt-toolbar' }, addBtn, saveBtn, status)
  overlay.append(el('div', { class: 'dt-modal' }, header, el('div', { class: 'dt-scroll' }, grid), toolbar))
  document.body.append(overlay)
}

// buildHead renders the band row (Input/Output) and the column header row.
function buildHead(t: TableView): HTMLElement {
  const head = el('thead')
  const band = el('tr', { class: 'dt-band' }, el('th', { class: 'dt-idx' }, ''))
  if (t.inputs.length) band.append(el('th', { class: 'dt-in', colspan: String(t.inputs.length) }, 'Input'))
  if (t.outputs.length) band.append(el('th', { class: 'dt-out', colspan: String(t.outputs.length) }, 'Output'))
  band.append(el('th', { class: 'dt-ann' }, ''), el('th', { class: 'dt-del' }, ''))
  head.append(band)

  const cols = el('tr', { class: 'dt-cols' }, el('th', { class: 'dt-idx' }, '#'))
  for (const c of t.inputs) cols.append(el('th', { class: 'dt-in', title: c.typeRef ?? '' }, colLabel(c.label || c.expression, c.typeRef)))
  for (const c of t.outputs) cols.append(el('th', { class: 'dt-out', title: c.typeRef ?? '' }, colLabel(c.label || c.name || '', c.typeRef)))
  cols.append(el('th', { class: 'dt-ann' }, 'Annotation'), el('th', { class: 'dt-del' }, ''))
  head.append(cols)
  return head
}

// buildRow renders one editable rule row: input cells (unary tests), output cells
// (FEEL expressions), a free-text annotation and a delete button.
function buildRow(t: TableView, names: string[], r: TableRule): HTMLElement {
  const row = el('tr', { class: 'dt-rule' }, el('td', { class: 'dt-idx' }, ''))
  t.inputs.forEach((_, k) => row.append(cellTd('dt-in', inputCell(names, r.inputEntries[k] ?? ''))))
  t.outputs.forEach((_, k) => row.append(cellTd('dt-out', outputCell(names, r.outputEntries[k] ?? ''))))
  row.append(cellTd('dt-ann', annotationCell((r.annotations ?? [])[0] ?? '')))

  const del = el('button', { class: 'dt-rowdel', type: 'button', title: 'Regel löschen' }, '🗑') as HTMLButtonElement
  del.addEventListener('click', () => {
    const body = row.parentElement
    row.remove()
    if (body) renumber(body)
  })
  row.append(el('td', { class: 'dt-del' }, del))
  return row
}

function cellTd(cls: string, input: HTMLInputElement): HTMLElement {
  return el('td', { class: cls }, input)
}

// inputCell is a unary-test field: empty means "any" (valid); otherwise the FEEL
// engine validates it live.
function inputCell(names: string[], value: string): HTMLInputElement {
  const box = el('input', { type: 'text', class: 'dt-cell dt-cell-in', value, placeholder: '–' }) as HTMLInputElement
  const check = (): void => {
    const s = box.value.trim()
    const res = s === '' || s === '-' ? { ok: true } : validateUnary(s, names)
    mark(box, res)
  }
  box.addEventListener('input', check)
  box.addEventListener('blur', check)
  check()
  return box
}

// outputCell is a FEEL-expression field: an empty cell is invalid (a rule must
// produce an output).
function outputCell(names: string[], value: string): HTMLInputElement {
  const box = el('input', { type: 'text', class: 'dt-cell dt-cell-out', value }) as HTMLInputElement
  const check = (): void => {
    const s = box.value.trim()
    const res = s === '' ? { ok: false, message: 'Output darf nicht leer sein' } : validateExpr(s, names)
    mark(box, res)
  }
  box.addEventListener('input', check)
  box.addEventListener('blur', check)
  check()
  return box
}

function annotationCell(value: string): HTMLInputElement {
  return el('input', { type: 'text', class: 'dt-cell dt-cell-ann', value, placeholder: '—' }) as HTMLInputElement
}

// mark toggles a cell's invalid state and shows the engine's reason as a tooltip.
function mark(box: HTMLInputElement, res: { ok: boolean; message?: string }): void {
  box.classList.toggle('dt-cell-invalid', !res.ok)
  box.title = res.ok ? '' : res.message ?? 'ungültig'
}

// collectRules reads the edited rows back into TableRules aligned with the table's
// columns.
function collectRules(tbody: HTMLElement, t: TableView): TableRule[] {
  const rows = [...tbody.querySelectorAll('.dt-rule')]
  return rows.map((row) => {
    const ins = [...row.querySelectorAll<HTMLInputElement>('.dt-cell-in')].map((b) => b.value)
    const outs = [...row.querySelectorAll<HTMLInputElement>('.dt-cell-out')].map((b) => b.value)
    const ann = (row.querySelector<HTMLInputElement>('.dt-cell-ann')?.value ?? '').trim()
    return {
      inputEntries: ins.slice(0, t.inputs.length),
      outputEntries: outs.slice(0, t.outputs.length),
      annotations: ann ? [ann] : [],
    }
  })
}

function emptyRule(t: TableView): TableRule {
  return { inputEntries: t.inputs.map(() => ''), outputEntries: t.outputs.map(() => ''), annotations: [] }
}

// renumber rewrites the visible row index after add/delete.
function renumber(tbody: HTMLElement): void {
  tbody.querySelectorAll('.dt-rule .dt-idx').forEach((td, i) => {
    td.textContent = String(i + 1)
  })
}

function colLabel(text: string, typeRef?: string): HTMLElement {
  const wrap = el('span', { class: 'dt-coltext' }, text || '—')
  if (typeRef) wrap.append(el('span', { class: 'dt-type' }, typeRef))
  return wrap
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
