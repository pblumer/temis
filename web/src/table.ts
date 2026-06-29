import { getTable, type TableView } from './api'

// openTableOverlay fetches a decision's decision-table and shows it read-only in a
// modal overlay (ADR-0016 WP-67: own modeler, no dmn-js). Double-clicking a
// table-decision in the DRD opens this. Editing the table is a later slice; for
// now it brings back the ability to *see* the rules the legacy editor offered.
export async function openTableOverlay(modelId: string, decisionId: string): Promise<void> {
  let table: TableView | null
  try {
    table = await getTable(modelId, decisionId)
  } catch (e) {
    table = null
    console.error(e)
  }
  if (!table) return

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

  const policy = table.hitPolicy + (table.aggregation ? ' ' + table.aggregation : '')
  const header = el(
    'div',
    { class: 'dt-head' },
    el('span', { class: 'dt-title' }, table.name || decisionId),
    el('span', { class: 'dt-policy', title: 'Hit Policy' }, policy),
    el('button', { class: 'dt-close', type: 'button', title: 'Schließen (Esc)' }, '✕'),
  )
  header.querySelector('.dt-close')?.addEventListener('click', close)

  overlay.append(el('div', { class: 'dt-modal' }, header, buildTable(table)))
  document.body.append(overlay)
}

// buildTable renders the decision table as an HTML table: an input/output header
// band, the column expressions/names, then one row per rule.
function buildTable(t: TableView): HTMLElement {
  const table = el('table', { class: 'dt' })

  // Band row: which columns are inputs vs outputs.
  const band = el('tr', { class: 'dt-band' })
  band.append(el('th', { class: 'dt-idx' }, ''))
  if (t.inputs.length) band.append(el('th', { class: 'dt-in', colspan: String(t.inputs.length) }, 'Input'))
  if (t.outputs.length) band.append(el('th', { class: 'dt-out', colspan: String(t.outputs.length) }, 'Output'))
  if (hasAnnotations(t)) band.append(el('th', { class: 'dt-ann' }, ''))
  table.append(band)

  // Column header row: input expressions and output names, with types.
  const head = el('tr', { class: 'dt-cols' })
  head.append(el('th', { class: 'dt-idx' }, '#'))
  for (const c of t.inputs) head.append(el('th', { class: 'dt-in', title: c.typeRef ?? '' }, colLabel(c.label || c.expression, c.typeRef)))
  for (const c of t.outputs) head.append(el('th', { class: 'dt-out', title: c.typeRef ?? '' }, colLabel(c.label || c.name || '', c.typeRef)))
  if (hasAnnotations(t)) head.append(el('th', { class: 'dt-ann' }, 'Annotation'))
  table.append(head)

  // Rule rows.
  t.rules.forEach((r, i) => {
    const row = el('tr', { class: 'dt-rule' })
    row.append(el('td', { class: 'dt-idx' }, String(i + 1)))
    for (let k = 0; k < t.inputs.length; k++) row.append(el('td', { class: 'dt-in' }, cell(r.inputEntries[k])))
    for (let k = 0; k < t.outputs.length; k++) row.append(el('td', { class: 'dt-out' }, cell(r.outputEntries[k])))
    if (hasAnnotations(t)) row.append(el('td', { class: 'dt-ann' }, cell((r.annotations ?? []).join(' · '))))
    table.append(row)
  })
  return table
}

function hasAnnotations(t: TableView): boolean {
  return t.rules.some((r) => (r.annotations ?? []).some((a) => a.trim() !== ''))
}

function colLabel(text: string, typeRef?: string): HTMLElement {
  const wrap = el('span', { class: 'dt-coltext' }, text || '—')
  if (typeRef) wrap.append(el('span', { class: 'dt-type' }, typeRef))
  return wrap
}

// cell renders a rule cell: a dash for an empty/“-” entry (the DMN "any" match).
function cell(text: string | undefined): string {
  const s = (text ?? '').trim()
  return s === '' || s === '-' ? '–' : s
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
