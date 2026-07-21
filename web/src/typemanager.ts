import { listTypes, saveType, deleteType, type ItemType } from './api'
import { FEEL_TYPES } from './feeltypes'
import { ensureFeel, validateName } from './feel'

// openTypeManager shows the model's named types (item definitions) and lets the
// user add/edit/remove them (ADR-0016): a SIMPLE type is a base FEEL type with an
// optional collection flag and allowed-values constraint; a STRUCTURED type is a
// list of fields (name + type + collection), nested by referencing another named
// type. onChanged(newModelId) fires after each save/delete so the app can switch to
// the saved revision; the manager reloads in place so several edits chain without
// reopening.
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

  // The model's own named types are valid base types too (e.g. tClaimList is a
  // collection of tClaim), so they're offered in the type pickers alongside the
  // built-in FEEL types — the modeler appends the custom names (feeltypes.ts).
  let modelTypes: ItemType[] = []
  const reload = async (): Promise<void> => {
    let types: ItemType[] = []
    try {
      types = await listTypes(current)
    } catch (e) {
      status.className = 'dt-status dt-error'
      status.textContent = (e as Error).message
    }
    modelTypes = types
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

  // A struct's fields shown compactly in the list, e.g. "struct { name, alter }".
  function structSummary(t: ItemType): string {
    const fields = (t.components ?? []).map((c) => c.name).join(', ')
    return fields ? `struct { ${fields} }` : 'struct'
  }

  function renderList(types: ItemType[]): HTMLElement {
    if (!types.length) return el('p', { class: 'eval-empty' }, 'Noch keine eigenen Typen.')
    const table = el('table', { class: 'tm-list' })
    table.append(el('tr', { class: 'tm-head' }, th('Name'), th('Basis / Felder'), th('Collection'), th('Erlaubte Werte'), th('')))
    for (const t of types) {
      const basis = t.structured ? structSummary(t) : t.typeRef || '—'
      const row = el('tr', {}, td(t.name), td(basis), td(t.isCollection ? '☑' : ''), td(t.allowedValues || ''))
      const actions = el('td', { class: 'tm-actions' })
      // Both simple and structured types are editable now (WP: struct editor).
      actions.append(iconBtn('✎', 'Bearbeiten', () => fillForm(t)), iconBtn('🗑', 'Löschen', () => void apply(() => deleteType(current, t.name))))
      row.append(actions)
      table.append(row)
    }
    return table
  }

  // The add/edit form. Editing pre-fills it (saving upserts by name). A "Struktur"
  // toggle switches between the simple fields (base type + allowed values) and the
  // field editor for a structured type.
  let nameInput: HTMLInputElement
  let baseSel: HTMLSelectElement
  let collChk: HTMLInputElement
  let avInput: HTMLInputElement
  let structChk: HTMLInputElement
  let fieldRows: { name: HTMLInputElement; type: HTMLSelectElement; coll: HTMLInputElement }[] = []
  let fieldsHost: HTMLElement

  function renderForm(): HTMLElement {
    nameInput = el('input', { class: 'tm-field', placeholder: 'Typname (FEEL-Name)' }) as HTMLInputElement
    baseSel = el('select', { class: 'tm-field' }) as HTMLSelectElement
    fillTypeOptions(baseSel)
    collChk = el('input', { type: 'checkbox' }) as HTMLInputElement
    avInput = el('input', { class: 'tm-field', placeholder: 'Erlaubte Werte (FEEL), z. B. "rot","grün"' }) as HTMLInputElement
    structChk = el('input', { type: 'checkbox' }) as HTMLInputElement
    fieldRows = []
    fieldsHost = el('div', { class: 'tm-fields' })

    const nameCheck = (): void => {
      const s = nameInput.value.trim()
      nameInput.classList.toggle('tm-invalid', s !== '' && !validateName(s).ok)
    }
    nameInput.addEventListener('input', nameCheck)

    const baseRow = el('div', { class: 'tm-form-row' }, label('Basistyp'), baseSel)
    const avRow = el('div', { class: 'tm-form-row' }, label('Erlaubte Werte'), avInput)
    const addFieldBtn = el('button', { class: 'tbtn tm-add-field', type: 'button' }, '+ Feld') as HTMLButtonElement
    addFieldBtn.addEventListener('click', () => addFieldRow())
    const fieldsSection = el('div', { class: 'tm-form-row tm-fields-section' }, label('Felder'), el('div', { class: 'tm-fields-col' }, fieldsHost, addFieldBtn))

    const setMode = (struct: boolean): void => {
      baseRow.style.display = struct ? 'none' : ''
      avRow.style.display = struct ? 'none' : ''
      fieldsSection.style.display = struct ? '' : 'none'
      if (struct && fieldRows.length === 0) addFieldRow()
    }
    structChk.addEventListener('change', () => setMode(structChk.checked))

    const saveBtn = el('button', { class: 'tbtn dt-save', type: 'button' }, 'Typ speichern') as HTMLButtonElement
    saveBtn.addEventListener('click', () => {
      const name = nameInput.value.trim()
      if (!name || !validateName(name).ok) {
        nameInput.classList.add('tm-invalid')
        return
      }
      if (structChk.checked) {
        const components: ItemType[] = []
        for (const r of fieldRows) {
          const fn = r.name.value.trim()
          if (fn === '') continue
          if (!validateName(fn).ok) {
            r.name.classList.add('tm-invalid')
            return
          }
          components.push({ name: fn, typeRef: r.type.value, isCollection: r.coll.checked })
        }
        if (components.length === 0) {
          status.className = 'dt-status dt-error'
          status.textContent = 'Eine Struktur braucht mindestens ein Feld.'
          return
        }
        void apply(() => saveType(current, { name, isCollection: collChk.checked, components }))
      } else {
        void apply(() => saveType(current, { name, typeRef: baseSel.value, isCollection: collChk.checked, allowedValues: avInput.value.trim() }))
      }
    })

    setMode(false)
    return el(
      'div',
      { class: 'tm-form' },
      el('div', { class: 'tm-form-row' }, label('Name'), nameInput),
      el('div', { class: 'tm-form-row' }, label('Struktur'), structChk),
      baseRow,
      avRow,
      fieldsSection,
      el('div', { class: 'tm-form-row' }, label('Collection'), collChk),
      el('div', { class: 'tm-form-actions' }, saveBtn),
    )
  }

  // addFieldRow appends one editable struct field (name + type + collection).
  function addFieldRow(name = '', typeRef = '', isCollection = false): void {
    const fname = el('input', { class: 'tm-field tm-field-name', placeholder: 'Feldname' }) as HTMLInputElement
    fname.value = name
    fname.addEventListener('input', () => fname.classList.toggle('tm-invalid', fname.value.trim() !== '' && !validateName(fname.value.trim()).ok))
    const ftype = el('select', { class: 'tm-field tm-field-type' }) as HTMLSelectElement
    fillTypeOptions(ftype, nameInput.value.trim())
    ftype.value = typeRef
    const fcoll = el('input', { type: 'checkbox', title: 'Feld ist eine Collection (Liste)' }) as HTMLInputElement
    fcoll.checked = isCollection
    const entry = { name: fname, type: ftype, coll: fcoll }
    const rm = iconBtn('✕', 'Feld entfernen', () => {
      fieldRows = fieldRows.filter((r) => r !== entry)
      row.remove()
    })
    const row = el('div', { class: 'tm-field-row' }, fname, ftype, el('label', { class: 'tm-field-coll', title: 'Collection' }, fcoll, ' Liste'), rm)
    fieldRows.push(entry)
    fieldsHost.append(row)
  }

  // fillTypeOptions rebuilds a type dropdown from the built-in FEEL types plus the
  // model's own type names. exclude drops one name (the type being edited) so a
  // type can't be made its own base or field type.
  function fillTypeOptions(sel: HTMLSelectElement, exclude?: string): void {
    sel.textContent = ''
    for (const ft of FEEL_TYPES) sel.append(opt(ft, ft || '— Typ —'))
    for (const t of modelTypes) {
      if (t.name !== exclude) sel.append(opt(t.name, t.name))
    }
  }

  function fillForm(t: ItemType): void {
    nameInput.value = t.name
    collChk.checked = !!t.isCollection
    structChk.checked = !!t.structured
    // Rebuild the simple base picker excluding this type.
    fillTypeOptions(baseSel, t.name)
    baseSel.value = t.typeRef ?? ''
    avInput.value = t.allowedValues ?? ''
    // Rebuild the field editor for a structured type.
    fieldsHost.textContent = ''
    fieldRows = []
    if (t.structured) {
      for (const c of t.components ?? []) addFieldRow(c.name, c.typeRef ?? '', !!c.isCollection)
    }
    structChk.dispatchEvent(new Event('change'))
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
