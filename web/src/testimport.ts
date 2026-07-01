import { evaluateGraph, InputValidationError, type ModelDetail, type InputField, type GraphEvalResult } from './api'
import { leafInputs, coerce } from './evaluate'

// The Import cockpit: a batch test-runner that reads like a conveyor belt. A
// user (or an AI agent) downloads a CSV/JSON template shaped to the model's leaf
// inputs, fills it with test cases, imports it, and hits "Durchlaufen lassen" —
// then watches each record zip from the left "Eingang" lane, through the
// "Evaluation" lane (the live engine), into the right "clio Store" lane, carrying
// its computed results. It reuses the exact same leaf-input set and coercion the
// evaluate form uses (evaluate.ts), so a template always matches the model, and
// the same whole-graph evaluate endpoint (evaluateGraph) the Operate cockpit runs.

// A single test case: the inputs to feed the graph, optional expected decision
// outputs (turning a run into a pass/fail assertion), and its live run state.
type Lane = 'in' | 'eval' | 'store'
type Status = 'staged' | 'running' | 'done' | 'error'
type TestCase = {
  id: number
  name: string
  inputs: Record<string, unknown>
  expected: Record<string, unknown>
  lane: Lane
  status: Status
  result?: GraphEvalResult
  error?: string
  // pass is undefined when the case declares no expectations (a pure run), else
  // whether every expected decision value matched.
  pass?: boolean
}

export type ImportOptions = {
  // Where the cockpit chrome is rendered (shown only in Import mode).
  host: HTMLElement
  // The model currently loaded in the editor, or null before one is picked.
  getModel: () => ModelDetail | null
}

export type ImportView = {
  // render redraws the cockpit from the current model + case list.
  render: () => void
  // reset clears the imported cases (called when the model changes).
  reset: () => void
}

