// A translucent "what is expected here" hint that floats just above a FEEL field
// while the caret sits inside a function call or a control-flow construct. It is
// deliberately a beginner aid: it never inserts or changes anything, it only
// spells out the shape the engine expects — `count(list)`, or
// `if Bedingung then Wert else anderer Wert` — with the part the caret is
// currently on picked out. The catalog of function signatures comes straight
// from the engine (src/feel.ts → cmd/feel-wasm), so a hint can never claim a
// parameter the real function does not take (ADR-0016).
//
// It shares the same sources the completion dropdown and highlighter use, and is
// wired through attachFeelField so every FEEL field gets it at once.

import { builtins, ensureFeel, type FeelBuiltin } from './feel'

// One coloured piece of the hint. kind drives the styling (keyword/function name
// vs. a placeholder to fill in vs. punctuation); active marks the placeholder the
// caret is currently on, so it stands out from the rest of the (muted) template.
export type HintSegment = { text: string; kind: 'kw' | 'fn' | 'ph' | 'punct'; active?: boolean }
export type SignatureHint = { segments: HintSegment[] }

// The control-flow constructs worth a template, each with its literal keywords,
// the placeholders between them, and the sub-keywords that tell us which
// placeholder the caret has reached. Placeholders are German to match the UI.
type Construct = {
  // The template rendered left to right: keyword or placeholder pieces in order.
  parts: { text: string; ph?: boolean }[]
  // As each of these words appears after the opening keyword, the active
  // placeholder advances by one (index into the placeholder parts).
  advance: string[]
}
const CONSTRUCTS: Record<string, Construct> = {
  if: {
    parts: [{ text: 'if' }, { text: 'Bedingung', ph: true }, { text: 'then' }, { text: 'Wert', ph: true }, { text: 'else' }, { text: 'anderer Wert', ph: true }],
    advance: ['then', 'else'],
  },
  for: {
    parts: [{ text: 'for' }, { text: 'Element', ph: true }, { text: 'in' }, { text: 'Liste', ph: true }, { text: 'return' }, { text: 'Ausdruck', ph: true }],
    advance: ['in', 'return'],
  },
  some: {
    parts: [{ text: 'some' }, { text: 'Element', ph: true }, { text: 'in' }, { text: 'Liste', ph: true }, { text: 'satisfies' }, { text: 'Bedingung', ph: true }],
    advance: ['in', 'satisfies'],
  },
  every: {
    parts: [{ text: 'every' }, { text: 'Element', ph: true }, { text: 'in' }, { text: 'Liste', ph: true }, { text: 'satisfies' }, { text: 'Bedingung', ph: true }],
    advance: ['in', 'satisfies'],
  },
}

// The innermost function call the caret is inside: the source index just before
// its opening "(" (where the function name ends) and which argument the caret is
// on (0-based, counted by the top-level commas of that call). Strings are skipped
// so a "(" or "," inside a literal never counts. Returns null when the caret is
// not inside any open call.
function enclosingCall(value: string, caret: number): { nameEnd: number; argIndex: number } | null {
  const stack: { nameEnd: number; args: number }[] = []
  let inStr = false
  for (let i = 0; i < caret; i++) {
    const c = value[i]
    if (inStr) {
      if (c === '\\') i++
      else if (c === '"') inStr = false
      continue
    }
    if (c === '"') inStr = true
    else if (c === '(') stack.push({ nameEnd: i, args: 0 })
    else if (c === ')') stack.pop()
    else if (c === ',' && stack.length) stack[stack.length - 1].args++
  }
  const top = stack[stack.length - 1]
  return top ? { nameEnd: top.nameEnd, argIndex: top.args } : null
}

// The built-in whose call the caret is inside, matched from the name written just
// before the "(". A multi-word built-in ("substring after") matches whole; a
// name preceded by other words (e.g. `x and count`) falls back to its last word
// so `count(` still resolves. Returns null for a user function (BKM) or a plain
// parenthesised group — we only carry signatures for the engine's built-ins.
function builtinBefore(value: string, parenIdx: number, fns: FeelBuiltin[]): FeelBuiltin | null {
  const m = value.slice(0, parenIdx).match(/([A-Za-z_][A-Za-z0-9_]*(?:\s+[A-Za-z_][A-Za-z0-9_]*)*)\s*$/)
  if (!m) return null
  const cand = m[1]
  const exact = fns.find((f) => f.name === cand)
  if (exact) return exact
  const last = cand.split(/\s+/).pop() ?? ''
  return fns.find((f) => f.name === last) ?? null
}

// Build the hint for a function call: `name(p1, p2 …)` with the argument the
// caret is on marked active. A variadic function shows a trailing "…" that stays
// active once the caret runs past the last named parameter.
function functionHint(fn: FeelBuiltin, argIndex: number): SignatureHint {
  const segments: HintSegment[] = [{ text: fn.name, kind: 'fn' }, { text: '(', kind: 'punct' }]
  const overflow = argIndex >= fn.params.length
  fn.params.forEach((p, i) => {
    if (i > 0) segments.push({ text: ', ', kind: 'punct' })
    // Past the last named param a variadic keeps the last one lit; otherwise no
    // param is active (the caller typed more args than the signature names).
    const active = i === argIndex || (overflow && fn.variadic && i === fn.params.length - 1)
    segments.push({ text: p, kind: 'ph', active })
  })
  if (fn.variadic) {
    if (fn.params.length) segments.push({ text: ', ', kind: 'punct' })
    segments.push({ text: '…', kind: 'ph', active: overflow })
  }
  segments.push({ text: ')', kind: 'punct' })
  return { segments }
}

