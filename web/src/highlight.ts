// Syntax highlighting for the FEEL fields throughout the modeler. Because a
// <textarea>/<input> can't render coloured text, we lay a transparent copy of
// the field over a backdrop that renders the same text as coloured token spans,
// scrolled in lock-step. The field keeps its border, background, caret and every
// event — only its glyphs are made transparent, so the coloured backdrop shows
// through exactly where the text is (ADR-0016).
//
// Tokenising is deliberately lightweight (not the full FEEL parser): it knows the
// in-scope variable names and the engine's built-in function names — the same
// sources the completion dropdown uses — plus strings, numbers and keywords. That
// is enough to colour functions, variables and keywords distinctly.

import { escapeHtml } from './dom'
import { builtins, ensureFeel } from './feel'

// FEEL keywords coloured as such (the words a user types; operators are not).
const KEYWORDS = new Set([
  'if', 'then', 'else', 'for', 'in', 'return', 'some', 'every', 'satisfies',
  'and', 'or', 'not', 'true', 'false', 'null', 'function', 'instance', 'of', 'between',
])

type Token = { text: string; cls: string }

const isIdentStart = (c: string): boolean => /[A-Za-z_]/.test(c)
const isIdent = (c: string): boolean => /[A-Za-z0-9_]/.test(c)


// tokenize splits FEEL source into coloured spans. Known names (variables and
// built-ins, possibly multi-word like "Guest Count" or "substring after") are
// matched greedily longest-first so a multi-word name stays one token.
function tokenize(text: string, names: string[], builtinSet: Set<string>): Token[] {
  // Known multi-word-capable names, longest first, each tagged with its class.
  const known: { name: string; cls: string }[] = [
    ...names.filter(Boolean).map((n) => ({ name: n, cls: 'hl-var' })),
    ...[...builtinSet].map((n) => ({ name: n, cls: 'hl-fn' })),
  ].sort((a, b) => b.name.length - a.name.length)

  const out: Token[] = []
  const n = text.length
  let i = 0
  while (i < n) {
    const c = text[i]
    if (c === '"') {
      // String literal (tolerate escapes; run to the closing quote or end).
      let j = i + 1
      while (j < n && text[j] !== '"') j += text[j] === '\\' ? 2 : 1
      j = Math.min(j + 1, n)
      out.push({ text: text.slice(i, j), cls: 'hl-str' })
      i = j
      continue
    }
    if (c >= '0' && c <= '9') {
      let j = i + 1
      while (j < n && /[0-9.]/.test(text[j])) j++
      out.push({ text: text.slice(i, j), cls: 'hl-num' })
      i = j
      continue
    }
    if (/\s/.test(c)) {
      let j = i + 1
      while (j < n && /\s/.test(text[j])) j++
      out.push({ text: text.slice(i, j), cls: '' })
      i = j
      continue
    }
    // Greedily match a known variable/built-in name at this position.
    let hit: { name: string; cls: string } | undefined
    for (const k of known) {
      if (text.startsWith(k.name, i)) {
        const after = text[i + k.name.length]
        if (after === undefined || !isIdent(after)) {
          hit = k
          break
        }
      }
    }
    if (hit) {
      out.push({ text: hit.name, cls: hit.cls })
      i += hit.name.length
      continue
    }
    if (isIdentStart(c)) {
      let j = i + 1
      while (j < n && isIdent(text[j])) j++
      const word = text.slice(i, j)
      let cls = ''
      if (KEYWORDS.has(word)) cls = 'hl-kw'
      else {
        // An unknown identifier directly calling with "(" is a function.
        let k = j
        while (k < n && text[k] === ' ') k++
        if (text[k] === '(') cls = 'hl-fn'
      }
      out.push({ text: word, cls })
      i = j
      continue
    }
    if (c === '?') {
      out.push({ text: c, cls: 'hl-var' })
      i++
      continue
    }
    // Operator / punctuation — one char at a time.
    out.push({ text: c, cls: 'hl-op' })
    i++
  }
  return out
}

function toHtml(text: string, names: string[], builtinSet: Set<string>): string {
  let html = ''
  for (const t of tokenize(text, names, builtinSet)) {
    const esc = escapeHtml(t.text)
    html += t.cls ? `<span class="${t.cls}">${esc}</span>` : esc
  }
  // A trailing newline is collapsed by the browser; pad so the backdrop's height
  // matches the textarea when the text ends on a blank line.
  return html + (text.endsWith('\n') ? '\n' : '')
}

// The style properties the backdrop must share with the field so the coloured
// text lands exactly over the (transparent) field text.
const COPY = [
  'boxSizing', 'borderTopWidth', 'borderRightWidth', 'borderBottomWidth', 'borderLeftWidth',
  'paddingTop', 'paddingRight', 'paddingBottom', 'paddingLeft',
  'fontFamily', 'fontSize', 'fontWeight', 'fontStyle', 'lineHeight', 'letterSpacing',
  'textAlign', 'textIndent', 'textTransform', 'wordSpacing', 'tabSize',
] as const

// attachHighlighter wraps a FEEL field with a coloured backdrop. names is read
// lazily on each render, so callers can return a live list (e.g. the current
// decision-table input names). Returns a refresh() to re-render on demand.
export function attachHighlighter(
  field: HTMLInputElement | HTMLTextAreaElement,
  names: () => string[],
): { refresh: () => void } {
  void ensureFeel()
  const isInput = field.tagName === 'INPUT'

  const wrap = document.createElement('div')
  wrap.className = 'hl-wrap'
  const backdrop = document.createElement('div')
  backdrop.className = 'hl-backdrop'
  const content = document.createElement('div')
  content.className = 'hl-content'
  backdrop.appendChild(content)

  field.parentNode?.insertBefore(wrap, field)
  // Backdrop first (behind), field on top: the field is made transparent so the
  // coloured backdrop shows through, while the field still owns the caret,
  // selection and every event.
  wrap.appendChild(backdrop)
  wrap.appendChild(field)

  const copyStyles = (): void => {
    const cs = getComputedStyle(field)
    for (const p of COPY) content.style[p] = cs[p] as string
    content.style.whiteSpace = isInput ? 'pre' : 'pre-wrap'
    if (!isInput) content.style.overflowWrap = 'break-word'
    // The backdrop carries the field's original background (so a decision-table
    // cell stays transparent and shows its trace tint, while a literal editor
    // keeps its off-white). The content's transparent border of the field's width
    // keeps the coloured text aligned with the field's text.
    backdrop.style.background = cs.backgroundColor
    content.style.borderStyle = 'solid'
    content.style.borderColor = 'transparent'
    field.classList.add('hl-field')
  }
  const builtinSet = (): Set<string> => new Set(builtins().map((b) => b.name))
  const refresh = (): void => {
    content.innerHTML = toHtml(field.value, names(), builtinSet())
  }
  const sync = (): void => {
    backdrop.scrollTop = field.scrollTop
    backdrop.scrollLeft = field.scrollLeft
  }

  // getComputedStyle needs the field in the document; if a caller attaches before
  // insertion (e.g. a freshly built grid), defer the first paint to the next frame.
  const init = (): void => {
    copyStyles()
    refresh()
    sync()
  }
  if (field.isConnected) init()
  else requestAnimationFrame(init)
  field.addEventListener('input', () => {
    refresh()
    sync()
  })
  field.addEventListener('scroll', sync)
  // The engine's built-in catalog arrives after the wasm module loads; re-render
  // once it is ready so function names light up.
  void ensureFeel().then(refresh).catch(() => {})
  return { refresh }
}