// mountImport wires the cockpit once; render() refreshes it from live state.
export function mountImport(opts: ImportOptions): ImportView {
  const { host, getModel } = opts
  let cases: TestCase[] = []
  let nextId = 1
  let running = false
  let note = '' // a transient status line (import errors, run progress)

  host.textContent = ''
  const bar = el('div', 'imp-bar')
  const flow = el('div', 'imp-flow')
  const empty = el('div', 'imp-empty')
  host.append(bar, empty, flow)

  const fileInput = el('input', 'imp-file') as HTMLInputElement
  fileInput.type = 'file'
  fileInput.accept = '.csv,.json,text/csv,application/json'
  fileInput.hidden = true
  host.append(fileInput)
  fileInput.addEventListener('change', () => {
    const file = fileInput.files?.[0]
    if (file) void importFile(file)
    fileInput.value = '' // allow re-importing the same file
  })

  // importFile parses a dropped/selected CSV or JSON file into staged cases.
  const importFile = async (file: File): Promise<void> => {
    const model = getModel()
    if (!model) return
    try {
      const text = await file.text()
      const isJson = file.name.toLowerCase().endsWith('.json') || text.trimStart().startsWith('{') || text.trimStart().startsWith('[')
      const parsed = isJson ? parseJSONCases(text, model) : parseCSVCases(text, model)
      if (!parsed.length) {
        note = 'Keine Testfälle in der Datei gefunden.'
        render()
        return
      }
      for (const c of parsed) cases.push({ ...c, id: nextId++ })
      note = parsed.length === 1 ? '1 Testfall importiert.' : `${parsed.length} Testfälle importiert.`
      render()
    } catch (e) {
      note = 'Import fehlgeschlagen: ' + (e as Error).message
      render()
    }
  }

  // Drag & drop a CSV/JSON straight onto the cockpit.
  host.addEventListener('dragover', (e) => {
    e.preventDefault()
    host.classList.add('imp-drop')
  })
  host.addEventListener('dragleave', (e) => {
    if (e.target === host) host.classList.remove('imp-drop')
  })
  host.addEventListener('drop', (e) => {
    e.preventDefault()
    host.classList.remove('imp-drop')
    const file = e.dataTransfer?.files?.[0]
    if (file) void importFile(file)
  })

  // runAll drives the conveyor: each staged case flies into the Evaluation lane,
  // gets evaluated against the live engine, then lands in the clio Store lane
  // with its results. Sequential, with a beat between records so the flow reads.
  const runAll = async (): Promise<void> => {
    const model = getModel()
    if (!model || running) return
    const queue = cases.filter((c) => c.lane === 'in')
    if (!queue.length) return
    running = true
    render()
    for (const c of queue) {
      c.lane = 'eval'
      c.status = 'running'
      render()
      await sleep(260) // let the fly-into-evaluation animation read
      try {
        const res = await evaluateGraph(model.modelId, c.inputs, true, true)
        c.result = res
        c.status = 'done'
        c.pass = computePass(c, res)
      } catch (e) {
        c.status = 'error'
        c.error = e instanceof InputValidationError ? e.problems.map((p) => p.input + ': ' + p.message).join(' · ') : (e as Error).message
      }
      c.lane = 'store'
      render()
      await sleep(180)
    }
    running = false
    note = summarize(cases)
    render()
  }

  // clearAll empties the belt (keeps the model selected).
  const clearAll = (): void => {
    cases = []
    note = ''
    render()
  }

  // addSamples seeds a couple of example cases from the model's inferred input
  // values, so a user can try the whole flow without authoring a file first.
  const addSamples = (): void => {
    const model = getModel()
    if (!model) return
    const rows = sampleRows(leafInputs(model), 3)
    rows.forEach((inputs, i) => cases.push({ id: nextId++, name: 'Beispiel ' + (i + 1), inputs, expected: {}, lane: 'in', status: 'staged' }))
    note = rows.length ? `${rows.length} Beispiel-Testfälle eingefügt.` : 'Dieses Modell braucht keine Eingaben.'
    render()
  }

  const renderBar = (): void => {
    bar.textContent = ''
    const model = getModel()
    const csvBtn = button('Vorlage · CSV', () => model && download(templateCSV(model), safeName(model) + '-testfaelle.csv', 'text/csv'))
    const jsonBtn = button('Vorlage · JSON', () => model && download(templateJSON(model), safeName(model) + '-testfaelle.json', 'application/json'))
    const impBtn = button('Testfälle importieren …', () => fileInput.click(), 'imp-primary')
    const sampleBtn = button('Beispiele einfügen', addSamples)
    const staged = cases.filter((c) => c.lane === 'in').length
    const runBtn = button(running ? 'läuft …' : 'Durchlaufen lassen ▶', () => void runAll(), 'imp-run')
    runBtn.disabled = running || staged === 0
    const clearBtn = button('Leeren', clearAll)
    clearBtn.disabled = running || cases.length === 0

    const left = el('div', 'imp-bar-group', csvBtn, jsonBtn, impBtn, sampleBtn)
    const right = el('div', 'imp-bar-group', clearBtn, runBtn)
    bar.append(left, right)
    if (note) bar.append(el('div', 'imp-note', note))
  }

  const renderFlow = (): void => {
    flow.textContent = ''
    const model = getModel()
    empty.textContent = ''
    if (!model) {
      empty.append(el('p', 'imp-empty-msg', 'Kein Modell geladen.'))
      flow.hidden = true
      return
    }
    if (!cases.length) {
      empty.append(
        el('p', 'imp-empty-msg', 'Noch keine Testfälle. Lade eine Vorlage herunter (CSV/JSON), fülle sie mit Testdaten — von Hand, in der Tabellenkalkulation oder von einem KI-Agenten — und importiere sie. Dann „Durchlaufen lassen“ und den Datensätzen beim Flitzen zusehen.'),
      )
      flow.hidden = true
      return
    }
    flow.hidden = false

    flow.append(
      lane('in', 'Eingang', cases.filter((c) => c.lane === 'in')),
      evalLane(model, cases.find((c) => c.lane === 'eval') ?? null),
      lane('store', 'clio Store', cases.filter((c) => c.lane === 'store')),
    )
  }

  // lane draws the left (Eingang) or right (clio Store) column with its cards.
  const lane = (kind: Lane, title: string, items: TestCase[]): HTMLElement => {
    const col = el('div', 'imp-lane imp-lane-' + kind)
    col.append(el('div', 'imp-lane-title', title, el('span', 'imp-lane-count', String(items.length))))
    const shelf = el('div', 'imp-shelf')
    if (!items.length) shelf.append(el('div', 'imp-lane-none', kind === 'in' ? '(leer)' : '(noch nichts gelaufen)'))
    for (const c of items) shelf.append(card(c))
    col.append(shelf)
    return col
  }

  // evalLane is the middle column: the live engine, with the case currently under
  // evaluation sitting on it (or a resting state between records).
  const evalLane = (model: ModelDetail, active: TestCase | null): HTMLElement => {
    const col = el('div', 'imp-lane imp-lane-eval' + (active ? ' is-busy' : ''))
    col.append(el('div', 'imp-lane-title', 'Evaluation'))
    const engine = el('div', 'imp-engine')
    engine.append(el('div', 'imp-engine-name', model.name || 'Modell'))
    const chips = el('div', 'imp-engine-chips')
    for (const d of model.decisions ?? []) chips.append(el('span', 'imp-chip', d))
    engine.append(chips)
    engine.append(el('div', 'imp-engine-pulse'))
    col.append(engine)
    if (active) col.append(card(active))
    else col.append(el('div', 'imp-lane-none', 'bereit'))
    return col
  }

  // card renders one test case; in the store lane it also shows its results and,
  // when the case declared expectations, a pass/fail badge.
  const card = (c: TestCase): HTMLElement => {
    const node = el('div', 'imp-card imp-card-' + c.status + (c.lane !== 'in' ? ' imp-fly' : ''))
    const head = el('div', 'imp-card-head', el('span', 'imp-card-name', c.name || 'Fall ' + c.id))
    if (c.lane === 'store' && c.pass !== undefined) head.append(el('span', 'imp-badge ' + (c.pass ? 'imp-pass' : 'imp-fail'), c.pass ? '✓ OK' : '✗ Abweichung'))
    else if (c.status === 'error') head.append(el('span', 'imp-badge imp-fail', '✗ Fehler'))
    node.append(head)

    node.append(el('div', 'imp-kv', summarizeKV(c.inputs)))

    if (c.status === 'error') {
      node.append(el('div', 'imp-card-err', c.error ?? 'Fehler'))
    } else if (c.lane === 'store' && c.result) {
      const out = el('div', 'imp-out')
      for (const d of Object.keys(c.result.values)) {
        const got = c.result.values[d]
        const exp = c.expected[d]
        const bad = exp !== undefined && !looseEqual(got, exp)
        out.append(el('div', 'imp-out-row' + (bad ? ' imp-out-bad' : ''), el('span', 'imp-out-k', d), el('span', 'imp-out-v', fmt(got))))
      }
      node.append(out)
    }
    return node
  }

  const render = (): void => {
    renderBar()
    renderFlow()
  }

  render()
  return {
    render,
    reset: () => {
      cases = []
      running = false
      note = ''
      render()
    },
  }
}