// Every identifier word before the caret, with its start index and whether it
// falls inside a string literal (string words are not keywords). Used to find the
// nearest enclosing construct keyword and how far into it the caret has moved.
function wordsBefore(value: string, caret: number): { w: string; i: number; str: boolean }[] {
  const out: { w: string; i: number; str: boolean }[] = []
  let inStr = false
  let i = 0
  while (i < caret) {
    const c = value[i]
    if (inStr) {
      if (c === '\\') i += 2
      else {
        if (c === '"') inStr = false
        i++
      }
      continue
    }
    if (c === '"') {
      inStr = true
      i++
      continue
    }
    if (/[A-Za-z_]/.test(c)) {
      let j = i + 1
      while (j < caret && /[A-Za-z0-9_]/.test(value[j])) j++
      out.push({ w: value.slice(i, j), i, str: false })
      i = j
      continue
    }
    i++
  }
  return out
}

// Build the hint for the nearest control-flow construct the caret is within
// (`if`/`for`/`some`/`every`), advancing the active placeholder as the construct's
// sub-keywords (then/else, in/return, in/satisfies) appear before the caret.
function constructHint(value: string, caret: number): SignatureHint | null {
  const words = wordsBefore(value, caret)
  // Nearest preceding construct keyword (scan from the caret backwards).
  let start = -1
  let key = ''
  for (let k = words.length - 1; k >= 0; k--) {
    if (!words[k].str && CONSTRUCTS[words[k].w]) {
      start = k
      key = words[k].w
      break
    }
  }
  if (start < 0) return null
  const con = CONSTRUCTS[key]
  // How many of the construct's sub-keywords have been typed since it opened →
  // which placeholder (0-based) the caret is on.
  let step = 0
  for (let k = start + 1; k < words.length; k++) {
    if (!words[k].str && con.advance[step] === words[k].w) step++
  }
  const segments: HintSegment[] = []
  let phSeen = -1
  con.parts.forEach((part, i) => {
    if (i > 0) segments.push({ text: ' ', kind: 'punct' })
    if (part.ph) {
      phSeen++
      segments.push({ text: part.text, kind: 'ph', active: phSeen === step })
    } else {
      segments.push({ text: part.text, kind: 'kw' })
    }
  })
  return { segments }
}

// signatureAt is the pure analysis: given the field text and caret, return the
// hint to show, or null when the caret is on a plain expression. A function call
// wins over a construct, because being inside "()" is the more specific context.
export function signatureAt(value: string, caret: number, fns: FeelBuiltin[]): SignatureHint | null {
  const call = enclosingCall(value, caret)
  if (call) {
    const fn = builtinBefore(value, call.nameEnd, fns)
    if (fn) return functionHint(fn, call.argIndex)
  }
  return constructHint(value, caret)
}

// Serialise a hint to a compact one-line string ("if Bedingung then Wert …"),
// used by the highlighter-independent tests and for the element's title.
function hintText(hint: SignatureHint): string {
  return hint.segments
    .map((s) => s.text)
    .join('')
    .replace(/\s+/g, ' ')
    .trim()
}

// attachSignatureHint floats the translucent hint above a FEEL field while it is
// focused and the caret is inside a call or construct. It is read-only chrome:
// no field value or event is touched. names is unused today but kept in the
// signature so callers wire it the same way as the highlighter/completion, and a
// future revision can resolve user-function signatures from scope.
export function attachSignatureHint(field: HTMLInputElement | HTMLTextAreaElement): void {
  void ensureFeel()
  let box: HTMLDivElement | null = null

  const hide = (): void => {
    box?.remove()
    box = null
  }

  const render = (hint: SignatureHint): void => {
    if (!box) {
      box = document.createElement('div')
      box.className = 'sig-hint'
      // Never steal focus from the field when clicked.
      box.addEventListener('mousedown', (e) => e.preventDefault())
      document.body.append(box)
    }
    box.textContent = ''
    box.title = hintText(hint)
    for (const s of hint.segments) {
      const span = document.createElement('span')
      span.className = 'sig-seg sig-' + s.kind + (s.active ? ' sig-active' : '')
      span.textContent = s.text
      box.append(span)
    }
    place()
  }

  // Anchor the hint to the field's top-left, sitting just above it. Flip below
  // when there is no room above (a field near the top of the viewport).
  const place = (): void => {
    if (!box) return
    const r = field.getBoundingClientRect()
    box.style.left = Math.max(6, r.left) + 'px'
    box.style.maxWidth = Math.max(160, window.innerWidth - r.left - 12) + 'px'
    const h = box.offsetHeight
    const above = r.top - h - 4
    box.style.top = (above >= 4 ? above : r.bottom + 4) + 'px'
  }

  const update = (): void => {
    if (document.activeElement !== field) {
      hide()
      return
    }
    const caret = field.selectionStart ?? field.value.length
    const hint = signatureAt(field.value, caret, builtins())
    if (hint) render(hint)
    else hide()
  }

  field.addEventListener('input', update)
  field.addEventListener('keyup', update)
  field.addEventListener('click', update)
  field.addEventListener('focus', update)
  field.addEventListener('blur', hide)
  // The hint is position-anchored to the field, so a page scroll or resize while
  // it is open must move it too.
  window.addEventListener('scroll', () => box && place(), true)
  window.addEventListener('resize', () => box && place())
}
