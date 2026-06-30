import { getContext, saveContext, type ContextView, type ContextEntryEdit } from './api'
import { ensureFeel, validateExpr } from './feel'
import { FEEL_TYPES } from './feeltypes'

// openContextOverlay shows a decision's boxed context in an editable modal
// (ADR-0016): a grid of (name → FEEL expression) entries plus an optional result
// cell, each expression validated live against the real engine and saved back.
// A later entry may reference the names bound by the entries above it, so each
// row validates with the preceding names in scope. When the decision has no
// context yet (an undecided decision) the editor opens with one empty entry;
// saving creates it. A context that contains a nested non-literal expression is
// shown read-only (it cannot be represented by this simple editor). names are the
// in-scope variables from the rest of the model. onSaved gets the new model id.
export async function openContextOverlay(modelId: string, decisionId: string, title: string, names: string[], onSaved?: (newModelId: string) => void, opts?: { fresh?: boolean; typeOptions?: string[]; readOnly?: boolean }): Promise<void> {
  const typeOptions = opts?.typeOptions ?? FEEL_TYPES
  let ctx: ContextView | null = null
  if (!opts?.fresh) {
    try {
      ctx = await getContext(modelId, decisionId)
    } catch (e) {
      console.error(e)
      return
    }
  }
  void ensureFeel()

  // A nested non-literal context is not editable here; force read-only.
  const readOnly = opts?.readOnly || (ctx != null && !ctx.simple)

  type Row = { name: string; text: string; typeRef: string; result: boolean; kind: string }
  const rows: Row[] = []
  for (const e of ctx?.entries ?? []) {
    rows.push({ name: e.name ?? '', text: e.text, typeRef: e.typeRef ?? '', result: !!e.result, kind: e.kind })
  }
  if (rows.length === 0) rows.push({ name: '', text: '', typeRef: '', result: false, kind: 'literal' })

  const close = (): void => {
    overlay.remove()
    document.removeEventListener('keydown', onKey)
  }
  const onKey = (e: KeyboardEvent): void => {
    const a = document.activeElement
    if (e.key === 'Escape' && a?.tagName !== 'TEXTAREA' && a?.tagName !== 'INPUT') close()
  }
  document.addEventListener('keydown', onKey)

  const overlay = el('div', { class: 'dt-overlay' })
  overlay.addEventListener('click', (e) => {
    if (e.target === overlay) close()
  })

  const closeBtn = el('button', { class: 'dt-close', type: 'button', title: 'Schließen (Esc)' }, '✕') as HTMLButtonElement
  closeBtn.addEventListener('click', close)
  const header = el('div', { class: 'dt-head' }, el('span', { class: 'dt-title' }, title || ctx?.name || decisionId), closeBtn)

  const status = el('span', { class: 'dt-status' })
  const grid = el('div', { class: 'ctx-grid' })

  // nameScope returns the variables visible to the row at index i: the model's
  // names plus the names bound by the named entries above it.
  const nameScope = (i: number): string[] => {
    const above = rows.slice(0, i).filter((r) => !r.result && r.name.trim()).map((r) => r.name.trim())
    return [...names, ...above]
  }

  const validateRow = (row: Row, i: number): string => {
    const text = row.text.trim()
    if (text === '') return row.result ? '' : 'Ausdruck darf nicht leer sein'
    if (!row.result && row.name.trim() === '') return 'Name fehlt'
    const dup = rows.some((r, j) => j !== i && !r.result && r.name.trim() !== '' && r.name.trim() === row.name.trim())
    if (dup) return 'Name doppelt'
    const res = validateExpr(text, nameScope(i))
    return res.ok ? '' : res.message ?? 'ungültig'
  }

  let render = (): void => {}
  const removeRow = (i: number): void => {
    rows.splice(i, 1)
    render()
  }
  const addRow = (): void => {
    // Insert a new named entry before the result cell, if any.
    const at = rows.findIndex((r) => r.result)
    const row: Row = { name: '', text: '', typeRef: '', result: false, kind: 'literal' }
    if (at < 0) rows.push(row)
    else rows.splice(at, 0, row)
    render()
  }
  const ensureResult = (on: boolean): void => {
    const at = rows.findIndex((r) => r.result)
    if (on && at < 0) rows.push({ name: '', text: '', typeRef: '', result: true, kind: 'literal' })
    else if (!on && at >= 0) rows.splice(at, 1)
    render()
  }

  const typeSelect = (row: Row): HTMLSelectElement => {
    const sel = el('select', { class: 'ctx-type', title: 'Typ' }) as HTMLSelectElement
    const cur = row.typeRef
    const list = cur && !typeOptions.includes(cur) ? [...typeOptions, cur] : typeOptions
    for (const t of list) {
      const o = el('option', { value: t }, t || '— Typ —') as HTMLOptionElement
      o.selected = cur === t
      sel.append(o)
    }
    sel.disabled = readOnly
    sel.addEventListener('change', () => {
      row.typeRef = sel.value
    })
    return sel
  }

  render = (): void => {
    grid.replaceChildren()
    grid.append(
      el('div', { class: 'ctx-h' }, 'Name'),
      el('div', { class: 'ctx-h' }, 'Ausdruck (FEEL)'),
      el('div', { class: 'ctx-h' }, 'Typ'),
      el('div', { class: 'ctx-h' }, ''),
    )
    rows.forEach((row, i) => {
      const nameCell = row.result
        ? el('div', { class: 'ctx-name ctx-result-label', title: 'Ergebniszelle des Kontexts' }, '▸ Ergebnis')
        : (() => {
            const inp = el('input', { class: 'ctx-name-in', type: 'text', placeholder: 'name', value: row.name, spellcheck: 'false' }) as HTMLInputElement
            inp.disabled = readOnly
            inp.addEventListener('input', () => {
              row.name = inp.value
              check()
            })
            return inp
          })()

      let exprCell: HTMLElement
      if (row.kind !== 'literal') {
        exprCell = el('div', { class: 'ctx-nested', title: 'Verschachtelter Ausdruck — hier nicht editierbar' }, '⟨' + row.kind + '⟩')
      } else {
        const ta = el('textarea', { class: 'ctx-text', rows: '1', spellcheck: 'false', placeholder: row.result ? 'Ergebnisausdruck' : 'FEEL-Ausdruck' }) as HTMLTextAreaElement
        ta.value = row.text
        ta.readOnly = readOnly
        ta.addEventListener('input', () => {
          row.text = ta.value
          check()
        })
        exprCell = ta
      }

      const del = el('button', { class: 'ctx-del', type: 'button', title: 'Zeile entfernen' }, '✕') as HTMLButtonElement
      del.disabled = readOnly || (!row.result && rows.filter((r) => !r.result).length <= 1)
      del.addEventListener('click', () => removeRow(i))

      grid.append(
        el('div', { class: 'ctx-name' }, nameCell),
        el('div', { class: 'ctx-expr' }, exprCell),
        el('div', { class: 'ctx-typecell' }, typeSelect(row)),
        el('div', { class: 'ctx-act' }, readOnly ? document.createTextNode('') : del),
      )
    })
    check()
  }

  const hasResult = (): boolean => rows.some((r) => r.result)
  const check = (): void => {
    let firstErr = ''
    rows.forEach((row, i) => {
      const msg = validateRow(row, i)
      if (msg && !firstErr) firstErr = (row.result ? 'Ergebnis' : row.name.trim() || '#' + (i + 1)) + ': ' + msg
    })
    status.className = 'dt-status' + (firstErr ? ' dt-error' : '')
    status.textContent = firstErr
    saveBtn.disabled = readOnly || !!firstErr
  }

  const addBtn = el('button', { class: 'tbtn ctx-add', type: 'button' }, '+ Eintrag') as HTMLButtonElement
  addBtn.addEventListener('click', addRow)
  const resultToggle = el('label', { class: 'ctx-result-toggle' }) as HTMLLabelElement
  const resultChk = el('input', { type: 'checkbox' }) as HTMLInputElement
  resultChk.checked = hasResult()
  resultChk.disabled = readOnly
  resultChk.addEventListener('change', () => ensureResult(resultChk.checked))
  resultToggle.append(resultChk, document.createTextNode(' Ergebniszelle'))

  const saveBtn = el('button', { class: 'tbtn dt-save', type: 'button' }, 'Speichern') as HTMLButtonElement
  const save = async (): Promise<void> => {
    check()
    if (saveBtn.disabled) return
    saveBtn.disabled = true
    status.className = 'dt-status'
    status.textContent = 'speichert …'
    const entries: ContextEntryEdit[] = rows
      .filter((r) => r.text.trim() !== '')
      .map((r) => ({ name: r.result ? '' : r.name.trim(), text: r.text.trim(), typeRef: r.typeRef }))
    try {
      const saved = await saveContext(modelId, decisionId, entries)
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

  const tools = readOnly
    ? el('div', { class: 'dt-toolbar' }, el('span', { class: 'dt-readonly-note' }, ctx && !ctx.simple ? 'Verschachtelter Kontext — schreibgeschützt' : 'Schreibgeschützt'), status)
    : el('div', { class: 'dt-toolbar' }, addBtn, resultToggle, saveBtn, status)

  const modal = el('div', { class: 'dt-modal ctx-modal' }, header, el('div', { class: 'ctx-body' }, grid), tools)
  if (readOnly) modal.classList.add('dt-readonly')
  overlay.append(modal)
  document.body.append(overlay)
  render()
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
