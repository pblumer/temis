// A "deluxe" JSON editor (ADR-0016): everywhere a field accepts a FEEL/JSON value
// (the Evaluate form, the Flow run panel and the Flow designer's test form all
// coerce their inputs through JSON.parse), a small { } icon next to the field
// opens this roomy modal editor. It gives a monospace textarea with far more room
// than the one-line input, live JSON validation, and format/compact/copy tools —
// then writes the value back into the field on „Übernehmen".

// el is a tiny DOM builder: tag, attributes, then string/Node children.
function el(tag: string, attrs: Record<string, string> = {}, ...children: (string | Node)[]): HTMLElement {
  const n = document.createElement(tag)
  for (const [k, v] of Object.entries(attrs)) if (v !== '') n.setAttribute(k, v)
  n.append(...children)
  return n
}

// The { } braces glyph that marks a JSON-capable field — an inline SVG so it stays
// crisp and tints with the button state (see .je-open in style.css).
const BRACES_SVG =
  '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">' +
  '<path d="M8 4H7a2 2 0 0 0-2 2v3a2 2 0 0 1-2 2 2 2 0 0 1 2 2v3a2 2 0 0 0 2 2h1"/>' +
  '<path d="M16 4h1a2 2 0 0 1 2 2v3a2 2 0 0 0 2 2 2 2 0 0 0-2 2v3a2 2 0 0 1-2 2h-1"/></svg>'

// validate reports whether text is acceptable to save (empty or well-formed JSON)
// and, when it isn't, the parser's message — shown live under the textarea.
function validate(text: string): { ok: boolean; empty: boolean; error?: string } {
  const s = text.trim()
  if (s === '') return { ok: true, empty: true }
  try {
    JSON.parse(s)
    return { ok: true, empty: false }
  } catch (e) {
    return { ok: false, empty: false, error: (e as Error).message }
  }
}

