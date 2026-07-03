import { getTable, saveTable, type Anchor, type TableView, type TableInput, type TableOutput, type TableRule, type TableEdit, type TableTrace, type TraceInput, type TraceRule } from './api'
import { ensureFeel, validateExpr, validateUnary, validateName } from './feel'
import { attachCompletion, feelItems, type CompletionItem } from './complete'
import { attachHighlighter } from './highlight'
import { FEEL_TYPES } from './feeltypes'

// Hit policies offered in the editor (single-letter DMN codes) and the Collect
// aggregations.
const HIT_POLICIES: [string, string][] = [
  ['U', 'Unique'], ['A', 'Any'], ['P', 'Priority'], ['F', 'First'], ['R', 'Rule order'], ['C', 'Collect'],
]
const AGGREGATIONS = ['', 'SUM', 'COUNT', 'MIN', 'MAX']

// openTableOverlay fetches a decision's decision-table and shows it in a fully
// editable modal (ADR-0016): hit policy, input/output columns (add/remove, edit
// expression/name/type) and rule cells, all FEEL-validated, then saved back into
// the model. onSaved gets the saved model's new id (its content hash changed).
// opts.matched highlights the rule row(s) that fired in an evaluation (the
// Operate view's hit-rule highlight); opts.readOnly opens the table for viewing
// only (no editing/saving) — used when inspecting a past run. opts.trace, when
// given (read-only), draws the decision PATH: the tested input value per column,
// a per-cell pass/fail heatmap over every rule and the winning rule glowing, plus
// a chip-and-arrow summary bar (input value → matched rule → output).
// opts.wiredInputs are the decision's information-requirement inputs, surfaced as
// columns when the table lacks them. opts.scope is the decision's in-scope
// variables from the graph (connected input-data nodes and upstream decisions)
// that the input-column expressions may reference. opts.anchor targets a BKM's
// boxed decision-table body instead of a decision's logic (WP-66).
export async function openTableOverlay(modelId: string, decisionId: string, onSaved?: (newModelId: string) => void, typeOptions: string[] = FEEL_TYPES, opts: { matched?: number[]; readOnly?: boolean; trace?: TableTrace; wiredInputs?: { expression: string; typeRef?: string }[]; scope?: string[]; anchor?: Anchor } = {}): Promise<void> {
  let fetched: TableView | null
  try {
    fetched = await getTable(modelId, decisionId, opts.anchor)
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

  // Reconcile with the decision's wired inputs (its information requirements). A
  // table's input columns are derived from requirements only when the table is
  // created, so an input wired in afterwards has no column and is missing from the
  // editor. Surface each such input as a column (matching by expression), extending
  // any existing rules to keep them aligned. Skipped in read-only/trace inspection,
  // where the grid must mirror the run exactly. The user can still remove a column.
  if (!opts.readOnly) {
    const have = new Set(state.inputs.map((c) => c.expression.trim()))
    for (const w of opts.wiredInputs ?? []) {
      const expr = w.expression.trim()
      if (!expr || have.has(expr)) continue
      have.add(expr)
      state.inputs.push({ expression: w.expression, typeRef: w.typeRef ?? '' })
      state.rules.forEach((r) => r.inputEntries.push(''))
    }
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

  // The decision's in-scope variables (connected input-data nodes and upstream
  // decisions), passed in from the graph. The input-column *expressions* reference
  // these; without them a plain reference like `Name` reads as an unknown variable.
  const scope = opts.scope ?? []
  const scopeNames = (): string[] => scope
  // names that a rule cell may reference: the decision's scope plus the input
  // column expressions (each column expression is bound as a name for the cells).
  const inputNames = (): string[] => state.inputs.map((c) => c.expression).filter((s) => s.trim() !== '')
  const cellNames = (): string[] => [...scope, ...inputNames()]

  const matched = new Set(opts.matched ?? [])
  // The decision trace is only shown in read-only inspection (a past run).
  const trace = opts.readOnly ? opts.trace : undefined
  // Syntax-highlight every FEEL cell/header. Cells are created detached during
  // buildGrid, so this runs once the grid is in the DOM (after the modal is
  // appended, and again after each structural rebuild). Each field is wrapped
  // only once (guarded by data-hl); fresh cells from a rebuild are wrapped anew.
  const highlightCells = (): void => {
    if (!scroll.isConnected) return
    for (const f of scroll.querySelectorAll<HTMLInputElement>('.dt-head-field, .dt-cell-in, .dt-cell-out')) {
      if (f.dataset.hl) continue
      f.dataset.hl = '1'
      attachHighlighter(f, cellNames)
    }
  }
  const rebuild = (): void => {
    scroll.textContent = ''
    // scopeNames feeds the input-column headers (the decision's variables);
    // cellNames feeds the rule cells (scope + the current column expressions).
    // Both are live providers so completion/validation reflect edits immediately.
    scroll.append(buildGrid(state, scopeNames, cellNames, rebuild, typeOptions, matched, trace))
    highlightCells()
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
      const saved = await saveTable(modelId, decisionId, edit, opts.anchor)
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
  const modal = el('div', { class: 'dt-modal' }, header, scroll, toolbar)
  // Read-only trace inspection: prepend the decision-path summary bar and switch
  // the grid into heatmap mode (per-cell pass/fail, glowing hit row).
  if (trace) {
    modal.classList.add('dt-trace')
    modal.insertBefore(decisionPath(state, trace), scroll)
  }
  overlay.append(modal)

  // Read-only (Operate): no structural edits — disable every field/button and
  // replace the editing toolbar with a hint. The matched-rule highlight stays.
  if (opts.readOnly) {
    modal.classList.add('dt-readonly')
    for (const f of scroll.querySelectorAll<HTMLInputElement | HTMLSelectElement | HTMLButtonElement>('input, select, button')) f.disabled = true
    toolbar.textContent = ''
    if (matched.size) toolbar.append(el('span', { class: 'dt-status' }, 'Regel ' + [...matched].map((m) => m + 1).join(', ') + ' hat gehittet'))
    else toolbar.append(el('span', { class: 'dt-status' }, 'keine Regel hat gehittet'))
  }

  document.body.append(overlay)
  // The first grid was built before the modal was in the DOM; highlight it now
  // that the fields have their computed styles.
  highlightCells()
}

// buildGrid renders the editable table from the working state. rebuild is called
// after a structural change (column/row add/remove) to redraw. scopeProvider
// yields the decision's in-scope variables (for the input-column headers);
// cellProvider yields those plus the column names (for the rule cells). Both are
// live so validation/completion follow edits without a full rebuild.
function buildGrid(state: TableView, scopeProvider: () => string[], cellProvider: () => string[], rebuild: () => void, typeOptions: string[], matched: Set<number>, trace?: TableTrace): HTMLElement {
  // Trace rules keyed by their (0-based) rule index, so a row finds its per-cell
  // pass/fail verdict even if the engine omitted or reordered any.
  const traceRules = new Map<number, TraceRule>((trace?.rules ?? []).map((r) => [r.index, r]))
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
  state.inputs.forEach((c, k) => cols.append(inputHeader(state, c, k, rebuild, typeOptions, scopeProvider, trace?.inputs?.[k])))
  state.outputs.forEach((c, k) => cols.append(outputHeader(state, c, k, rebuild, typeOptions)))
  cols.append(el('th', { class: 'dt-ann' }, ''), el('th', { class: 'dt-del' }, ''))
  head.append(cols)
  table.append(head)

  // Rule rows.
  const body = el('tbody')
  state.rules.forEach((r, i) => body.append(ruleRow(state, r, i, rebuild, matched.has(i), cellProvider, traceRules.get(i))))
  table.append(body)
  return table
}

function inputHeader(state: TableView, col: TableInput, k: number, rebuild: () => void, typeOptions: string[], scopeProvider: () => string[], traceInput?: TraceInput): HTMLElement {
  const expr = el('input', { class: 'dt-head-field', value: col.expression ?? '', placeholder: 'FEEL' }) as HTMLInputElement
  const check = (): void => {
    const s = expr.value.trim()
    col.expression = expr.value
    // The input expression references the decision's scope (input-data nodes and
    // upstream decisions), so validate/complete against those, not the columns.
    mark(expr, s === '' ? { ok: false, message: 'Input-Ausdruck darf nicht leer sein' } : validateExpr(s, scopeProvider()))
  }
  expr.addEventListener('input', check)
  attachCompletion(expr, () => feelItems(scopeProvider()))
  check()
  const colhead = el('div', { class: 'dt-colhead' }, expr, typeSelect(col, typeOptions), removeBtn(() => {
    state.inputs.splice(k, 1)
    state.rules.forEach((r) => r.inputEntries.splice(k, 1))
    rebuild()
  }))
  // In trace mode, show the value this column was tested with (the input flowing in).
  if (traceInput) colhead.append(el('span', { class: 'dt-trace-val', title: 'getesteter Eingabewert' }, '= ' + fmtVal(traceInput.value)))
  return el('th', { class: 'dt-in' }, colhead)
}

function outputHeader(state: TableView, col: TableOutput, k: number, rebuild: () => void, typeOptions: string[]): HTMLElement {
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
  return el('th', { class: 'dt-out' }, el('div', { class: 'dt-colhead' }, name, typeSelect(col, typeOptions), rm))
}

function ruleRow(state: TableView, r: TableRule, i: number, rebuild: () => void, hit = false, cellProvider: () => string[] = () => [], tr?: TraceRule): HTMLElement {
  const row = el('tr', { class: hit ? 'dt-rule dt-hit' : 'dt-rule' }, el('td', { class: 'dt-idx' }, String(i + 1)))
  // In trace mode each input cell is tinted by whether its condition held for the
  // tested value — the per-cell heatmap that makes the taken path readable.
  state.inputs.forEach((_, k) => {
    const cond = tr?.conditions?.[k]
    const tint = cond ? (cond.matched ? ' dt-c-ok' : ' dt-c-no') : ''
    row.append(el('td', { class: 'dt-in' + tint }, cell(r.inputEntries, k, 'in', cellProvider)))
  })
  state.outputs.forEach((_, k) => row.append(el('td', { class: 'dt-out' + (hit && tr ? ' dt-out-hit' : '') }, cell(r.outputEntries, k, 'out', cellProvider))))
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
function cell(entries: string[], k: number, kind: 'in' | 'out', namesProvider: () => string[]): HTMLInputElement {
  const box = el('input', { class: 'dt-cell dt-cell-' + kind, value: entries[k] ?? '', placeholder: kind === 'in' ? '–' : '' }) as HTMLInputElement
  const check = (): void => {
    entries[k] = box.value
    const s = box.value.trim()
    if (kind === 'in') mark(box, s === '' || s === '-' ? { ok: true } : validateUnary(s, namesProvider()))
    else mark(box, s === '' ? { ok: false, message: 'Output darf nicht leer sein' } : validateExpr(s, namesProvider()))
  }
  box.addEventListener('input', check)
  // Input cells are unary tests over the column value, bound to `?` by the engine,
  // so offer it alongside the in-scope names; output cells are plain expressions.
  const extra: CompletionItem[] = kind === 'in' ? [{ label: '?', kind: 'variable', detail: 'Eingabewert dieser Spalte' }] : []
  attachCompletion(box, () => feelItems(namesProvider(), extra))
  check()
  return box
}

// withCurrent ensures the column's current type appears in the options, even if
// it is a custom type that has since been removed from the model.
function withCurrent(options: string[], current?: string): string[] {
  return current && !options.includes(current) ? [...options, current] : options
}

function typeSelect(col: { typeRef?: string }, typeOptions: string[]): HTMLSelectElement {
  const sel = el('select', { class: 'dt-type-sel', title: 'Typ' }) as HTMLSelectElement
  for (const t of withCurrent(typeOptions, col.typeRef)) sel.append(option(t, t || '— Typ —', (col.typeRef ?? '') === t))
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

// decisionPath draws the taken route as a chip-and-arrow summary bar:
//   [Age = 85]  →  [Regel 2 · >= 18]  →  [Category = adult]
// so the decision's way through the table reads left-to-right at a glance.
function decisionPath(state: TableView, tr: TableTrace): HTMLElement {
  const bar = el('div', { class: 'dt-path' })
  const chip = (cls: string, label: string, value: string): HTMLElement =>
    el('span', { class: 'dt-path-chip ' + cls }, el('span', { class: 'dt-path-lbl' }, label), el('span', { class: 'dt-path-val' }, value))
  const arrow = (): HTMLElement => el('span', { class: 'dt-path-arrow' }, '→')

  // The inputs that flowed in.
  for (const in_ of tr.inputs ?? []) bar.append(chip('dt-path-in', in_.expression, fmtVal(in_.value)))

  const hits = (tr.rules ?? []).filter((r) => r.matched)
  if (hits.length) {
    bar.append(arrow())
    for (const r of hits) {
      // The rule's non-trivial conditions, so the chip reads like the rule (">= 18").
      const conds = r.conditions.map((c) => (c.entry ?? '').trim()).filter((e) => e !== '' && e !== '-')
      bar.append(chip('dt-path-rule', 'Regel ' + (r.index + 1), conds.join(' · ') || 'sonst'))
    }
    bar.append(arrow())
    // The outputs the winning rule produced.
    const r0 = hits[0]
    state.outputs.forEach((o, k) => bar.append(chip('dt-path-out', o.name || 'Output', fmtVal(r0.outputs?.[k]))))
  } else {
    bar.append(arrow(), chip('dt-path-rule dt-path-none', 'keine Regel', '—'))
  }
  return bar
}

// fmtVal renders a traced value compactly: strings as-is, null as a dash, other
// JSON-serialisable values as compact JSON.
function fmtVal(v: unknown): string {
  if (v === null || v === undefined) return '—'
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