// ---- template generation -------------------------------------------------

// templateCSV builds a spreadsheet-fillable template: a `case` label column, one
// column per leaf input, and one `→Decision` expected column per decision (leave
// blank for a pure run, fill to assert). Two example rows are pre-filled from the
// inputs' inferred values so the shape is obvious. A leading comment row documents
// the format for humans and AI agents alike.
export function templateCSV(model: ModelDetail): string {
  const fields = leafInputs(model)
  const decisions = model.decisions ?? []
  const header = ['case', ...fields.map((f) => f.name), ...decisions.map((d) => '→' + d)]
  const samples = sampleRows(fields, 2)
  const rows = samples.map((inputs, i) => ['Fall ' + (i + 1), ...fields.map((f) => csvCell(inputs[f.name])), ...decisions.map(() => '')])
  const comment = `# Testfall-Vorlage für „${model.name || 'Modell'}". Eine Zeile = ein Testfall. Spalten „→Decision" sind erwartete Ergebnisse (optional, für Pass/Fail). Werte als FEEL/JSON: 1200, "Business", true.`
  return [comment, header.map(csvField).join(','), ...rows.map((r) => r.map(csvField).join(','))].join('\n') + '\n'
}

// templateJSON builds the same template as a JSON document: a model name and a
// `cases` array of { name, input, expect }. AI-agent-friendly and round-trips
// through importFile. One example case is pre-filled.
export function templateJSON(model: ModelDetail): string {
  const fields = leafInputs(model)
  const [sample] = sampleRows(fields, 1)
  const expect: Record<string, unknown> = {}
  for (const d of model.decisions ?? []) expect[d] = null
  const doc = {
    _hinweis: 'Testfälle für die Import-Cockpit. "input" füllt die Modell-Eingaben; "expect" (optional) sind erwartete Decision-Ergebnisse für Pass/Fail.',
    model: model.name || model.modelId,
    cases: [{ name: 'Fall 1', input: sample ?? {}, expect }],
  }
  return JSON.stringify(doc, null, 2) + '\n'
}

