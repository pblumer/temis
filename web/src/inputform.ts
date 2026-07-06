import type { InputField } from './api'

// coerce turns a raw form value into a JSON value: an empty box contributes
// nothing (undefined); otherwise try JSON (numbers, booleans, null, lists,
// objects) and fall back to the raw string for bare FEEL text like "2024-01-01".
// It lives here (re-exported from evaluate.ts and testimport.ts for their existing
// importers) so the "Auswerten" panel and the on-canvas input pills coerce typed
// input through exactly one rule.
export function coerce(raw: string): unknown {
  const s = raw.trim()
  if (s === '') return undefined
  try {
    return JSON.parse(s)
  } catch {
    return raw
  }
}

// FieldControl is one built input widget for a decision's leaf input: the element
// to place, any sibling nodes it needs (a datalist), and read/setValue/markInvalid
// helpers. Sharing it keeps the panel form and the on-node pills identical — a
// closed enumeration is a <select> of only its declared values; anything else is a
// text box coerced through JSON, with inferred cell values offered as suggestions
// and (opt-in) the deluxe JSON editor attached for structured/list values.
export type FieldControl = {
  field: InputField
  input: HTMLInputElement | HTMLSelectElement
  extras: Node[]
  read: () => unknown
  setValue: (v: unknown) => void
  markInvalid: (on: boolean, msg?: string) => void
}

// buildFieldControl builds the widget for one field. className is put on the input
// (so the panel keeps its `eval-field` styling and the pills get their own), and
// the invalid marker class is `<className>-invalid`. The caller is responsible for
// placing `input` (+ `extras`) in the DOM; a caller that wants the deluxe JSON
// editor beside a text field calls attachJsonEditor on `input` after inserting it
// (attaching wraps the input in place, so it must already have a parent).
export function buildFieldControl(field: InputField, opts: { idx: number; className: string }): FieldControl {
  const values = field.values ?? []
  const cls = opts.className
  let input: HTMLInputElement | HTMLSelectElement
  const extras: Node[] = []
  if (values.length && field.valuesClosed) {
    // Closed enumeration → a dropdown; only declared values are accepted.
    const sel = document.createElement('select')
    sel.className = cls
    const blank = document.createElement('option')
    blank.value = ''
    blank.textContent = '— wählen —'
    sel.append(blank)
    for (const v of values) {
      const o = document.createElement('option')
      o.value = v
      o.textContent = v
      sel.append(o)
    }
    input = sel
  } else {
    // Free text, with the inferred cell values offered as suggestions.
    const box = document.createElement('input')
    box.type = 'text'
    box.className = cls
    box.placeholder = field.type || 'FEEL'
    if (field.constraint) box.title = 'erlaubte Werte: ' + field.constraint
    if (values.length) {
      const id = cls + '-dl-' + opts.idx
      const dl = document.createElement('datalist')
      dl.id = id
      for (const v of values) {
        const o = document.createElement('option')
        o.value = v
        dl.append(o)
      }
      box.setAttribute('list', id)
      extras.push(dl)
    }
    input = box
  }
  const markInvalid = (on: boolean, msg?: string): void => {
    input.classList.toggle(cls + '-invalid', on)
    if (on) input.title = msg ?? 'ungültig'
    else input.title = field.constraint ? 'erlaubte Werte: ' + field.constraint : ''
  }
  const setValue = (v: unknown): void => {
    input.value = v === undefined || v === null ? '' : typeof v === 'string' ? v : JSON.stringify(v)
  }
  return { field, input, extras, read: () => coerce(input.value), setValue, markInvalid }
}
