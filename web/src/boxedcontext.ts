import { getContext, saveContext, type Anchor, type ContextView, type ContextEntryView } from './api'
import { ensureFeel, validateExpr, validateName } from './feel'
import { attachFeelField } from './feelfield'
import { FEEL_TYPES } from './feeltypes'
import { openBoxed, joinAt } from './boxededitors'

// A working row in the editor: a named entry's name, declared type and FEEL text.
// childKind (with index) is set when the entry's value is a nested boxed
// expression rather than a literal — that row shows a drill-in instead of a text
// field, and text stays empty (WP-66 Phase 2).
type Row = { name: string; typeRef: string; text: string; childKind?: string; index: number }

// openBoxedContextOverlay edits a decision's boxed context (WP-66): an ordered
// list of `name = FEEL` entries plus an optional result-cell expression, each
// validated live against the real FEEL engine and saved back into the model.
// baseNames are the in-scope variables the expressions may reference (the other
// nodes' names); later entries also see the earlier entry names. A context whose
// entries nest other boxed expressions (cv.simple === false) opens read-only, so
// the text editor never clobbers the nesting. onSaved gets the saved model's id.
export async function openBoxedContextOverlay(modelId: string, decisionId: string, baseNames: string[], onSaved?: (newModelId: string) => void, opts?: { typeOptions?: string[]; readOnly?: boolean; anchor?: Anchor; at?: string }): Promise<void> {
  const typeOptions = opts?.typeOptions ?? FEEL_TYPES
  let cv: ContextView | null = null
  try {
    cv = await getContext(modelId, decisionId, opts?.anchor, opts?.at)
  } catch (e) {
    console.error(e)
    return
  }
  if (!cv) return
  void ensureFeel()

  // explicitReadOnly is the caller's intent (e.g. Operate mode); a context is also
  // rendered read-only when it nests boxed entries (cv.simple === false) — but its
  // nested entries can still be drilled into and edited unless explicitly read-only.
  const explicitReadOnly = !!opts?.readOnly
  const readOnly = explicitReadOnly || cv.simple === false
  const rows: Row[] = cv.entries.map((e: ContextEntryView, i: number) => ({ name: e.name, typeRef: e.typeRef ?? '', text: e.text, childKind: e.childKind, index: e.index ?? i }))
  const anchor: Anchor = opts?.anchor ?? { kind: 'decision', id: decisionId }
  let resultText = cv.result ?? ''
  let resultType = cv.resultTypeRef ?? ''

  // openChild drills into a nested boxed entry, opening the matching editor at the
  // entry's locator (the parent path extended by entry.N). Saving the child yields
  // a new revision, so it reselects and closes this (now-stale) parent overlay.
  const openChild = (row: Row, i: number): void => {
    if (!row.childKind) return
    openBoxed(row.childKind, {
      modelId,
      anchor,
      at: joinAt(opts?.at, 'entry.' + row.index),
      // The nested value sees the same names as the entry itself: the base names
      // plus the entries declared before it.
      names: scopeFor(i),
      onSaved: (id) => {
        onSaved?.(id)
        close()
      },
      typeOptions,
      readOnly: explicitReadOnly,
    })
  }

  const close = (): void => {
    overlay.remove()
    document.removeEventListener('keydown', onKey)
  }
  const onKey = (e: KeyboardEvent): void => {
    const tag = (document.activeElement?.tagName ?? '').toLowerCase()
    if (e.key === 'Escape' && tag !== 'input' && tag !== 'textarea' && tag !== 'select') close()
  }
  document.addEventListener('keydown', onKey)

  const overlay = el('div', { class: 'dt-overlay' })
  overlay.addEventListener('click', (e) => {
    if (e.target === overlay) close()
  })

  const closeBtn = el('button', { class: 'dt-close', type: 'button', title: 'Schließen (Esc)' }, '✕') as HTMLButtonElement
  closeBtn.addEventListener('click', close)
  const header = el('div', { class: 'dt-head' }, el('span', { class: 'dt-title' }, 'Boxed Context · ' + (cv.name || decisionId)), closeBtn)

  const status = el('span', { class: 'dt-status' })
  const grid = el('div', { class: 'ctx-grid' })
  const saveBtn = el('button', { class: 'tbtn dt-save', type: 'button' }, 'Speichern') as HTMLButtonElement

  // typeSelect builds a result-type dropdown over the built-in/custom types.
  const typeSelect = (cur: string): HTMLSelectElement => {
    const sel = el('select', { class: 'ctx-type', title: 'Typ' }) as HTMLSelectElement
    const list = cur && !typeOptions.includes(cur) ? [...typeOptions, cur] : typeOptions
    for (const t of list) {
      const o = el('option', { value: t }, t || '— Typ —') as HTMLOptionElement
      o.selected = cur === t
      sel.append(o)
    }
    return sel
  }

  // render redraws the entry rows from `rows`, then validates.
  const render = (): void => {
    grid.innerHTML = ''
    grid.append(
      el('div', { class: 'ctx-head' }, 'Name'),
      el('div', { class: 'ctx-head' }, 'Typ'),
      el('div', { class: 'ctx-head' }, 'Ausdruck (FEEL)'),
      el('div', { class: 'ctx-head' }, ''),
    )
    rows.forEach((row, i) => {
      const nameIn = el('input', { class: 'ctx-name', value: row.name, placeholder: 'Name', spellcheck: 'false' }) as HTMLInputElement
      nameIn.value = row.name
      nameIn.addEventListener('input', () => {
        row.name = nameIn.value
        check()
      })
      const typeSel = typeSelect(row.typeRef)
      typeSel.addEventListener('change', () => {
        row.typeRef = typeSel.value
      })
      const boxed = !!row.childKind
      const exprIn = el('input', { class: 'ctx-expr', value: row.text, placeholder: boxed ? '‹' + row.childKind + '›' : 'z. B. Points * 2', spellcheck: 'false' }) as HTMLInputElement
      exprIn.value = row.text
      exprIn.addEventListener('input', () => {
        row.text = exprIn.value
        check()
      })
      const del = el('button', { class: 'ctx-del', type: 'button', title: 'Eintrag entfernen' }, '✕') as HTMLButtonElement
      del.addEventListener('click', () => {
        rows.splice(i, 1)
        render()
      })
      if (readOnly || boxed) {
        nameIn.disabled = typeSel.disabled = exprIn.disabled = del.disabled = true
      }
      // A nested boxed entry shows a drill-in (open the matching editor at entry.N)
      // instead of the delete action; a literal entry keeps its delete + highlighter.
      let action: HTMLElement = del
      if (boxed) {
        const drill = el('button', { class: 'tbtn ctx-drill', type: 'button', title: 'Verschachtelten Ausdruck bearbeiten' }, '✎ ' + row.childKind) as HTMLButtonElement
        drill.addEventListener('click', () => openChild(row, i))
        action = drill
      }
      grid.append(nameIn, typeSel, exprIn, action)
      // The entry expression sees the base names plus the earlier entries.
      if (!boxed) attachFeelField(exprIn, () => scopeFor(i), { readOnly })
    })
    check()
  }

  // names in scope for the entry at index i: the base names plus all earlier
  // entry names (a context entry sees the ones declared above it).
  const scopeFor = (i: number): string[] => [...baseNames, ...rows.slice(0, i).map((r) => r.name).filter((n) => n.trim() !== '')]
  const allNames = (): string[] => [...baseNames, ...rows.map((r) => r.name).filter((n) => n.trim() !== '')]

  // check validates every name and expression, marks invalid fields and reports
  // the first problem; returns whether everything is valid.
  const resultIn = el('input', { class: 'ctx-expr', value: resultText, placeholder: 'Ergebnis (optional), z. B. Bonus', spellcheck: 'false' }) as HTMLInputElement
  const resultTypeSel = typeSelect(resultType)
  const check = (): boolean => {
    // A read-only context (Operate, or one nesting boxed entries) is not validated
    // for saving — its fields aren't editable and a nested entry's placeholder must
    // not read as an empty-expression error. Just guide the user to the drill-ins.
    if (readOnly) {
      const hasNested = rows.some((r) => r.childKind)
      status.className = 'dt-status'
      status.textContent = hasNested ? 'Verschachtelte Einträge über „✎" bearbeiten.' : ''
      return true
    }
    let firstErr = ''
    const nameEls = grid.querySelectorAll<HTMLInputElement>('.ctx-name')
    const exprEls = grid.querySelectorAll<HTMLInputElement>('.ctx-expr')
    // Count each trimmed entry name so duplicates can be flagged: a context is a
    // map, so two same-named entries silently clobber each other (the engine keeps
    // the last, with no error) — catch it here before the value goes wrong.
    const nameCounts = new Map<string, number>()
    for (const row of rows) {
      const n = row.name.trim()
      if (n !== '') nameCounts.set(n, (nameCounts.get(n) ?? 0) + 1)
    }
    rows.forEach((row, i) => {
      const nameEl = nameEls[i]
      const exprEl = exprEls[i]
      const trimmed = row.name.trim()
      const dup = trimmed !== '' && (nameCounts.get(trimmed) ?? 0) > 1
      const nameRes = trimmed === '' ? { ok: false, message: 'Name darf nicht leer sein' } : validateName(trimmed)
      nameEl?.classList.toggle('ctx-invalid', !nameRes.ok || dup)
      if (!nameRes.ok && !firstErr) firstErr = 'Name „' + row.name + '": ' + (nameRes.message ?? 'ungültig')
      else if (dup && !firstErr) firstErr = 'Name „' + trimmed + '" doppelt — Eintragsnamen müssen eindeutig sein'
      const exprRes = row.text.trim() === '' ? { ok: false, message: 'Ausdruck darf nicht leer sein' } : validateExpr(row.text.trim(), scopeFor(i))
      exprEl?.classList.toggle('ctx-invalid', !exprRes.ok)
      if (!exprRes.ok && !firstErr) firstErr = (row.name || 'Eintrag ' + (i + 1)) + ': ' + (exprRes.message ?? 'ungültig')
    })
    // The result cell is optional; validate only when filled.
    const r = resultIn.value.trim()
    const resOk = r === '' ? true : validateExpr(r, allNames()).ok
    resultIn.classList.toggle('ctx-invalid', !resOk)
    if (!resOk && !firstErr) firstErr = 'Ergebnis: ' + (validateExpr(r, allNames()).message ?? 'ungültig')
    const empty = rows.length === 0 && r === ''
    if (empty && !firstErr) firstErr = 'Ein Context braucht mindestens einen Eintrag oder ein Ergebnis.'
    status.className = 'dt-status' + (firstErr ? ' dt-error' : '')
    status.textContent = firstErr
    return !firstErr
  }
  resultIn.addEventListener('input', check)
  resultTypeSel.addEventListener('change', () => {
    resultType = resultTypeSel.value
  })

  const addBtn = el('button', { class: 'tbtn', type: 'button' }, '+ Eintrag') as HTMLButtonElement
  addBtn.addEventListener('click', () => {
    rows.push({ name: 'Eintrag ' + (rows.length + 1), typeRef: '', text: '', index: rows.length })
    render()
  })

  const save = async (): Promise<void> => {
    resultText = resultIn.value.trim()
    if (!check()) return
    saveBtn.disabled = true
    status.className = 'dt-status'
    status.textContent = 'speichert …'
    try {
      const saved = await saveContext(modelId, decisionId, {
        entries: rows.map((r) => ({ name: r.name.trim(), text: r.text.trim(), typeRef: r.typeRef })),
        result: resultText,
        resultTypeRef: resultText ? resultType : '',
      }, opts?.anchor, opts?.at)
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

  const resultRow = el('div', { class: 'ctx-result' }, el('span', { class: 'ctx-result-label' }, 'Ergebnis'), resultTypeSel, resultIn)
  // The result cell sees every entry name (all in-scope names).
  attachFeelField(resultIn, allNames, { readOnly })
  const toolbar = el('div', { class: 'dt-toolbar' }, addBtn, saveBtn, status)
  const body = el('div', { class: 'ctx-body' }, grid, resultRow)
  const modal = el('div', { class: 'dt-modal ctx-modal' }, header, body, toolbar)
  overlay.append(modal)

  if (readOnly) {
    modal.classList.add('dt-readonly')
    addBtn.style.display = 'none'
    saveBtn.style.display = 'none'
    resultIn.disabled = resultTypeSel.disabled = true
    if (cv.simple === false) {
      status.className = 'dt-status'
      status.textContent = 'Dieser Context enthält verschachtelte Boxed-Ausdrücke und ist hier nur lesbar.'
    }
  }

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