// sampleRows builds up to n example input rows from each field's inferred values
// (a declared enumeration or the literals seen in table cells), cycling through
// them so successive rows differ. Fields without suggestions get a typed
// placeholder. Returns [] when the model has no inputs.
function sampleRows(fields: InputField[], n: number): Record<string, unknown>[] {
  if (!fields.length) return []
  const rows: Record<string, unknown>[] = []
  for (let i = 0; i < n; i++) {
    const row: Record<string, unknown> = {}
    for (const f of fields) {
      const vals = f.values ?? []
      if (vals.length) row[f.name] = coerce(vals[i % vals.length])
      else row[f.name] = placeholder(f.type)
    }
    rows.push(row)
  }
  return rows
}

// placeholder is a neutral example value for a field with no inferred values.
function placeholder(type?: string): unknown {
  switch ((type ?? '').toLowerCase()) {
    case 'number':
      return 0
    case 'boolean':
      return false
    case 'date':
      return '2024-01-01'
    default:
      return ''
  }
}

// ---- import parsing ------------------------------------------------------

// parseCSVCases reads the template CSV back into cases. The `case` column is the
// label; columns matching a leaf-input name are inputs; `→Decision`/`->Decision`
// columns are expectations. Empty cells are omitted (contribute nothing). Comment
// lines (starting with #) and blank lines are skipped.
function parseCSVCases(text: string, model: ModelDetail): Omit<TestCase, 'id'>[] {
  const inputNames = new Set(leafInputs(model).map((f) => f.name))
  const table = parseCSV(text).filter((row) => row.length && !(row.length === 1 && row[0].trim() === '') && !row[0].trimStart().startsWith('#'))
  if (table.length < 2) return []
  const header = table[0]
  const kinds = header.map((h) => classifyColumn(h.trim(), inputNames))
  const out: Omit<TestCase, 'id'>[] = []
  for (const row of table.slice(1)) {
    const inputs: Record<string, unknown> = {}
    const expected: Record<string, unknown> = {}
    let name = ''
    row.forEach((raw, i) => {
      const k = kinds[i]
      if (!k) return
      if (k.kind === 'label') name = raw.trim()
      else if (k.kind === 'input') {
        const v = coerce(raw)
        if (v !== undefined) inputs[k.name] = v
      } else {
        const v = coerce(raw)
        if (v !== undefined) expected[k.name] = v
      }
    })
    if (Object.keys(inputs).length || name) out.push({ name, inputs, expected, lane: 'in', status: 'staged' })
  }
  return out
}

type Column = { kind: 'label' | 'input' | 'expect'; name: string } | null
function classifyColumn(h: string, inputNames: Set<string>): Column {
  const low = h.toLowerCase()
  if (low === 'case' || low === 'name' || low === 'fall' || low === '#' || low === '') return { kind: 'label', name: '' }
  if (h.startsWith('→') || h.startsWith('->') || h.startsWith('=')) return { kind: 'expect', name: h.replace(/^(→|->|=)\s*/, '') }
  if (inputNames.has(h)) return { kind: 'input', name: h }
  // Unknown header — treat as an input by name (tolerant: the engine validates it).
  return { kind: 'input', name: h }
}

// parseJSONCases reads either the template shape ({ cases: [...] }), a bare array
// of { name, input, expect } objects, or a bare array of input objects.
function parseJSONCases(text: string, _model: ModelDetail): Omit<TestCase, 'id'>[] {
  const doc = JSON.parse(text) as unknown
  const arr = Array.isArray(doc) ? doc : Array.isArray((doc as { cases?: unknown }).cases) ? (doc as { cases: unknown[] }).cases : []
  const out: Omit<TestCase, 'id'>[] = []
  ;(arr as unknown[]).forEach((raw, i) => {
    const o = (raw ?? {}) as Record<string, unknown>
    const input = isObj(o.input) ? o.input : isObj(o.inputs) ? o.inputs : hasInputShape(o) ? stripMeta(o) : {}
    const expect = isObj(o.expect) ? o.expect : isObj(o.expected) ? o.expected : {}
    const name = typeof o.name === 'string' ? o.name : 'Fall ' + (i + 1)
    if (Object.keys(input).length || o.name) out.push({ name, inputs: input, expected: expect, lane: 'in', status: 'staged' })
  })
  return out
}

function isObj(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v)
}
// hasInputShape: a bare object with no input/expect wrapper is itself the inputs.
function hasInputShape(o: Record<string, unknown>): boolean {
  return !('input' in o) && !('inputs' in o) && !('expect' in o) && !('expected' in o)
}
function stripMeta(o: Record<string, unknown>): Record<string, unknown> {
  const { name: _n, ...rest } = o
  return rest
}

