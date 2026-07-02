// A lightweight code-completion dropdown for the FEEL fields throughout the
// modeler (literal/BKM bodies and every decision-table cell). It surfaces the
// in-scope variables and the engine's built-in functions: the list pops up under
// the caret as you type a word, or on demand with Ctrl/Cmd+Space — never just
// from entering or clicking a field. The function catalog comes straight from the
// real engine (src/feel.ts → cmd/feel-wasm), so it can never drift from what
// actually evaluates (ADR-0016).

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

// The style properties a mirror element must copy so its text lays out exactly
// like the field's — used to find the caret's pixel position.
const MIRROR_PROPS = [
  'boxSizing', 'width', 'borderTopWidth', 'borderRightWidth', 'borderBottomWidth', 'borderLeftWidth',
  'paddingTop', 'paddingRight', 'paddingBottom', 'paddingLeft',
  'fontFamily', 'fontSize', 'fontWeight', 'fontStyle', 'lineHeight', 'letterSpacing',
  'textAlign', 'textIndent', 'textTransform', 'wordSpacing', 'tabSize',
] as const

// caretCoords returns the caret's pixel offset within the field's border box
// (plus the line height), so the dropdown can be anchored right under the caret
// instead of far below the whole field. It mirrors the field into a hidden div
// (the well-known textarea-caret-position technique) and measures a marker span.
function caretCoords(field: HTMLInputElement | HTMLTextAreaElement, pos: number): { top: number; left: number; height: number } {
  const isInput = field.tagName === 'INPUT'
  const cs = getComputedStyle(field)
  const div = document.createElement('div')
  const s = div.style
  s.position = 'absolute'
  s.visibility = 'hidden'
  s.whiteSpace = isInput ? 'pre' : 'pre-wrap'
  s.wordWrap = 'break-word'
  s.overflow = 'hidden'
  for (const p of MIRROR_PROPS) s[p] = cs[p] as string
  // A single-line input never wraps and can be wider than its box; let the mirror
  // grow so the marker's offset reflects the (clipped) caret column.
  if (isInput) s.width = 'auto'
  const pre = field.value.slice(0, pos)
  div.textContent = isInput ? pre.replace(/ /g, ' ') : pre
  const marker = document.createElement('span')
  marker.textContent = field.value.slice(pos) || '.'
  div.appendChild(marker)
  document.body.appendChild(div)
  const top = marker.offsetTop
  const left = Math.min(marker.offsetLeft, field.clientWidth)
  const height = parseInt(cs.lineHeight) || parseInt(cs.fontSize) * 1.3
  document.body.removeChild(div)
  return { top, left, height }
}

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

  // manual = the user asked for it (Ctrl+Space): show everything even with no
  // token. On plain typing we only pop up once there is a word to complete, so
  // the list never appears just from entering or clicking the field.
  const open = (manual = false): void => {
    if (suppress) return
    const { token } = tokenAt(field)
    const t = token.toLowerCase()
    if (!manual && t === '') {
      close()
      return
    }
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

  // Anchor the dropdown just under the caret (not below the whole field), so it
  // stays close to where the user is typing. Flips above the caret line when it
  // would overflow the viewport bottom.
  const place = (): void => {
    if (!pop) return
    const caret = field.selectionStart ?? field.value.length
    const c = caretCoords(field, caret)
    const r = field.getBoundingClientRect()
    const left = r.left + window.scrollX + c.left - field.scrollLeft
    const lineTop = r.top + window.scrollY + c.top - field.scrollTop
    pop.style.minWidth = '180px'
    pop.style.top = lineTop + c.height + 2 + 'px'
    // Clamp horizontally so the (sometimes wide) list never runs off-screen.
    const pw = pop.offsetWidth
    const maxLeft = window.scrollX + window.innerWidth - pw - 6
    pop.style.left = Math.max(window.scrollX + 4, Math.min(left, maxLeft)) + 'px'
    const ph = pop.offsetHeight
    const caretViewportBottom = r.top + c.top - field.scrollTop + c.height
    if (caretViewportBottom + ph + 6 > window.innerHeight && r.top + c.top - field.scrollTop - ph - 2 > 0) {
      pop.style.top = lineTop - ph - 2 + 'px'
    }
  }

  field.addEventListener('input', () => open(false))
  field.addEventListener('blur', close)
  field.addEventListener('keydown', (ev) => {
    const e = ev as KeyboardEvent
    if (!pop) {
      // Ctrl/Cmd+Space forces the list open (showing everything on an empty word).
      if ((e.ctrlKey || e.metaKey) && e.key === ' ') {
        e.preventDefault()
        open(true)
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
