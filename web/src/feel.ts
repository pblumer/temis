// Lazy bridge to temis' FEEL engine compiled to WebAssembly (cmd/feel-wasm).
// The ~1 MB (gzipped) module is fetched on first use (e.g. the first rename), so
// the initial app stays light. Once loaded, validation is synchronous and runs
// fully in the browser — the same parser the engine uses, offline (ADR-0016).

type CellResult = { ok: boolean; line?: number; col?: number; message?: string }

// A FEEL built-in function as the engine reports it: its name plus the ordered
// parameter names (and whether it is variadic) for signature hints.
export type FeelBuiltin = { name: string; params: string[]; variadic: boolean }

declare global {
  interface Window {
    Go: new () => { run: (instance: WebAssembly.Instance) => void; importObject: WebAssembly.Imports }
    temisFeelValidateName?: (name: string) => { ok: boolean; message?: string }
    temisFeelValidate?: (expr: string, namesCsv: string) => CellResult
    temisFeelValidateUnary?: (test: string, namesCsv: string) => CellResult
    temisFeelBuiltins?: () => FeelBuiltin[]
  }
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

export type CellCheck = { ok: boolean; message?: string }

// validateExpr checks a decision-table output cell (a full FEEL expression) that
// may reference the given input names. ok=true optimistically until the module
// loads, so editing is never blocked while it downloads.
export function validateExpr(expr: string, names: string[]): CellCheck {
  const fn = window.temisFeelValidate
  if (!fn) return { ok: true }
  return fn(expr, names.join(','))
}

// validateUnary checks a decision-table input cell (a FEEL unary test, e.g.
// `> 10`, `"Winter"`, `[1..5]`). The engine binds the column value to "?"; the
// cell may also reference the given input names.
export function validateUnary(test: string, names: string[]): CellCheck {
  const fn = window.temisFeelValidateUnary
  if (!fn) return { ok: true }
  return fn(test, names.join(','))
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