// ---- CSV primitives ------------------------------------------------------

// parseCSV is a minimal RFC-4180-ish reader: comma-separated, double-quote
// quoting with "" escapes, CRLF/LF rows. Enough for the template round-trip.
function parseCSV(text: string): string[][] {
  const rows: string[][] = []
  let row: string[] = []
  let field = ''
  let quoted = false
  for (let i = 0; i < text.length; i++) {
    const ch = text[i]
    if (quoted) {
      if (ch === '"') {
        if (text[i + 1] === '"') {
          field += '"'
          i++
        } else quoted = false
      } else field += ch
    } else if (ch === '"') {
      quoted = true
    } else if (ch === ',') {
      row.push(field)
      field = ''
    } else if (ch === '\n' || ch === '\r') {
      if (ch === '\r' && text[i + 1] === '\n') i++
      row.push(field)
      rows.push(row)
      row = []
      field = ''
    } else field += ch
  }
  if (field !== '' || row.length) {
    row.push(field)
    rows.push(row)
  }
  return rows
}

// csvField quotes a field when it contains a comma, quote or newline.
function csvField(s: string): string {
  return /[",\n\r]/.test(s) ? '"' + s.replace(/"/g, '""') + '"' : s
}
// csvCell renders a JSON value for a template cell (strings bare, others as JSON).
function csvCell(v: unknown): string {
  if (v === undefined || v === null) return ''
  if (typeof v === 'string') return v
  return JSON.stringify(v)
}

// ---- pass/fail + formatting ---------------------------------------------

// computePass returns whether every expected decision value matched the computed
// one, or undefined when the case declared no expectations (a pure run).
function computePass(c: TestCase, res: GraphEvalResult): boolean | undefined {
  const keys = Object.keys(c.expected)
  if (!keys.length) return undefined
  return keys.every((k) => looseEqual(res.values[k], c.expected[k]))
}

// looseEqual compares an actual value to an expected one tolerantly: numbers
// (which the engine may return as exact decimal strings) compare numerically,
// everything else by canonical JSON.
function looseEqual(got: unknown, exp: unknown): boolean {
  const gn = asNumber(got)
  const en = asNumber(exp)
  if (gn !== null && en !== null) return gn === en
  return JSON.stringify(got) === JSON.stringify(exp)
}
function asNumber(v: unknown): number | null {
  if (typeof v === 'number' && Number.isFinite(v)) return v
  if (typeof v === 'string' && v.trim() !== '' && Number.isFinite(Number(v))) return Number(v)
  return null
}

function summarizeKV(inputs: Record<string, unknown>): string {
  const parts = Object.entries(inputs).map(([k, v]) => k + '=' + fmt(v))
  return parts.length ? parts.join(' · ') : '(keine Eingaben)'
}
function summarize(cases: TestCase[]): string {
  const done = cases.filter((c) => c.lane === 'store')
  const errs = done.filter((c) => c.status === 'error').length
  const asserted = done.filter((c) => c.pass !== undefined)
  const passed = asserted.filter((c) => c.pass).length
  const parts = [`${done.length} gelaufen`]
  if (asserted.length) parts.push(`${passed}/${asserted.length} bestanden`)
  if (errs) parts.push(`${errs} Fehler`)
  return parts.join(' · ')
}

function safeName(model: ModelDetail): string {
  return (model.name || 'modell').replace(/[^a-zA-Z0-9-_]+/g, '-').replace(/^-+|-+$/g, '').toLowerCase() || 'modell'
}

// download triggers a client-side file download of the given text.
function download(text: string, filename: string, mime: string): void {
  const blob = new Blob([text], { type: mime + ';charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.append(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 0)
}

function fmt(v: unknown): string {
  if (v === null || v === undefined) return 'null'
  if (typeof v === 'string') return v
  return JSON.stringify(v)
}

function sleep(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms))
}

// button builds a toolbar button with a click handler and optional extra class.
function button(label: string, onClick: () => void, cls = ''): HTMLButtonElement {
  const b = el('button', 'imp-btn' + (cls ? ' ' + cls : '')) as HTMLButtonElement
  b.type = 'button'
  b.textContent = label
  b.addEventListener('click', onClick)
  return b
}

// el is a tiny DOM builder: tag, class, then string/Node children.
function el(tag: string, cls: string, ...kids: (string | Node)[]): HTMLElement {
  const n = document.createElement(tag)
  if (cls) n.className = cls
  n.append(...kids)
  return n
}
