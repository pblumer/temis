import { getList, saveList, type ListView } from './api'
import { ensureFeel, validateExpr } from './feel'

// openListOverlay edits a decision's boxed list (WP-66): an ordered list of FEEL
// item expressions, each validated live against the real engine and saved back
// into the model. baseNames are the in-scope variables the items may reference
// (the other nodes' names). A list whose items nest other boxed expressions
// (lv.simple === false) opens read-only, so the text editor never clobbers the
// nesting. onSaved gets the saved model's id.
export async function openListOverlay(modelId: string, decisionId: string, baseNames: string[], onSaved?: (newModelId: string) => void, opts?: { readOnly?: boolean }): Promise<void> {
  let lv: ListView | null = null
  try {
    lv = await getList(modelId, decisionId)
  } catch (e) {
    console.error(e)
    return
  }
  if (!lv) return
  void ensureFeel()

  const readOnly = !!opts?.readOnly || lv.simple === false
  const items: string[] = lv.items.length ? [...lv.items] : ['']

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
  const header = el('div', { class: 'dt-head' }, el('span', { class: 'dt-title' }, 'Liste · ' + (lv.name || decisionId)), closeBtn)

  const status = el('span', { class: 'dt-status' })
  const saveBtn = el('button', { class: 'tbtn dt-save', type: 'button' }, 'Speichern') as HTMLButtonElement
  const rows = el('div', { class: 'list-rows' })

  // check validates every item against the in-scope names, marks invalid fields
  // and reports the first problem; toggles the save button.
  const check = (): boolean => {
    let firstErr = ''
    const cells = rows.querySelectorAll<HTMLInputElement>('.list-item')
    items.forEach((it, i) => {
      const cell = cells[i]
      const res = it.trim() === '' ? { ok: false, message: 'darf nicht leer sein' } : validateExpr(it.trim(), baseNames)
      cell?.classList.toggle('list-invalid', !res.ok)
      if (!res.ok && !firstErr) firstErr = 'Eintrag ' + (i + 1) + ': ' + (res.message ?? 'ungültig')
    })
    status.className = 'dt-status' + (firstErr ? ' dt-error' : '')
    status.textContent = firstErr
    saveBtn.disabled = readOnly || !!firstErr
    return !firstErr
  }

  const render = (): void => {
    rows.replaceChildren()
    items.forEach((it, i) => {
      const idx = el('span', { class: 'list-idx' }, String(i + 1))
      const input = el('input', { class: 'list-item', type: 'text', spellcheck: 'false', placeholder: 'FEEL-Ausdruck, z. B. "rot"' }) as HTMLInputElement
      input.value = it
      input.readOnly = readOnly
      input.addEventListener('input', () => {
        items[i] = input.value
        check()
      })
      const del = el('button', { class: 'list-del', type: 'button', title: 'Eintrag entfernen' }, '✕') as HTMLButtonElement
      del.disabled = readOnly || items.length <= 1
      del.addEventListener('click', () => {
        items.splice(i, 1)
        render()
      })
      rows.append(el('div', { class: 'list-row' }, idx, input, readOnly ? document.createTextNode('') : del))
    })
    check()
  }

  const addBtn = el('button', { class: 'tbtn', type: 'button' }, '+ Eintrag') as HTMLButtonElement
  addBtn.addEventListener('click', () => {
    items.push('')
    render()
    ;(rows.querySelector('.list-row:last-child .list-item') as HTMLInputElement | null)?.focus()
  })

  const save = async (): Promise<void> => {
    if (!check()) return
    saveBtn.disabled = true
    status.className = 'dt-status'
    status.textContent = 'speichert …'
    try {
      const saved = await saveList(modelId, decisionId, { items: items.map((s) => s.trim()).filter((s) => s !== '') })
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
    ? el('div', { class: 'dt-toolbar' }, el('span', { class: 'dt-readonly-note' }, lv.simple === false ? 'Verschachtelte Liste — schreibgeschützt' : 'Schreibgeschützt'), status)
    : el('div', { class: 'dt-toolbar' }, addBtn, saveBtn, status)
  const modal = el('div', { class: 'dt-modal list-modal' }, header, el('div', { class: 'list-body' }, rows), toolbar)
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