// openJsonEditor opens the modal seeded with `value` (pretty-printed if it already
// parses as JSON) and resolves to the new text on „Übernehmen", or null when
// cancelled (Cancel, Esc or backdrop click). A saved value is returned compacted
// when it is valid JSON — the target is a single-line field — or empty to clear it.
export function openJsonEditor(opts: { value: string; title?: string }): Promise<string | null> {
  return new Promise((resolve) => {
    // Seed: pretty-print valid JSON so the user lands in a readable, editable
    // shape; otherwise show the raw text as-is (e.g. a half-typed value).
    let seed = opts.value ?? ''
    try {
      if (seed.trim() !== '') seed = JSON.stringify(JSON.parse(seed), null, 2)
    } catch {
      /* not JSON yet — leave the raw text for the user to fix */
    }

    const text = el('textarea', { class: 'je-text', spellcheck: 'false', placeholder: '{ }' }) as HTMLTextAreaElement
    text.value = seed
    const status = el('div', { class: 'je-status' })

    const fmtBtn = el('button', { class: 'je-tool', type: 'button', title: 'Einrücken und ordnen' }, 'Formatieren') as HTMLButtonElement
    const minBtn = el('button', { class: 'je-tool', type: 'button', title: 'In eine Zeile zusammenfalten' }, 'Kompakt') as HTMLButtonElement
    const copyBtn = el('button', { class: 'je-tool', type: 'button', title: 'In die Zwischenablage kopieren' }, 'Kopieren') as HTMLButtonElement

    const okBtn = el('button', { class: 'dlg-btn dlg-btn-primary', type: 'button' }, 'Übernehmen') as HTMLButtonElement
    const cancelBtn = el('button', { class: 'dlg-btn', type: 'button' }, 'Abbrechen') as HTMLButtonElement

    // sync refreshes the validity status line and gates the format/save actions.
    const sync = (): void => {
      const v = validate(text.value)
      okBtn.disabled = !v.ok
      fmtBtn.disabled = v.empty || !v.ok
      minBtn.disabled = v.empty || !v.ok
      if (v.empty) {
        status.textContent = 'Leer — übernehmen leert das Feld.'
        status.className = 'je-status'
      } else if (v.ok) {
        status.textContent = '✓ Gültiges JSON'
        status.className = 'je-status is-ok'
      } else {
        status.textContent = '✕ ' + v.error
        status.className = 'je-status is-err'
      }
    }

    // reflow rewrites the textarea with a re-serialised value (used by Formatieren
    // and Kompakt); a no-op when the current text is not valid JSON.
    const reflow = (indent: number): void => {
      const s = text.value.trim()
      if (s === '') return
      try {
        text.value = JSON.stringify(JSON.parse(s), null, indent)
      } catch {
        return
      }
      sync()
      text.focus()
    }

    let done = false
    const finish = (val: string | null): void => {
      if (done) return
      done = true
      overlay.remove()
      document.removeEventListener('keydown', onKey)
      resolve(val)
    }
    const submit = (): void => {
      const v = validate(text.value)
      if (!v.ok) return
      finish(v.empty ? '' : JSON.stringify(JSON.parse(text.value)))
    }
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') {
        e.preventDefault()
        finish(null)
      } else if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
        e.preventDefault()
        submit()
      }
    }

    // Tab inserts two spaces instead of leaving the field — expected in a code editor.
    text.addEventListener('keydown', (e) => {
      if (e.key === 'Tab') {
        e.preventDefault()
        const s = text.selectionStart
        const end = text.selectionEnd
        text.value = text.value.slice(0, s) + '  ' + text.value.slice(end)
        text.selectionStart = text.selectionEnd = s + 2
        sync()
      }
    })
    text.addEventListener('input', sync)
    fmtBtn.addEventListener('click', () => reflow(2))
    minBtn.addEventListener('click', () => reflow(0))
    copyBtn.addEventListener('click', () => {
      void navigator.clipboard?.writeText(text.value).then(
        () => {
          copyBtn.textContent = 'Kopiert ✓'
          window.setTimeout(() => (copyBtn.textContent = 'Kopieren'), 1200)
        },
        () => {
          /* clipboard blocked — ignore */
        },
      )
    })
    okBtn.addEventListener('click', submit)
    cancelBtn.addEventListener('click', () => finish(null))
    document.addEventListener('keydown', onKey)

    const modal = el(
      'div',
      { class: 'je-modal' },
      el(
        'div',
        { class: 'je-head' },
        el('span', { class: 'je-title' }, opts.title ?? 'JSON-Editor'),
        el('div', { class: 'je-tools' }, fmtBtn, minBtn, copyBtn),
      ),
      el('div', { class: 'je-body' }, text, status),
      el('div', { class: 'je-actions' }, cancelBtn, okBtn),
    )
    const overlay = el('div', { class: 'dt-overlay' }, modal)
    overlay.addEventListener('click', (e) => {
      if (e.target === overlay) finish(null)
    })
    document.body.append(overlay)
    sync()
    text.focus()
  })
}

// attachJsonEditor wraps an already-mounted text input with the { } opener button:
// clicking it opens the deluxe editor seeded from the field and, on „Übernehmen",
// writes the value back and fires input+change so the field's own listeners run.
// Call it after the input is in the DOM. Safe to call once per field.
export function attachJsonEditor(input: HTMLInputElement, opts: { title?: string } = {}): void {
  if (input.dataset.jeAttached === '1') return
  input.dataset.jeAttached = '1'

  const btn = el('button', {
    class: 'je-open',
    type: 'button',
    title: 'Im JSON-Editor öffnen',
    'aria-label': 'JSON-Editor öffnen',
  }) as HTMLButtonElement
  btn.innerHTML = BRACES_SVG

  // Wrap the input and the button in a flex row that takes the input's place, so
  // the button sits beside the field without overlapping its text.
  const row = el('span', { class: 'je-row' })
  input.replaceWith(row)
  row.append(input, btn)

  btn.addEventListener('click', () => {
    void openJsonEditor({ value: input.value, title: opts.title }).then((next) => {
      if (next === null) return
      input.value = next
      input.dispatchEvent(new Event('input', { bubbles: true }))
      input.dispatchEvent(new Event('change', { bubbles: true }))
      input.focus()
    })
  })
}
