import { listTypes, saveType, deleteType, type ItemType } from './api'
import { FEEL_TYPES } from './feeltypes'
import { ensureFeel, validateName } from './feel'

// openTypeManager shows the model's named types (item definitions) and lets the
// user add/edit/remove simple ones — a base FEEL type with an optional collection
// flag and allowed-values constraint (ADR-0016). Structured types (with
// components) are listed read-only. onChanged(newModelId) fires after each save/
// delete so the app can switch to the saved revision; the manager reloads in
// place so several edits chain without reopening.
export async function openTypeManager(modelId: string, onChanged: (newModelId: string) => Promise<void> | void): Promise<void> {
  void ensureFeel()
  let current = modelId

  const overlay = el('div', { class: 'dt-overlay' })
  overlay.addEventListener('click', (e) => {
    if (e.target === overlay) close()
  })
  const onKey = (e: KeyboardEvent): void => {
    if (e.key === 'Escape' && !overlay.querySelector('input:focus')) close()
  }
  function close(): void {
    overlay.remove()
    document.removeEventListener('keydown', onKey)
  }
  document.addEventListener('keydown', onKey)

  const closeBtn = el('button', { class: 'dt-close', type: 'button', title: 'Schließen (Esc)' }, '✕') as HTMLButtonElement
  closeBtn.addEventListener('click', close)
  const body = el('div', { class: 'tm-body' })
  const status = el('span', { class: 'dt-status' })
  overlay.append(el('div', { class: 'dt-modal tm-modal' }, el('div', { class: 'dt-head' }, el('span', { class: 'dt-title' }, 'Typen'), closeBtn), body, el('div', { class: 'dt-toolbar' }, status)))
  document.body.append(overlay)

  const reload = async (): Promise<void> => {
    let types: ItemType[] = []
    try {
      types = await listTypes(current)
    } catch (e) {
      status.className = 'dt-status dt-error'
      status.textContent = (e as Error).message
    }
    body.textContent = ''
    body.append(renderList(types), el('hr', { class: 'tm-sep' }), renderForm())
  }

  const apply = async (fn: () => Promise<{ modelId: string }>): Promise<void> => {
    status.className = 'dt-status'
    status.textContent = 'speichert …'
    try {
      const saved = await fn()
      current = saved.modelId
      await onChanged(saved.modelId)
      status.textContent = 'gespeichert ✓'
      await reload()
    } catch (e) {
      status.className = 'dt-status dt-error'
      status.textContent = (e as Error).message
    }
  }

  function renderList(types: ItemType[]): HTMLElement {
    if (!types.length) return el('p', { class: 'eval-empty' }, 'Noch keine eigenen Typen.')
    const table = el('table', { class: 'tm-list' })
    table.append(el('tr', { class: 'tm-head' }, th('Name'), th('Basis'), th('Collection'), th('Erlaubte Werte'), th('')))
    for (const t of types) {
      const row = el('tr', {}, td(t.name), td(t.typeRef || (t.structured ? 'struct' : '—')), td(t.isCollection ? '☑' : ''), td(t.allowedValues || ''))
      const actions = el('td', { class: 'tm-actions' })
      if (!t.structured) {
        actions.append(iconBtn('✎', 'Bearbeiten', () => fillForm(t)), iconBtn('🗑', 'Löschen', () => void apply(() => deleteType(current, t.name))))
      } else {
        actions.append(el('span', { class: 'tm-ro', title: 'Strukturierter Typ — hier schreibgeschützt' }, 'struct'))
      }
      row.append(actions)
      table.append(row)
    }
    return table
  }

  // The add/edit form. Editing pre-fills it (name read-only — saving upserts).
  let nameInput: HTMLInputElement
  let baseSel: HTMLSelectElement
  let collChk: HTMLInputElement
  let avInput: HTMLInputElement
  function renderForm(): HTMLElement {
    nameInput = el('input', { class: 'tm-field', placeholder: 'Typname (FEEL-Name)' }) as HTMLInputElement
    baseSel = el('select', { class: 'tm-field' }) as HTMLSelectElement
    for (const ft of FEEL_TYPES) baseSel.append(opt(ft, ft || '— Basis —'))
    collChk = el('input', { type: 'checkbox', id: 'tm-coll' }) as HTMLInputElement
    avInput = el('input', { class: 'tm-field', placeholder: 'Erlaubte Werte (FEEL), z. B. "rot","grün"' }) as HTMLInputElement

    const nameCheck = (): void => {
      const s = nameInput.value.trim()
      nameInput.classList.toggle('tm-invalid', s !== '' && !validateName(s).ok)
    }
    nameInput.addEventListener('input', nameCheck)

    const addBtn = el('button', { class: 'tbtn dt-save', type: 'button' }, 'Typ speichern') as HTMLButtonElement
    addBtn.addEventListener('click', () => {
      const name = nameInput.value.trim()
      if (!name || !validateName(name).ok) {
        nameInput.classList.add('tm-invalid')
        return
      }
      void apply(() => saveType(current, { name, typeRef: baseSel.value, isCollection: collChk.checked, allowedValues: avInput.value.trim() }))
    })

    return el(
      'div',
      { class: 'tm-form' },
      el('div', { class: 'tm-form-row' }, label('Name'), nameInput),
      el('div', { class: 'tm-form-row' }, label('Basistyp'), baseSel),
      el('div', { class: 'tm-form-row' }, label('Collection'), collChk),
      el('div', { class: 'tm-form-row' }, label('Erlaubte Werte'), avInput),
      el('div', { class: 'tm-form-actions' }, addBtn),
    )
  }

  function fillForm(t: ItemType): void {
    nameInput.value = t.name
    baseSel.value = t.typeRef ?? ''
    collChk.checked = !!t.isCollection
    avInput.value = t.allowedValues ?? ''
    nameInput.focus()
  }

  await reload()
}

function th(text: string): HTMLElement {
  return el('th', {}, text)
}
function td(text: string): HTMLElement {
  return el('td', {}, text)
}
function label(text: string): HTMLElement {
  return el('label', { class: 'tm-label' }, text)
}
function opt(value: string, text: string): HTMLOptionElement {
  return el('option', { value }, text) as HTMLOptionElement
}
function iconBtn(glyph: string, title: string, onClick: () => void): HTMLButtonElement {
  const b = el('button', { class: 'tm-icon', type: 'button', title }, glyph) as HTMLButtonElement
  b.addEventListener('click', onClick)
  return b
}

function el(tag: string, attrs: Record<string, string> = {}, ...children: (string | Node)[]): HTMLElement {
  const node = document.createElement(tag)
  for (const [k, v] of Object.entries(attrs)) {
    if (v !== '') node.setAttribute(k, v)
  }
  node.append(...children)
  return node
}
