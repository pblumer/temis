import { getConditional, saveConditional, type ConditionalView } from './api'
import { ensureFeel, validateExpr } from './feel'

// A branch of the conditional: its label, placeholder and current FEEL text.
type Branch = { key: 'if' | 'then' | 'else'; label: string; placeholder: string; text: string }

// openConditionalOverlay edits a decision's boxed conditional (WP-66): the three
// FEEL branches of an if/then/else, each validated live against the real engine
// and saved back into the model. baseNames are the in-scope variables the
// branches may reference (the other nodes' names). A conditional whose branches
// nest other boxed expressions (cv.simple === false) opens read-only, so the text
// editor never clobbers the nesting. onSaved gets the saved model's id.
export async function openConditionalOverlay(modelId: string, decisionId: string, baseNames: string[], onSaved?: (newModelId: string) => void, opts?: { readOnly?: boolean }): Promise<void> {
  let cv: ConditionalView | null = null
  try {
    cv = await getConditional(modelId, decisionId)
  } catch (e) {
    console.error(e)
    return
  }
  if (!cv) return
  void ensureFeel()

  const readOnly = !!opts?.readOnly || cv.simple === false
  const branches: Branch[] = [
    { key: 'if', label: 'Wenn (if)', placeholder: 'Bedingung, z. B. Alter >= 18', text: cv.if },
    { key: 'then', label: 'Dann (then)', placeholder: 'Wert wenn wahr, z. B. "Erwachsen"', text: cv.then },
    { key: 'else', label: 'Sonst (else)', placeholder: 'Wert wenn falsch, z. B. "Minderjährig"', text: cv.else },
  ]

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
  const header = el('div', { class: 'dt-head' }, el('span', { class: 'dt-title' }, 'Conditional · ' + (cv.name || decisionId)), closeBtn)

  const status = el('span', { class: 'dt-status' })
  const saveBtn = el('button', { class: 'tbtn dt-save', type: 'button' }, 'Speichern') as HTMLButtonElement

  const inputs = new Map<string, HTMLTextAreaElement>()
  const grid = el('div', { class: 'cond-grid' })
  for (const b of branches) {
    const ta = el('textarea', { class: 'cond-text', rows: '2', spellcheck: 'false', placeholder: b.placeholder }) as HTMLTextAreaElement
    ta.value = b.text
    ta.readOnly = readOnly
    ta.addEventListener('input', () => {
      b.text = ta.value
      check()
    })
    inputs.set(b.key, ta)
    grid.append(el('label', { class: 'cond-label' }, b.label), el('div', { class: 'cond-cell' }, ta))
  }

  // check validates every branch against the in-scope names, marks invalid fields
  // and reports the first problem; toggles the save button.
  const check = (): boolean => {
    let firstErr = ''
    for (const b of branches) {
      const ta = inputs.get(b.key)
      const res = b.text.trim() === '' ? { ok: false, message: 'darf nicht leer sein' } : validateExpr(b.text.trim(), baseNames)
      ta?.classList.toggle('cond-invalid', !res.ok)
      if (!res.ok && !firstErr) firstErr = b.label + ': ' + (res.message ?? 'ungültig')
    }
    status.className = 'dt-status' + (firstErr ? ' dt-error' : '')
    status.textContent = firstErr
    saveBtn.disabled = readOnly || !!firstErr
    return !firstErr
  }

  const save = async (): Promise<void> => {
    if (!check()) return
    saveBtn.disabled = true
    status.className = 'dt-status'
    status.textContent = 'speichert …'
    try {
      const saved = await saveConditional(modelId, decisionId, {
        if: branches[0].text.trim(),
        then: branches[1].text.trim(),
        else: branches[2].text.trim(),
      })
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

  if (readOnly) {
    modal.classList.add('dt-readonly')
    saveBtn.style.display = 'none'
    if (cv.simple === false) {
      status.className = 'dt-status'
      status.textContent = 'Dieser Conditional enthält verschachtelte Boxed-Ausdrücke und ist hier nur lesbar.'
    }
  }

  document.body.append(overlay)
  check()
  if (!readOnly) inputs.get('if')?.focus()
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
