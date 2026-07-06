import { getIterator, saveIterator, type Anchor, type IteratorView } from './api'
import { ensureFeel, validateExpr, validateName } from './feel'
import { attachFeelField } from './feelfield'

// openIteratorOverlay edits a decision's boxed iteration (WP-66): a `for` (which
// yields a list via its return branch) or a `some`/`every` quantifier (which
// yields a boolean via its satisfies branch). The kind, iterator variable,
// collection and body are edited, the body and collection validated live against
// the engine. baseNames are the in-scope variables; the body additionally sees
// the iterator variable. A branch that nests another boxed expression
// (iv.simple === false) opens read-only. onSaved gets the saved model's id.
export async function openIteratorOverlay(modelId: string, decisionId: string, baseNames: string[], onSaved?: (newModelId: string) => void, opts?: { readOnly?: boolean; anchor?: Anchor; at?: string }): Promise<void> {
  let iv: IteratorView | null = null
  try {
    iv = await getIterator(modelId, decisionId, opts?.anchor, opts?.at)
  } catch (e) {
    console.error(e)
    return
  }
  if (!iv) return
  void ensureFeel()

  const readOnly = !!opts?.readOnly || iv.simple === false
  let kind: 'for' | 'some' | 'every' = iv.kind
  let variable = iv.variable
  let inText = iv.in
  let body = iv.body

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
  const header = el('div', { class: 'dt-head' }, el('span', { class: 'dt-title' }, 'Iteration · ' + (iv.name || decisionId)), closeBtn)

  const status = el('span', { class: 'dt-status' })
  const saveBtn = el('button', { class: 'tbtn dt-save', type: 'button' }, 'Speichern') as HTMLButtonElement

  // Kind selector: for → return (a list), some/every → satisfies (a boolean).
  const kindSel = el('select', { class: 'ctx-type' }) as HTMLSelectElement
  for (const [val, label] of [['for', 'for — für jedes Element (Liste)'], ['every', 'every — alle erfüllen (boolean)'], ['some', 'some — mindestens eins (boolean)']] as [string, string][]) {
    const o = el('option', { value: val }, label) as HTMLOptionElement
    o.selected = kind === val
    kindSel.append(o)
  }
  kindSel.disabled = readOnly
  const nameIn = el('input', { class: 'iter-var', type: 'text', spellcheck: 'false', placeholder: 'x' }) as HTMLInputElement
  nameIn.value = variable
  nameIn.readOnly = readOnly
  const inTa = el('textarea', { class: 'cond-text', rows: '2', spellcheck: 'false', placeholder: 'Sammlung, z. B. [1, 2, 3] oder Eingabeliste' }) as HTMLTextAreaElement
  inTa.value = inText
  inTa.readOnly = readOnly
  const bodyTa = el('textarea', { class: 'cond-text', rows: '2', spellcheck: 'false' }) as HTMLTextAreaElement
  bodyTa.value = body
  bodyTa.readOnly = readOnly
  const bodyLabel = el('label', { class: 'cond-label' })

  const bodyLabelText = (): string => (kind === 'for' ? 'Rückgabe (return)' : 'Bedingung (satisfies)')
  const bodyPlaceholder = (): string => (kind === 'for' ? 'Ausdruck je Element, z. B. x * 2' : 'Bedingung je Element, z. B. x > 0')

  const grid = el('div', { class: 'cond-grid' })
  const rebuildLabels = (): void => {
    bodyLabel.textContent = bodyLabelText()
    bodyTa.placeholder = bodyPlaceholder()
  }
  grid.append(
    el('label', { class: 'cond-label' }, 'Art'), el('div', { class: 'cond-cell' }, kindSel),
    el('label', { class: 'cond-label' }, 'Variable'), el('div', { class: 'cond-cell' }, nameIn),
    el('label', { class: 'cond-label' }, 'Sammlung (in)'), el('div', { class: 'cond-cell' }, inTa),
    bodyLabel, el('div', { class: 'cond-cell' }, bodyTa),
  )
  rebuildLabels()

  // The body sees the iterator variable in addition to the model's names.
  const bodyScope = (): string[] => (variable.trim() ? [...baseNames, variable.trim()] : baseNames)
  attachFeelField(inTa, () => baseNames, { readOnly })
  attachFeelField(bodyTa, bodyScope, { readOnly })
  const check = (): boolean => {
    let firstErr = ''
    const nameRes = variable.trim() === '' ? { ok: false, message: 'darf nicht leer sein' } : validateName(variable.trim())
    nameIn.classList.toggle('cond-invalid', !nameRes.ok)
    if (!nameRes.ok) firstErr = 'Variable: ' + (nameRes.message ?? 'ungültig')
    const inRes = inText.trim() === '' ? { ok: false, message: 'darf nicht leer sein' } : validateExpr(inText.trim(), baseNames)
    inTa.classList.toggle('cond-invalid', !inRes.ok)
    if (!inRes.ok && !firstErr) firstErr = 'Sammlung (in): ' + (inRes.message ?? 'ungültig')
    const bodyRes = body.trim() === '' ? { ok: false, message: 'darf nicht leer sein' } : validateExpr(body.trim(), bodyScope())
    bodyTa.classList.toggle('cond-invalid', !bodyRes.ok)
    if (!bodyRes.ok && !firstErr) firstErr = bodyLabelText() + ': ' + (bodyRes.message ?? 'ungültig')
    status.className = 'dt-status' + (firstErr ? ' dt-error' : '')
    status.textContent = firstErr
    saveBtn.disabled = readOnly || !!firstErr
    return !firstErr
  }

  kindSel.addEventListener('change', () => {
    kind = kindSel.value as 'for' | 'some' | 'every'
    rebuildLabels()
    check()
  })
  nameIn.addEventListener('input', () => {
    variable = nameIn.value
    check()
  })
  inTa.addEventListener('input', () => {
    inText = inTa.value
    check()
  })
  bodyTa.addEventListener('input', () => {
    body = bodyTa.value
    check()
  })

  const save = async (): Promise<void> => {
    if (!check()) return
    saveBtn.disabled = true
    status.className = 'dt-status'
    status.textContent = 'speichert …'
    try {
      const saved = await saveIterator(modelId, decisionId, { kind, variable: variable.trim(), in: inText.trim(), body: body.trim() }, opts?.anchor, opts?.at)
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

  const toolbar = el('div', { class: 'dt-toolbar' }, saveBtn, status)
  const modal = el('div', { class: 'dt-modal cond-modal' }, header, el('div', { class: 'cond-body' }, grid), toolbar)
  overlay.append(modal)
  document.body.append(overlay)

  if (readOnly) {
    modal.classList.add('dt-readonly')
    saveBtn.style.display = 'none'
    if (iv.simple === false) {
      status.className = 'dt-status'
      status.textContent = 'Diese Iteration enthält verschachtelte Boxed-Ausdrücke und ist hier nur lesbar.'
    }
  } else {
    check()
    nameIn.focus()
  }
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
