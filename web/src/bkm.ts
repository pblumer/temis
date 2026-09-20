import { getBKM, saveBKM, type BKMView, type BKMParam } from './api'
import { ensureFeel, validateExpr, validateName, upsertModelFunction } from './feel'
import { attachFeelField } from './feelfield'
import { FEEL_TYPES } from './feeltypes'
import { openBoxed } from './boxededitors'

// openBKMOverlay edits a business knowledge model's encapsulated function (ADR-
// 0016): its formal parameters (name + type, add/remove) and a literal FEEL body,
// validated live against the parameter names. A BKM with a boxed (non-literal)
// body is shown read-only. onSaved gets the saved model's new id.
export async function openBKMOverlay(modelId: string, bkmId: string, onSaved?: (newModelId: string) => void, typeOptions: string[] = FEEL_TYPES): Promise<void> {
  let view: BKMView | null
  try {
    view = await getBKM(modelId, bkmId)
  } catch (e) {
    console.error(e)
    return
  }
  if (!view) return
  void ensureFeel()

  // Mutable working copy of the parameters. A BKM with no encapsulated logic yet
  // (e.g. one just dropped on the canvas) has no parameters — the server sends
  // params as null there, so default to an empty list.
  const params: BKMParam[] = (view.params ?? []).map((p) => ({ ...p }))

  // A boxed (non-literal) body opens in the matching boxed editor, anchored at the
  // BKM so it writes back to the encapsulated body (WP-66). The function's formal
  // parameters are the body's in-scope names. Only a literal or empty body uses
  // the simple parameter + FEEL editor below; an unknown boxed kind falls through
  // to the read-only note.
  if (!view.simple && openBoxedBody(modelId, bkmId, view.bodyKind ?? '', params.map((p) => p.name.trim()).filter((n) => n !== ''), onSaved, typeOptions)) {
    return
  }

  const close = (): void => {
    overlay.remove()
    document.removeEventListener('keydown', onKey)
  }
  const onKey = (e: KeyboardEvent): void => {
    if (e.key === 'Escape' && !overlay.querySelector('input:focus, textarea:focus')) close()
  }
  document.addEventListener('keydown', onKey)

  const overlay = el('div', { class: 'dt-overlay' })
  overlay.addEventListener('click', (e) => {
    if (e.target === overlay) close()
  })

  const typeSel = el('select', { class: 'dt-type-sel lit-type', title: 'Ergebnistyp' }) as HTMLSelectElement
  const cur = view.bodyTypeRef ?? ''
  for (const t of cur && !typeOptions.includes(cur) ? [...typeOptions, cur] : typeOptions) {
    const o = el('option', { value: t }, t || '— beliebig —') as HTMLOptionElement
    o.selected = cur === t
    typeSel.append(o)
  }
  const closeBtn = el('button', { class: 'dt-close', type: 'button', title: 'Schließen (Esc)' }, '✕') as HTMLButtonElement
  closeBtn.addEventListener('click', close)
  const header = el('div', { class: 'dt-head' }, el('span', { class: 'dt-title' }, (view.name || bkmId) + ' (BKM)'), el('span', { class: 'lit-type-label' }, 'Ergebnis'), typeSel, closeBtn)

  const body = el('div', { class: 'lit-body' })
  const status = el('span', { class: 'dt-status' })

  if (!view.simple) {
    body.append(el('p', { class: 'eval-empty' }, 'Diese BKM hat einen Boxed-Expression-Body dieses Typs — hier (noch) schreibgeschützt.'))
    overlay.append(el('div', { class: 'dt-modal lit-modal' }, header, body, el('div', { class: 'dt-toolbar' }, status)))
    document.body.append(overlay)
    return
  }

  // Parameter editor.
  const paramsHost = el('div', { class: 'bkm-params' })
  const textarea = el('textarea', { class: 'lit-text', spellcheck: 'false', placeholder: 'FEEL-Ausdruck über die Parameter, z. B. if total > 1000 then 0.2 else 0.1' }) as HTMLTextAreaElement
  textarea.value = view.bodyText

  // Set once the highlighter is attached; re-render highlighting when a parameter
  // is renamed (which doesn't fire the textarea's own input event).
  let hlRefresh: (() => void) | null = null
  const paramNames = (): string[] => params.map((p) => p.name.trim()).filter((n) => n !== '')
  const checkBody = (): void => {
    // Keep this BKM registered as a function of the model, with its current
    // parameters, so a recursive call in its own body (e.g. fact(n - 1) inside
    // fact) resolves as a known function and is offered in completion — even for
    // a just-created BKM the last model load did not yet know about.
    if (view.name) upsertModelFunction({ name: view.name, params: paramNames() })
    const s = textarea.value.trim()
    const res = s === '' ? { ok: false, message: 'Body darf nicht leer sein' } : validateExpr(s, paramNames())
    textarea.classList.toggle('lit-invalid', !res.ok)
    status.className = 'dt-status' + (res.ok ? '' : ' dt-error')
    status.textContent = res.ok ? '' : res.message ?? 'ungültig'
    hlRefresh?.()
  }
  textarea.addEventListener('input', checkBody)

  const renderParams = (): void => {
    paramsHost.textContent = ''
    paramsHost.append(el('div', { class: 'bkm-params-title' }, 'Parameter'))
    params.forEach((p, i) => {
      const name = el('input', { class: 'bkm-pname', value: p.name, placeholder: 'Name' }) as HTMLInputElement
      name.addEventListener('input', () => {
        p.name = name.value
        name.classList.toggle('tm-invalid', name.value.trim() !== '' && !validateName(name.value.trim()).ok)
        checkBody()
      })
      const type = el('select', { class: 'bkm-ptype' }) as HTMLSelectElement
      for (const t of p.typeRef && !typeOptions.includes(p.typeRef) ? [...typeOptions, p.typeRef] : typeOptions) type.append(option(t, t || '— beliebig —', (p.typeRef ?? '') === t))
      type.addEventListener('change', () => {
        p.typeRef = type.value
      })
      const del = el('button', { class: 'tm-icon', type: 'button', title: 'Parameter entfernen' }, '🗑') as HTMLButtonElement
      del.addEventListener('click', () => {
        params.splice(i, 1)
        renderParams()
        checkBody()
      })
      paramsHost.append(el('div', { class: 'bkm-param-row' }, name, type, del))
    })
    const add = el('button', { class: 'tbtn bkm-addparam', type: 'button' }, '+ Parameter') as HTMLButtonElement
    add.addEventListener('click', () => {
      params.push({ name: '', typeRef: '' })
      renderParams()
    })
    paramsHost.append(add)
  }
  renderParams()

  body.append(paramsHost, el('div', { class: 'bkm-body-title' }, 'Body (FEEL)'), textarea)

  const saveBtn = el('button', { class: 'tbtn dt-save', type: 'button' }, 'Speichern') as HTMLButtonElement
  const save = async (): Promise<void> => {
    checkBody()
    if (textarea.classList.contains('lit-invalid') || paramsHost.querySelector('.tm-invalid')) {
      if (!status.textContent) {
        status.className = 'dt-status dt-error'
        status.textContent = 'Bitte die rot markierten Felder korrigieren.'
      }
      return
    }
    saveBtn.disabled = true
    status.className = 'dt-status'
    status.textContent = 'speichert …'
    try {
      const saved = await saveBKM(modelId, bkmId, { params, bodyText: textarea.value.trim(), bodyTypeRef: typeSel.value })
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

  overlay.append(el('div', { class: 'dt-modal lit-modal' }, header, body, el('div', { class: 'dt-toolbar' }, saveBtn, status)))
  document.body.append(overlay)
  // Highlighting + completion over the BKM's own parameters (read live so newly
  // added/renamed parameters appear immediately) plus the engine's built-ins.
  hlRefresh = attachFeelField(textarea, paramNames).refresh
  checkBody()
}

function option(value: string, text: string, selected: boolean): HTMLOptionElement {
  const o = el('option', { value }, text) as HTMLOptionElement
  o.selected = selected
  return o
}

function el(tag: string, attrs: Record<string, string> = {}, ...children: (string | Node)[]): HTMLElement {
  const node = document.createElement(tag)
  for (const [k, v] of Object.entries(attrs)) {
    if (v !== '') node.setAttribute(k, v)
  }
  node.append(...children)
  return node
}

// openBoxedBody opens the boxed editor matching kind on a BKM's encapsulated body,
// anchored so edits write back to the body (WP-66). names are the function's
// formal parameters (the body's in-scope variables). It returns false for a kind
// with no editor (e.g. a nested function), so the caller can fall back to a
// read-only note.
function openBoxedBody(modelId: string, bkmId: string, kind: string, names: string[], onSaved?: (newModelId: string) => void, typeOptions: string[] = FEEL_TYPES): boolean {
  return openBoxed(kind, { modelId, anchor: { kind: 'bkm', id: bkmId }, names, onSaved, typeOptions })
}
