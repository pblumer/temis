import { getLiteral, saveLiteral, type LiteralView } from './api'
import { ensureFeel, validateExpr } from './feel'
import { attachFeelField } from './feelfield'
import { FEEL_TYPES } from './feeltypes'

// openLiteralOverlay shows a decision's literal FEEL expression in an editable
// modal (ADR-0016): a FEEL textarea validated live against the real engine plus a
// result-type dropdown, saved back into the model. When the decision has no
// literal yet (an undecided decision), the editor opens empty — saving creates
// it. names are the in-scope variables the expression may reference. onSaved gets
// the saved model's new id.
export async function openLiteralOverlay(modelId: string, decisionId: string, title: string, names: string[], onSaved?: (newModelId: string) => void, opts?: { fresh?: boolean; typeOptions?: string[]; readOnly?: boolean }): Promise<void> {
  const typeOptions = opts?.typeOptions ?? FEEL_TYPES
  let lit: LiteralView | null = null
  if (!opts?.fresh) {
    // fresh = an undecided decision with no literal yet; skip the fetch (and its
    // expected 404) and open an empty editor.
    try {
      lit = await getLiteral(modelId, decisionId)
    } catch (e) {
      console.error(e)
      return
    }
  }
  void ensureFeel()

  const close = (): void => {
    overlay.remove()
    document.removeEventListener('keydown', onKey)
  }
  const onKey = (e: KeyboardEvent): void => {
    if (e.key === 'Escape' && document.activeElement !== textarea) close()
  }
  document.addEventListener('keydown', onKey)

  const overlay = el('div', { class: 'dt-overlay' })
  overlay.addEventListener('click', (e) => {
    if (e.target === overlay) close()
  })

  const typeSel = el('select', { class: 'dt-type-sel lit-type', title: 'Ergebnistyp' }) as HTMLSelectElement
  const cur = lit?.typeRef ?? ''
  const typeList = cur && !typeOptions.includes(cur) ? [...typeOptions, cur] : typeOptions
  for (const t of typeList) {
    const o = el('option', { value: t }, t || '— beliebig —') as HTMLOptionElement
    o.selected = cur === t
    typeSel.append(o)
  }
  const closeBtn = el('button', { class: 'dt-close', type: 'button', title: 'Schließen (Esc)' }, '✕') as HTMLButtonElement
  closeBtn.addEventListener('click', close)
  const header = el('div', { class: 'dt-head' }, el('span', { class: 'dt-title' }, title || lit?.name || decisionId), el('span', { class: 'lit-type-label' }, 'Ergebnistyp'), typeSel, closeBtn)

  const textarea = el('textarea', { class: 'lit-text', spellcheck: 'false', placeholder: 'FEEL-Ausdruck, z. B. Unit Price * Quantity' }) as HTMLTextAreaElement
  textarea.value = lit?.text ?? ''
  const status = el('span', { class: 'dt-status' })
  const check = (): void => {
    const s = textarea.value.trim()
    const res = s === '' ? { ok: false, message: 'Ausdruck darf nicht leer sein' } : validateExpr(s, names)
    textarea.classList.toggle('lit-invalid', !res.ok)
    status.className = 'dt-status' + (res.ok ? '' : ' dt-error')
    status.textContent = res.ok ? '' : res.message ?? 'ungültig'
  }
  textarea.addEventListener('input', check)

  const saveBtn = el('button', { class: 'tbtn dt-save', type: 'button' }, 'Speichern') as HTMLButtonElement
  const save = async (): Promise<void> => {
    check()
    if (textarea.classList.contains('lit-invalid')) return
    saveBtn.disabled = true
    status.className = 'dt-status'
    status.textContent = 'speichert …'
    try {
      const saved = await saveLiteral(modelId, decisionId, textarea.value.trim(), typeSel.value)
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

  const body = el('div', { class: 'lit-body' }, textarea)
  const toolbar = el('div', { class: 'dt-toolbar' }, saveBtn, status)
  const modal = el('div', { class: 'dt-modal lit-modal' }, header, body, toolbar)
  overlay.append(modal)

  // Read-only (Operate): view the expression without editing/saving.
  if (opts?.readOnly) {
    modal.classList.add('dt-readonly')
    textarea.readOnly = true
    typeSel.disabled = true
    saveBtn.style.display = 'none'
  }

  document.body.append(overlay)
  check()
  attachFeelField(textarea, () => names, { readOnly: opts?.readOnly })
  if (!opts?.readOnly) textarea.focus()
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
