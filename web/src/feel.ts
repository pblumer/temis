// Lazy bridge to temis' FEEL engine compiled to WebAssembly (cmd/feel-wasm).
// The ~1 MB (gzipped) module is fetched on first use (e.g. the first rename), so
// the initial app stays light. Once loaded, validation is synchronous and runs
// fully in the browser — the same parser the engine uses, offline (ADR-0016).

type CellResult = { ok: boolean; line?: number; col?: number; message?: string }

// A FEEL built-in function as the engine reports it: its name plus the ordered
// parameter names (and whether it is variadic) for signature hints.
export type FeelBuiltin = { name: string; params: string[]; variadic: boolean }

// A model's user-defined function (a BKM): its name and ordered parameter names.
// These are handed to the engine so calls to them validate as known functions,
// and offered by the completion dropdown alongside the built-ins.
export type ModelFunction = { name: string; params: string[] }

declare global {
  interface Window {
    Go: new () => { run: (instance: WebAssembly.Instance) => void; importObject: WebAssembly.Imports }
    temisFeelValidateName?: (name: string) => { ok: boolean; message?: string }
    temisFeelValidate?: (expr: string, namesCsv: string, funcNamesCsv?: string) => CellResult
    temisFeelValidateUnary?: (test: string, namesCsv: string, funcNamesCsv?: string) => CellResult
    temisFeelBuiltins?: () => FeelBuiltin[]
  }
}

// The user-defined functions of the model currently open in the editor. Set once
// per model load (setModelFunctions) and read by every FEEL field's validation
// and completion, so a call to a BKM — a BKM's own recursion included — is a
// known function everywhere, not just in the editor that happens to define it.
let modelFuncs: ModelFunction[] = []

// setModelFunctions records the open model's user-defined functions (its BKMs).
// The modeler calls it after loading a model's detail, so every FEEL field then
// completes and validates calls to those functions. Passing an empty list (a
// model with no BKMs) clears any previous model's functions.
export function setModelFunctions(fns: ModelFunction[]): void {
  modelFuncs = fns.filter((f) => f.name.trim() !== '').map((f) => ({ name: f.name, params: f.params ?? [] }))
}

// modelFunctions returns the open model's user-defined functions, for the
// completion dropdown to offer them alongside the engine's built-ins.
export function modelFunctions(): ModelFunction[] {
  return modelFuncs
}

// upsertModelFunction adds (or refreshes, by name) a single function, so an
// editor can guarantee its own function is in scope while it is open — a BKM
// editor registering the BKM it edits, so a recursive call resolves even for a
// freshly created BKM the last model load did not yet know. A blank name is
// ignored. The next model load's setModelFunctions replaces the whole set.
export function upsertModelFunction(fn: ModelFunction): void {
  if (fn.name.trim() === '') return
  const next = { name: fn.name, params: fn.params ?? [] }
  const i = modelFuncs.findIndex((f) => f.name === next.name)
  if (i >= 0) modelFuncs[i] = next
  else modelFuncs = [...modelFuncs, next]
}

// funcNamesCsv lists the current model functions' names for the wasm validators,
// comma-separated like the input names. Only the name is needed to recognise a
// call (the parameter list drives completion hints, built in the browser). It
// returns '' when there are none, so the argument stays optional and the engine
// skips it entirely.
function funcNamesCsv(): string {
  return modelFuncs.map((f) => f.name).join(',')
}

let loading: Promise<void> | null = null

function loadScript(src: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const s = document.createElement('script')
    s.src = src
    s.onload = () => resolve()
    s.onerror = () => reject(new Error('konnte ' + src + ' nicht laden'))
    document.head.appendChild(s)
  })
}

// ensureFeel loads (once) the wasm_exec.js loader and the feel.wasm module. URLs
// are relative to /app/, so they resolve to the embedded assets.
export function ensureFeel(): Promise<void> {
  if (!loading) {
    loading = (async () => {
      await loadScript('wasm_exec.js')
      const go = new window.Go()
      const bytes = await (await fetch('feel.wasm')).arrayBuffer()
      const { instance } = await WebAssembly.instantiate(bytes, go.importObject)
      go.run(instance)
    })()
  }
  return loading
}

export type NameCheck = { ok: boolean; message?: string }

// validateName checks a variable name against the FEEL engine (spaces ok,
// operator characters not). Returns ok=true optimistically until the module has
// loaded, so typing is never blocked while it downloads.
export function validateName(name: string): NameCheck {
  const fn = window.temisFeelValidateName
  if (!fn) return { ok: true }
  return fn(name)
}

// feelSafeName derives a FEEL-usable identifier from a free-form display label:
// characters FEEL treats as operators/punctuation (e.g. "-", ".", "/") become
// spaces — FEEL allows names with spaces — runs collapse, and a leading digit gets
// an underscore (a FEEL name may not start with a digit). "U-002 Nr." → "U 002 Nr".
// Best-effort: the FEEL-name editor still validates, so the author can refine it.
export function feelSafeName(raw: string): string {
  let s = (raw || '').normalize('NFC').replace(/[^\p{L}\p{N}_ ]+/gu, ' ').replace(/\s+/g, ' ').trim()
  if (s && /^\p{N}/u.test(s)) s = '_' + s
  return s
}

// feelRefFor picks the FEEL identifier for a display name: the name itself when it
// is already a valid FEEL name, otherwise a FEEL-safe derivation of it. This is the
// "follow the display name" default; an explicitly authored variable name overrides
// it (see the modeler's varNameLocked).
export function feelRefFor(name: string): string {
  const nm = (name || '').trim()
  if (!nm) return ''
  return validateName(nm).ok ? nm : feelSafeName(nm)
}

export type CellCheck = { ok: boolean; message?: string }

// validateExpr checks a decision-table output cell (a full FEEL expression) that
// may reference the given input names. ok=true optimistically until the module
// loads, so editing is never blocked while it downloads.
export function validateExpr(expr: string, names: string[]): CellCheck {
  const fn = window.temisFeelValidate
  if (!fn) return { ok: true }
  return fn(expr, names.join(','), funcNamesCsv())
}

// validateUnary checks a decision-table input cell (a FEEL unary test, e.g.
// `> 10`, `"Winter"`, `[1..5]`). The engine binds the column value to "?"; the
// cell may also reference the given input names.
export function validateUnary(test: string, names: string[]): CellCheck {
  const fn = window.temisFeelValidateUnary
  if (!fn) return { ok: true }
  return fn(test, names.join(','), funcNamesCsv())
}

// builtins returns the engine's catalog of FEEL built-in functions, or an empty
// list until the module has loaded. The catalog is the engine's own registry, so
// completions can never drift from what actually evaluates.
export function builtins(): FeelBuiltin[] {
  const fn = window.temisFeelBuiltins
  if (!fn) return []
  try {
    return fn()
  } catch {
    return []
  }
}
