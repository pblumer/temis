// A lightweight code-completion dropdown for the FEEL fields throughout the
// modeler (literal/BKM bodies and every decision-table cell). It makes the
// in-scope variables and the engine's built-in functions immediately visible:
// focus a field to see what's available, then type to filter. The function
// catalog comes straight from the real engine (src/feel.ts → cmd/feel-wasm), so
// it can never drift from what actually evaluates (ADR-0016).

import { builtins, ensureFeel } from './feel'

export type CompletionKind = 'variable' | 'function' | 'keyword'

// A single offered completion. label is what is shown and (by default) inserted;
// insert overrides the inserted text (e.g. `count(` for a function) and caretBack
// pulls the caret back from the end after inserting (e.g. 1 to land between `()`).
export type CompletionItem = {
  label: string
  kind: CompletionKind
  detail?: string
  insert?: string
  caretBack?: number
}

// The FEEL keywords worth completing — the control-flow and boolean words a user
// types by hand. Operators and punctuation are left out (you don't complete `+`).
const KEYWORDS = [
  'if', 'then', 'else', 'for', 'in', 'return', 'some', 'every', 'satisfies',
  'and', 'or', 'not', 'true', 'false', 'null', 'function', 'instance of', 'between',
]

// feelItems assembles the completion list for a FEEL field: the in-scope
// variables first (most relevant), then the engine's built-in functions, then the
// keywords. extra is for context-specific entries (e.g. the `?` input value in a
// decision-table input cell). Built-ins are absent until the wasm module loads;
// the list simply grows once it does.
export function feelItems(names: string[], extra: CompletionItem[] = []): CompletionItem[] {
  const items: CompletionItem[] = [...extra]
  const seen = new Set(extra.map((e) => e.label))
  for (const n of names) {
    if (n && !seen.has(n)) {
      seen.add(n)
      items.push({ label: n, kind: 'variable', detail: 'Variable' })
    }
  }
  for (const b of builtins()) {
    const sig = `${b.name}(${b.params.join(', ')}${b.variadic ? ' …' : ''})`
    items.push({ label: b.name, kind: 'function', detail: sig, insert: b.name + '(', caretBack: 0 })
  }
  for (const k of KEYWORDS) items.push({ label: k, kind: 'keyword', detail: 'Schlüsselwort', insert: k + ' ' })
  return items
}

// The trailing identifier the user is typing, and where it starts, so accepting a
// completion replaces exactly that token. FEEL names can contain spaces, but a
// completion is matched on the word at the caret; multi-word names complete from
// their first word.
function tokenAt(field: HTMLInputElement | HTMLTextAreaElement): { token: string; start: number; caret: number } {
  const caret = field.selectionStart ?? field.value.length
  const before = field.value.slice(0, caret)
  const m = before.match(/[A-Za-z_][A-Za-z0-9_]*$/)
  return m ? { token: m[0], start: caret - m[0].length, caret } : { token: '', start: caret, caret }
}

const ICON: Record<CompletionKind, string> = { variable: 'x', function: 'ƒ', keyword: 'K' }

// attachCompletion wires a completion dropdown onto a FEEL input/textarea. items
// is read lazily on each open, so callers can return a fresh list (e.g. the
// current decision-table input names) every time. The field's own value/events
// are otherwise untouched — accepting a completion dispatches an `input` event so
// existing live validation re-runs.
export function attachCompletion(
  field: HTMLInputElement | HTMLTextAreaElement,
  items: () => CompletionItem[],
): void {
  // Warm the engine so the function catalog is ready by the time it is opened.
  void ensureFeel()

  let pop: HTMLDivElement | null = null
  let rows: HTMLElement[] = []
  let active: CompletionItem[] = []
  let sel = 0
  let suppress = false // skip the reopen triggered by our own accept `input` event

  const close = (): void => {
    pop?.remove()
    pop = null
    rows = []
    active = []
  }

  const accept = (item: CompletionItem): void => {
    const { start, caret } = tokenAt(field)
    const insert = item.insert ?? item.label
    const before = field.value.slice(0, start)
    const after = field.value.slice(caret)
    field.value = before + insert + after
    const pos = before.length + insert.length - (item.caretBack ?? 0)
    field.setSelectionRange(pos, pos)
    // Re-run the field's own live validation, but keep this same edit from
    // immediately reopening the dropdown.
    suppress = true
    field.dispatchEvent(new Event('input', { bubbles: true }))
    suppress = false
    field.focus()
    close()
  }

  const highlight = (): void => {
    rows.forEach((r, i) => r.classList.toggle('cc-active', i === sel))
    rows[sel]?.scrollIntoView({ block: 'nearest' })
  }

  const open = (): void => {
    if (suppress) return
    const { token } = tokenAt(field)
    const t = token.toLowerCase()
    active = items().filter((it) => t === '' || it.label.toLowerCase().startsWith(t))
    if (!active.length) {
      close()
      return
    }
    if (!pop) {
      pop = document.createElement('div')
      pop.className = 'cc-pop'
      // Keep focus in the field while clicking a row.
      pop.addEventListener('mousedown', (e) => e.preventDefault())
      document.body.append(pop)
    }
    pop.textContent = ''
    rows = active.map((it, i) => {
      const row = document.createElement('div')
      row.className = 'cc-row'
      const icon = document.createElement('span')
      icon.className = 'cc-icon cc-' + it.kind
      icon.textContent = ICON[it.kind]
      const label = document.createElement('span')
      label.className = 'cc-label'
      label.textContent = it.label
      row.append(icon, label)
      if (it.detail) {
        const detail = document.createElement('span')
        detail.className = 'cc-detail'
        detail.textContent = it.detail
        row.append(detail)
      }
      row.addEventListener('mouseenter', () => {
        sel = i
        highlight()
      })
      row.addEventListener('click', () => accept(it))
      pop!.append(row)
      return row
    })
    sel = 0
    highlight()
    place()
  }

  // Anchor the dropdown to the field's lower-left, flipping above when it would
  // overflow the viewport bottom. Good enough for single-line inputs and the
  // modal textareas without tracking the caret's pixel position.
  const place = (): void => {
    if (!pop) return
    const r = field.getBoundingClientRect()
    pop.style.minWidth = Math.max(r.width, 180) + 'px'
    pop.style.left = r.left + window.scrollX + 'px'
    pop.style.top = r.bottom + window.scrollY + 2 + 'px'
    const ph = pop.offsetHeight
    if (r.bottom + ph + 6 > window.innerHeight && r.top - ph - 2 > 0) {
      pop.style.top = r.top + window.scrollY - ph - 2 + 'px'
    }
  }

  field.addEventListener('focus', open)
  field.addEventListener('click', open)
  field.addEventListener('input', open)
  field.addEventListener('blur', close)
  field.addEventListener('keydown', (ev) => {
    const e = ev as KeyboardEvent
    if (!pop) {
      // Ctrl/Cmd+Space forces the list open even on an empty field.
      if ((e.ctrlKey || e.metaKey) && e.key === ' ') {
        e.preventDefault()
        open()
      }
      return
    }
    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault()
        sel = (sel + 1) % active.length
        highlight()
        break
      case 'ArrowUp':
        e.preventDefault()
        sel = (sel - 1 + active.length) % active.length
        highlight()
        break
      case 'Enter':
      case 'Tab':
        e.preventDefault()
        e.stopPropagation()
        accept(active[sel])
        break
      case 'Escape':
        // Swallow Escape so the surrounding modal stays open; just close the list.
        e.preventDefault()
        e.stopPropagation()
        close()
        break
    }
  })
}
