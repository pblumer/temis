import { getInvocation, saveInvocation, type InvocationView } from './api'
import { ensureFeel, validateExpr, validateName } from './feel'

// openInvocationOverlay edits a decision's boxed invocation (WP-66): the called
// function/BKM and its parameter bindings (parameter name → FEEL argument), each
// argument validated live against the real engine and saved back into the model.
// Bindings can be added and removed. baseNames are the in-scope variables the
// arguments may reference. An invocation whose call or a binding nests another
// boxed expression (iv.simple === false) opens read-only, so the text editor
// never clobbers the nesting. onSaved gets the saved model's id.
export async function openInvocationOverlay(modelId: string, decisionId: string, baseNames: string[], onSaved?: (newModelId: string) => void, opts?: { readOnly?: boolean }): Promise<void> {
  let iv: InvocationView | null = null
  try {
    iv = await getInvocation(modelId, decisionId)
  } catch (e) {
    console.error(e)
    return
  }
  if (!iv) return
  void ensureFeel()

  const readOnly = !!opts?.readOnly || iv.simple === false
  let called = iv.called
  type Row = { parameter: string; value: string }
  const rows: Row[] = iv.bindings.map((b) => ({ parameter: b.parameter, value: b.value }))
  if (rows.length === 0) rows.push({ parameter: '', value: '' })

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
  const header = el('div', { class: 'dt-head' }, el('span', { class: 'dt-title' }, 'Invocation · ' + (iv.name || decisionId)), closeBtn)

  const status = el('span', { class: 'dt-status' })
  const saveBtn = el('button', { class: 'tbtn dt-save', type: 'button' }, 'Speichern') as HTMLButtonElement

  // Called function/BKM (a name).
  const calledIn = el('input', { class: 'inv-called', type: 'text', spellcheck: 'false', placeholder: 'Name der Funktion / BKM, z. B. Discount Rate' }) as HTMLInputElement
  calledIn.value = called
  calledIn.readOnly = readOnly
  calledIn.addEventListener('input', () => {
    called = calledIn.value
    check()
  })

  const rowsHost = el('div', { class: 'inv-rows' })

  // check validates the called name (non-empty), each binding's parameter (valid,
  // unique) and its argument (valid FEEL). A fully blank binding row is ignored.
  const check = (): boolean => {
    let firstErr = ''
    if (called.trim() === '') firstErr = 'Aufgerufene Funktion fehlt'
    calledIn.classList.toggle('inv-invalid', called.trim() === '')

    const params = rowsHost.querySelectorAll<HTMLInputElement>('.inv-param')
    const vals = rowsHost.querySelectorAll<HTMLInputElement>('.inv-value')
    const counts = new Map<string, number>()
    rows.forEach((r) => {
      const p = r.parameter.trim()
      if (p) counts.set(p, (counts.get(p) ?? 0) + 1)
    })
    rows.forEach((r, i) => {
      const p = r.parameter.trim()
      const v = r.value.trim()
      if (p === '' && v === '') {
        params[i]?.classList.remove('inv-invalid')
        vals[i]?.classList.remove('inv-invalid')
        return
      }
      const dup = p !== '' && (counts.get(p) ?? 0) > 1
      const pres = p === '' ? { ok: false, message: 'Parametername fehlt' } : validateName(p)
      const pbad = !pres.ok || dup
      params[i]?.classList.toggle('inv-invalid', pbad)
      if (pbad && !firstErr) firstErr = 'Binding ' + (i + 1) + ': ' + (dup ? 'Parameter doppelt' : pres.message ?? 'ungültig')
      const vres = v === '' ? { ok: false, message: 'Argument fehlt' } : validateExpr(v, baseNames)
      vals[i]?.classList.toggle('inv-invalid', !vres.ok)
      if (!vres.ok && !firstErr) firstErr = (p || 'Binding ' + (i + 1)) + ': ' + (vres.message ?? 'ungültig')
    })
    status.className = 'dt-status' + (firstErr ? ' dt-error' : '')
    status.textContent = firstErr
    saveBtn.disabled = readOnly || !!firstErr
    return !firstErr
  }

  const render = (): void => {
    rowsHost.replaceChildren()
    rows.forEach((row, i) => {
      const param = el('input', { class: 'inv-param', type: 'text', spellcheck: 'false', placeholder: 'Parameter' }) as HTMLInputElement
      param.value = row.parameter
      param.readOnly = readOnly
      param.addEventListener('input', () => {
        row.parameter = param.value
        check()
      })
      const val = el('input', { class: 'inv-value', type: 'text', spellcheck: 'false', placeholder: 'Argument (FEEL)' }) as HTMLInputElement
      val.value = row.value
      val.readOnly = readOnly
      val.addEventListener('input', () => {
        row.value = val.value
        check()
      })
      const del = el('button', { class: 'inv-del', type: 'button', title: 'Binding entfernen' }, '✕') as HTMLButtonElement
      del.disabled = readOnly || rows.length <= 1
      del.addEventListener('click', () => {
        rows.splice(i, 1)
        render()
      })
      rowsHost.append(el('div', { class: 'inv-row' }, param, el('span', { class: 'inv-arrow' }, '←'), val, readOnly ? document.createTextNode('') : del))
    })
    check()
  }

  const addBtn = el('button', { class: 'tbtn', type: 'button' }, '+ Binding') as HTMLButtonElement
  addBtn.addEventListener('click', () => {
    rows.push({ parameter: '', value: '' })
    render()
  })

  const save = async (): Promise<void> => {
    if (!check()) return
    saveBtn.disabled = true
    status.className = 'dt-status'
    status.textContent = 'speichert …'
    try {
      const saved = await saveInvocation(modelId, decisionId, {
        called: called.trim(),
        bindings: rows.map((r) => ({ parameter: r.parameter.trim(), value: r.value.trim() })),
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

  const body = el(
    'div',
    { class: 'inv-body' },
    el('label', { class: 'inv-called-label' }, 'Aufgerufene Funktion / BKM'),
    calledIn,
    el('div', { class: 'inv-bindings-title' }, 'Parameter-Bindings'),
    rowsHost,
  )
  const toolbar = readOnly
    ? el('div', { class: 'dt-toolbar' }, el('span', { class: 'dt-readonly-note' }, iv.simple === false ? 'Verschachtelte Invocation — schreibgeschützt' : 'Schreibgeschützt'), status)
    : el('div', { class: 'dt-toolbar' }, addBtn, saveBtn, status)
  const modal = el('div', { class: 'dt-modal inv-modal' }, header, body, toolbar)
  if (readOnly) modal.classList.add('dt-readonly')
  overlay.append(modal)
  document.body.append(overlay)
  render()
  if (!readOnly) calledIn.focus()
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
