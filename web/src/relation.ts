import { getRelation, saveRelation, type Anchor, type RelationView } from './api'
import { ensureFeel, validateExpr, validateName } from './feel'
import { attachFeelField } from './feelfield'

// openRelationOverlay edits a decision's boxed relation (WP-66): a grid of named
// columns and rows of FEEL cells (reference/lookup data), each cell validated
// live against the real engine and saved back into the model. Columns and rows
// can be added and removed. baseNames are the in-scope variables the cells may
// reference. A relation whose cells nest other boxed expressions (rv.simple ===
// false) opens read-only, so the text grid never clobbers the nesting. onSaved
// gets the saved model's id.
export async function openRelationOverlay(modelId: string, decisionId: string, baseNames: string[], onSaved?: (newModelId: string) => void, opts?: { readOnly?: boolean; anchor?: Anchor }): Promise<void> {
  let rv: RelationView | null = null
  try {
    rv = await getRelation(modelId, decisionId, opts?.anchor)
  } catch (e) {
    console.error(e)
    return
  }
  if (!rv) return
  void ensureFeel()

  const readOnly = !!opts?.readOnly || rv.simple === false
  let columns: string[] = rv.columns.length ? [...rv.columns] : ['Spalte 1']
  let rows: string[][] = rv.rows.length ? rv.rows.map((r) => [...r]) : [columns.map(() => '')]
  // Keep every row aligned to the columns (pad/truncate) so the grid is rectangular.
  const normalize = (): void => {
    rows = rows.map((r) => columns.map((_, j) => r[j] ?? ''))
  }
  normalize()

  const close = (): void => {
    overlay.remove()
    document.removeEventListener('keydown', onKey)
  }
  const onKey = (e: KeyboardEvent): void => {
    const tag = (document.activeElement?.tagName ?? '').toLowerCase()
    if (e.key === 'Escape' && tag !== 'input' && tag !== 'textarea') close()
  }
  document.addEventListener('keydown', onKey)

  const overlay = el('div', { class: 'dt-overlay' })
  overlay.addEventListener('click', (e) => {
    if (e.target === overlay) close()
  })

  const closeBtn = el('button', { class: 'dt-close', type: 'button', title: 'Schließen (Esc)' }, '✕') as HTMLButtonElement
  closeBtn.addEventListener('click', close)
  const header = el('div', { class: 'dt-head' }, el('span', { class: 'dt-title' }, 'Relation · ' + (rv.name || decisionId)), closeBtn)

  const status = el('span', { class: 'dt-status' })
  const saveBtn = el('button', { class: 'tbtn dt-save', type: 'button' }, 'Speichern') as HTMLButtonElement
  const grid = el('div', { class: 'rel-grid' })

  // check validates the column names (non-empty, valid, unique) and every non-blank
  // cell (valid FEEL); a partially filled row is an error, a fully blank row is
  // dropped. It marks the offending fields and reports the first problem.
  const check = (): boolean => {
    let firstErr = ''
    const heads = grid.querySelectorAll<HTMLInputElement>('.rel-colname')
    const counts = new Map<string, number>()
    columns.forEach((c) => {
      const n = c.trim()
      if (n) counts.set(n, (counts.get(n) ?? 0) + 1)
    })
    columns.forEach((c, j) => {
      const n = c.trim()
      const dup = n !== '' && (counts.get(n) ?? 0) > 1
      const res = n === '' ? { ok: false, message: 'Name fehlt' } : validateName(n)
      const bad = !res.ok || dup
      heads[j]?.classList.toggle('rel-invalid', bad)
      if (bad && !firstErr) firstErr = 'Spalte ' + (j + 1) + ': ' + (dup ? 'Name doppelt' : res.message ?? 'ungültig')
    })
    const cells = grid.querySelectorAll<HTMLInputElement>('.rel-cell')
    rows.forEach((row, i) => {
      const filled = row.map((c) => c.trim())
      const anyFilled = filled.some((c) => c !== '')
      const allFilled = filled.every((c) => c !== '')
      filled.forEach((c, j) => {
        const cell = cells[i * columns.length + j]
        const bad = anyFilled && (c === '' ? true : !validateExpr(c, baseNames).ok)
        cell?.classList.toggle('rel-invalid', bad)
        if (bad && !firstErr) {
          firstErr = 'Zeile ' + (i + 1) + ', ' + (columns[j].trim() || 'Spalte ' + (j + 1)) + ': ' + (c === '' ? 'leer' : validateExpr(c, baseNames).message ?? 'ungültig')
        }
      })
      if (anyFilled && !allFilled && !firstErr) firstErr = 'Zeile ' + (i + 1) + ': alle Zellen ausfüllen'
    })
    if (!rows.some((r) => r.some((c) => c.trim() !== '')) && !firstErr) firstErr = 'Mindestens eine Zeile ausfüllen.'
    status.className = 'dt-status' + (firstErr ? ' dt-error' : '')
    status.textContent = firstErr
    saveBtn.disabled = readOnly || !!firstErr
    return !firstErr
  }

  const render = (): void => {
    normalize()
    grid.style.gridTemplateColumns = `28px repeat(${columns.length}, minmax(110px, 1fr)) 28px`
    grid.replaceChildren()
    // Header: corner, column-name inputs (+ delete), trailing add-column button.
    grid.append(el('div', { class: 'rel-corner' }))
    columns.forEach((c, j) => {
      const name = el('input', { class: 'rel-colname', type: 'text', spellcheck: 'false', placeholder: 'Spalte' }) as HTMLInputElement
      name.value = c
      name.readOnly = readOnly
      name.addEventListener('input', () => {
        columns[j] = name.value
        check()
      })
      const del = el('button', { class: 'rel-coldel rel-x', type: 'button', title: 'Spalte entfernen' }, '✕') as HTMLButtonElement
      del.disabled = readOnly || columns.length <= 1
      del.addEventListener('click', () => {
        columns.splice(j, 1)
        rows.forEach((r) => r.splice(j, 1))
        render()
      })
      grid.append(el('div', { class: 'rel-head' }, name, readOnly ? document.createTextNode('') : del))
    })
    const addCol = el('button', { class: 'rel-add-col', type: 'button', title: 'Spalte hinzufügen' }, '+') as HTMLButtonElement
    addCol.disabled = readOnly
    addCol.addEventListener('click', () => {
      columns.push('Spalte ' + (columns.length + 1))
      rows.forEach((r) => r.push(''))
      render()
    })
    grid.append(readOnly ? el('div', { class: 'rel-corner' }) : el('div', { class: 'rel-corner' }, addCol))

    // Body: row index, cells, trailing delete-row button.
    rows.forEach((row, i) => {
      grid.append(el('div', { class: 'rel-rownum' }, String(i + 1)))
      row.forEach((cell, j) => {
        const input = el('input', { class: 'rel-cell', type: 'text', spellcheck: 'false', placeholder: 'FEEL' }) as HTMLInputElement
        input.value = cell
        input.readOnly = readOnly
        input.addEventListener('input', () => {
          rows[i][j] = input.value
          check()
        })
        grid.append(el('div', { class: 'rel-cellwrap' }, input))
        attachFeelField(input, () => baseNames, { readOnly })
      })
      const del = el('button', { class: 'rel-x', type: 'button', title: 'Zeile entfernen' }, '✕') as HTMLButtonElement
      del.disabled = readOnly || rows.length <= 1
      del.addEventListener('click', () => {
        rows.splice(i, 1)
        render()
      })
      grid.append(el('div', { class: 'rel-rowact' }, readOnly ? document.createTextNode('') : del))
    })
    check()
  }

  const addRow = el('button', { class: 'tbtn', type: 'button' }, '+ Zeile') as HTMLButtonElement
  addRow.addEventListener('click', () => {
    rows.push(columns.map(() => ''))
    render()
  })

  const save = async (): Promise<void> => {
    if (!check()) return
    saveBtn.disabled = true
    status.className = 'dt-status'
    status.textContent = 'speichert …'
    try {
      const saved = await saveRelation(modelId, decisionId, {
        columns: columns.map((c) => c.trim()),
        rows: rows.map((r) => r.map((c) => c.trim())),
      }, opts?.anchor)
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

  const toolbar = readOnly
    ? el('div', { class: 'dt-toolbar' }, el('span', { class: 'dt-readonly-note' }, rv.simple === false ? 'Verschachtelte Relation — schreibgeschützt' : 'Schreibgeschützt'), status)
    : el('div', { class: 'dt-toolbar' }, addRow, saveBtn, status)
  const modal = el('div', { class: 'dt-modal rel-modal' }, header, el('div', { class: 'rel-body' }, grid), toolbar)
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
