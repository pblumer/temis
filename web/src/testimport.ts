import { evaluateGraphBatch, ClioNotConfiguredError, type ModelDetail, type InputField, type GraphCaseResult, type BatchCase } from './api'
import { leafInputs, coerce } from './evaluate'

// The Import cockpit: a batch test-runner that reads like a conveyor belt. A
// user (or an AI agent) downloads a CSV/JSON template shaped to the model's leaf
// inputs, fills it with test cases, imports it, and hits "Durchlaufen lassen" —
// then watches the records fly from the left "Eingang" lane, through the
// "Evaluation" lane (the live engine), into the right "clio Store" lane, carrying
// their computed results. It reuses the exact same leaf-input set and coercion
// the evaluate form uses (evaluate.ts), so a template always matches the model.
//
// Throughput is the whole point: a run sends EVERY staged row in ONE batch
// request (evaluateGraphBatch) rather than one HTTP call per row, so thousands of
// cases evaluate in a single round-trip — the engine loops in-memory. The belt
// then fills in one render with a CSS-staggered cascade (no per-row JS timers, no
// full re-render per record), and both lanes cap how many cards they draw, so a
// 5000-case run stays instant instead of freezing the DOM.

// A single test case: the inputs to feed the graph, optional expected decision
// outputs (turning a run into a pass/fail assertion), and its live run state.
type Lane = 'in' | 'eval' | 'store'
type Status = 'staged' | 'running' | 'done' | 'error'
type TestCase = {
  id: number
  name: string
  // entity is the subject a productive run's clio quality event is filed on; a
  // template `entity` column fills it, else the subject-key field, else the label.
  entity: string
  inputs: Record<string, unknown>
  expected: Record<string, unknown>
  lane: Lane
  status: Status
  // values/evalErrors hold the batch outcome for a landed case; error holds a
  // whole-case failure message (rejected strict input or runtime failure).
  values?: Record<string, unknown>
  evalErrors?: Record<string, string>
  error?: string
  // pass is undefined when the case declares no expectations (a pure run), else
  // whether every expected decision value matched.
  pass?: boolean
  // recorded is true when this case was written to clio (a productive run).
  recorded?: boolean
}

// How many cards each lane draws at most. Thousands of DOM nodes would freeze the
// browser and nobody reads 5000 cards — the lane's count badge shows the true
// total, and an overflow footer names the rest.
const LANE_CAP = 120

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
  // animateOnce is true for exactly the one render right after a run, so the
  // freshly-landed store cards play their staggered fly-in without every later
  // (unrelated) render re-triggering the animation.
  let animateOnce = false
  // Run mode: a Testlauf (default) evaluates locally and writes NOTHING; a
  // Produktivlauf writes one clio quality event per evaluated case, on its
  // entity (queued, guaranteed). subjectKey names the input field used as the
  // entity when a case has no explicit one.
  let productive = false
  let subjectKey = ''

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

  // runAll drives the belt: it sends EVERY staged row in one batch request, then
  // lands them all in the clio Store in a single render with a staggered CSS
  // cascade. No per-row HTTP, no per-row timers, no full re-render per record —
  // that is what makes thousands of cases finish in one fast round-trip instead
  // of grinding for minutes. The engine loops in-memory server-side.
  const runAll = async (): Promise<void> => {
    const model = getModel()
    if (!model || running) return
    const queue = cases.filter((c) => c.lane === 'in')
    if (!queue.length) return
    running = true
    const verb = productive ? 'Produktivlauf' : 'Testlauf'
    note = `${verb}: ${queue.length.toLocaleString('de-CH')} Testfälle werden ausgewertet …`
    render()
    const t0 = performance.now()
    let recorded = 0
    try {
      const payload: BatchCase[] = queue.map((c) => ({ name: c.name, entity: c.entity || undefined, input: c.inputs, expect: Object.keys(c.expected).length ? c.expected : undefined }))
      const batch = await evaluateGraphBatch(model.modelId, { cases: payload, strict: true, record: productive, subjectKey: subjectKey || undefined })
      queue.forEach((c, i) => applyResult(c, batch.results[i]))
      recorded = batch.recorded
    } catch (e) {
      running = false
      // A productive run without clio configured: explain and keep the cases
      // staged so the user can switch to a Testlauf and re-run.
      note = e instanceof ClioNotConfiguredError ? 'Produktivlauf nicht möglich: clio ist nicht konfiguriert (TEMIS_CLIO_TOKEN setzen). Als Testlauf ausführen.' : 'Auswertung fehlgeschlagen: ' + (e as Error).message
      render()
      return
    }
    const ms = performance.now() - t0
    // All evaluated rows move to the clio Store at once; the cascade is pure CSS.
    for (const c of queue) c.lane = 'store'
    running = false
    animateOnce = true
    const tail = productive ? ` · ${recorded.toLocaleString('de-CH')} Quality Events an clio übergeben` : ''
    note = `${verb} · ${summarize(cases)} · Auswertung in ${fmtDuration(ms)}${tail} · Ergebnisse als CSV herunterladbar ↓`
    render()
    animateOnce = false
  }

  // applyResult folds one batch row's outcome into its case: values + pass/fail,
  // or a whole-case error (rejected strict input or a runtime failure).
  const applyResult = (c: TestCase, r: GraphCaseResult | undefined): void => {
    if (!r || r.problem) {
      c.status = 'error'
      const p = r?.problem
      c.error = p ? (p.problems?.length ? p.problems.map((x) => x.input + ': ' + x.message).join(' · ') : p.message) : 'kein Ergebnis'
    } else {
      c.status = 'done'
      c.values = r.values ?? {}
      c.evalErrors = r.errors
      c.pass = computePass(c.expected, c.values)
      c.recorded = productive
    }
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
    rows.forEach((inputs, i) => cases.push({ id: nextId++, name: 'Beispiel ' + (i + 1), entity: '', inputs, expected: {}, lane: 'in', status: 'staged' }))
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
    const hasResults = cases.some((c) => c.lane === 'store')
    // Offered after a run: the filled-in test sheet with the computed outputs.
    const resultsBtn = button('Ergebnisse · CSV ↓', () => model && download(resultsCSV(model, cases), safeName(model) + '-ergebnisse.csv', 'text/csv'), 'imp-results' + (hasResults ? ' imp-primary' : ''))
    resultsBtn.disabled = running || !hasResults
    resultsBtn.title = hasResults ? 'Eingaben + berechnete Outputs als CSV herunterladen' : 'Erst einen Lauf ausführen'
    const runBtn = button(running ? 'läuft …' : productive ? 'Produktivlauf ▶' : 'Testlauf ▶', () => void runAll(), 'imp-run' + (productive ? ' imp-prod' : ''))
    runBtn.disabled = running || staged === 0
    const clearBtn = button('Leeren', clearAll)
    clearBtn.disabled = running || cases.length === 0

    const left = el('div', 'imp-bar-group', csvBtn, jsonBtn, impBtn, sampleBtn)
    const right = el('div', 'imp-bar-group', modeToggle(model), resultsBtn, clearBtn, runBtn)
    bar.append(left, right)
    if (note) bar.append(el('div', 'imp-note', note))
  }

  // modeToggle is the Testlauf/Produktivlauf segmented control. In Produktivlauf
  // it reveals an optional "Entität aus Feld" picker (the subject-key fallback).
  const modeToggle = (model: ModelDetail | null): HTMLElement => {
    const wrap = el('div', 'imp-mode')
    const seg = el('div', 'imp-seg')
    const testBtn = el('button', 'imp-seg-btn' + (productive ? '' : ' is-on'), 'Testlauf') as HTMLButtonElement
    testBtn.type = 'button'
    testBtn.title = 'Nur lokal auswerten — es wird nichts nach clio geschrieben'
    const prodBtn = el('button', 'imp-seg-btn' + (productive ? ' is-on is-prod' : ''), 'Produktivlauf') as HTMLButtonElement
    prodBtn.type = 'button'
    prodBtn.title = 'Pro Fall ein clio-Quality-Event auf der Entität schreiben (queued, garantiert)'
    testBtn.disabled = running
    prodBtn.disabled = running
    testBtn.addEventListener('click', () => {
      productive = false
      render()
    })
    prodBtn.addEventListener('click', () => {
      productive = true
      render()
    })
    seg.append(testBtn, prodBtn)
    wrap.append(seg)
    if (productive && model) {
      const fields = leafInputs(model)
      const sel = el('select', 'imp-subject') as HTMLSelectElement
      sel.title = 'Entität aus einem Eingabefeld (Fallback, wenn keine entity-Spalte gesetzt ist)'
      const none = el('option', '', 'Entität: aus Spalte/Label') as HTMLOptionElement
      none.value = ''
      sel.append(none)
      for (const f of fields) {
        const o = el('option', '', 'Entität: ' + f.name) as HTMLOptionElement
        o.value = f.name
        sel.append(o)
      }
      sel.value = subjectKey
      sel.disabled = running
      sel.addEventListener('change', () => {
        subjectKey = sel.value
      })
      wrap.append(sel)
    }
    return wrap
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
      lane('in', 'Eingang', cases.filter((c) => c.lane === 'in'), false),
      evalLane(model, running),
      lane('store', 'clio Store', cases.filter((c) => c.lane === 'store'), animateOnce),
    )
  }

  // lane draws the left (Eingang) or right (clio Store) column with its cards,
  // capped at LANE_CAP so the DOM never blows up on a huge batch; the count badge
  // still shows the true total and an overflow footer names the rest. animate
  // plays the staggered fly-in on the freshly-landed store cards.
  const lane = (kind: Lane, title: string, items: TestCase[], animate: boolean): HTMLElement => {
    const col = el('div', 'imp-lane imp-lane-' + kind)
    col.append(el('div', 'imp-lane-title', title, el('span', 'imp-lane-count', items.length.toLocaleString('de-CH'))))
    const shelf = el('div', 'imp-shelf')
    if (!items.length) shelf.append(el('div', 'imp-lane-none', kind === 'in' ? '(leer)' : '(noch nichts gelaufen)'))
    const shown = items.slice(0, LANE_CAP)
    shown.forEach((c, i) => shelf.append(card(c, animate ? i : -1)))
    if (items.length > LANE_CAP) shelf.append(el('div', 'imp-lane-more', `+ ${(items.length - LANE_CAP).toLocaleString('de-CH')} weitere`))
    col.append(shelf)
    return col
  }

  // evalLane is the middle column: the live engine. It pulses while a batch is in
  // flight (busy) and otherwise rests, showing the model and its decisions.
  const evalLane = (model: ModelDetail, busy: boolean): HTMLElement => {
    const col = el('div', 'imp-lane imp-lane-eval' + (busy ? ' is-busy' : ''))
    col.append(el('div', 'imp-lane-title', 'Evaluation'))
    const engine = el('div', 'imp-engine')
    engine.append(el('div', 'imp-engine-name', model.name || 'Modell'))
    const chips = el('div', 'imp-engine-chips')
    for (const d of model.decisions ?? []) chips.append(el('span', 'imp-chip', d))
    engine.append(chips)
    engine.append(el('div', 'imp-engine-pulse'))
    col.append(engine)
    col.append(el('div', 'imp-lane-none', busy ? 'wertet aus …' : 'bereit'))
    return col
  }

  // card renders one test case; in the store lane it also shows its results and,
  // when the case declared expectations, a pass/fail badge. animIdx >= 0 gives the
  // card a staggered fly-in (its position in the freshly-landed cascade).
  const card = (c: TestCase, animIdx: number): HTMLElement => {
    const node = el('div', 'imp-card imp-card-' + c.status + (animIdx >= 0 ? ' imp-fly' : ''))
    if (animIdx >= 0) node.style.animationDelay = Math.min(animIdx, 40) * 12 + 'ms'
    const head = el('div', 'imp-card-head', el('span', 'imp-card-name', c.name || 'Fall ' + c.id))
    if (c.lane === 'store' && c.pass !== undefined) head.append(el('span', 'imp-badge ' + (c.pass ? 'imp-pass' : 'imp-fail'), c.pass ? '✓ OK' : '✗ Abweichung'))
    else if (c.status === 'error') head.append(el('span', 'imp-badge imp-fail', '✗ Fehler'))
    if (c.lane === 'store' && c.recorded) head.append(el('span', 'imp-rec', '→ clio'))
    node.append(head)

    node.append(el('div', 'imp-kv', summarizeKV(c.inputs)))

    if (c.status === 'error') {
      node.append(el('div', 'imp-card-err', c.error ?? 'Fehler'))
    } else if (c.lane === 'store' && c.values) {
      const out = el('div', 'imp-out')
      for (const d of Object.keys(c.values)) {
        const got = c.values[d]
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
      subjectKey = '' // the subject-key field names an input of the (now gone) model
      render()
    },
  }
}

// ---- template generation -------------------------------------------------

// templateCSV builds a spreadsheet-fillable template: a `case` label column, an
// `entity` column (the subject a productive run's quality event is filed on), one
// column per leaf input, and one `→Decision` expected column per decision (leave
// blank for a pure run, fill to assert). Two example rows are pre-filled from the
// inputs' inferred values so the shape is obvious. A leading comment row documents
// the format for humans and AI agents alike.
export function templateCSV(model: ModelDetail): string {
  const fields = leafInputs(model)
  const decisions = model.decisions ?? []
  const header = ['case', 'entity', ...fields.map((f) => f.name), ...decisions.map((d) => '→' + d)]
  const samples = sampleRows(fields, 2)
  const rows = samples.map((inputs, i) => ['Fall ' + (i + 1), '', ...fields.map((f) => csvCell(inputs[f.name])), ...decisions.map(() => '')])
  const comment = `# Testfall-Vorlage für „${model.name || 'Modell'}". Eine Zeile = ein Testfall. „entity" ist die Entität, auf die ein Produktivlauf ein clio-Quality-Event schreibt (optional). Spalten „→Decision" sind erwartete Ergebnisse (optional, für Pass/Fail). Werte als FEEL/JSON: 1200, "Business", true.`
  return [comment, header.map(csvField).join(','), ...rows.map((r) => r.map(csvField).join(','))].join('\n') + '\n'
}

// resultsCSV renders the run's outcome as a spreadsheet: the same case/entity/
// input columns as the template, plus one column per decision holding the
// COMPUTED output, and a trailing `status` column (OK/Abweichung/Fehler) when any
// case asserted or errored. This is what the cockpit offers for download after a
// run — the filled-in test sheet with the output variables written back.
export function resultsCSV(model: ModelDetail, cases: TestCase[]): string {
  const fields = leafInputs(model)
  const decisions = model.decisions ?? []
  const withStatus = cases.some((c) => c.status === 'error' || c.pass !== undefined)
  const header = ['case', 'entity', ...fields.map((f) => f.name), ...decisions, ...(withStatus ? ['status'] : [])]
  const rows = cases.map((c) => [
    c.name,
    c.entity,
    ...fields.map((f) => csvCell(c.inputs[f.name])),
    ...decisions.map((d) => (c.status === 'error' || !c.values ? '' : csvCell(c.values[d]))),
    ...(withStatus ? [statusText(c)] : []),
  ])
  const comment = `# Ergebnisse für „${model.name || 'Modell'}". Eingaben + berechnete Decision-Ausgaben je Testfall${withStatus ? ' (status: OK/Abweichung/Fehler)' : ''}.`
  return [comment, header.map(csvField).join(','), ...rows.map((r) => r.map(csvField).join(','))].join('\n') + '\n'
}

// statusText labels a run case for the results CSV's status column.
function statusText(c: TestCase): string {
  if (c.status === 'error') return 'Fehler'
  if (c.pass === true) return 'OK'
  if (c.pass === false) return 'Abweichung'
  return ''
}

// templateJSON builds the same template as a JSON document: a model name and a
// `cases` array of { name, entity, input, expect }. AI-agent-friendly and
// round-trips through importFile. One example case is pre-filled.
export function templateJSON(model: ModelDetail): string {
  const fields = leafInputs(model)
  const [sample] = sampleRows(fields, 1)
  const expect: Record<string, unknown> = {}
  for (const d of model.decisions ?? []) expect[d] = null
  const doc = {
    _hinweis: 'Testfälle für die Import-Cockpit. "input" füllt die Modell-Eingaben; "expect" (optional) sind erwartete Decision-Ergebnisse für Pass/Fail; "entity" (optional) ist die Entität, auf die ein Produktivlauf ein clio-Quality-Event schreibt.',
    model: model.name || model.modelId,
    cases: [{ name: 'Fall 1', entity: '', input: sample ?? {}, expect }],
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
  // Drop whole-line comments BEFORE the quote-aware CSV parse. The template's
  // leading comment holds double-quotes (German „…", plus a "Business" example),
  // an odd count — letting parseCSV see them turns quoting on and swallows the
  // whole rest of the file into one field, so no cases are found. Comments are
  // always whole lines, so strip them at the line level first.
  const body = text
    .split(/\r\n|\r|\n/)
    .filter((line) => !line.trimStart().startsWith('#'))
    .join('\n')
  const table = parseCSV(body).filter((row) => row.length && !(row.length === 1 && row[0].trim() === ''))
  if (table.length < 2) return []
  const header = table[0]
  const kinds = header.map((h) => classifyColumn(h.trim(), inputNames))
  const out: Omit<TestCase, 'id'>[] = []
  for (const row of table.slice(1)) {
    const inputs: Record<string, unknown> = {}
    const expected: Record<string, unknown> = {}
    let name = ''
    let entity = ''
    row.forEach((raw, i) => {
      const k = kinds[i]
      if (!k) return
      if (k.kind === 'label') name = raw.trim()
      else if (k.kind === 'entity') entity = raw.trim()
      else if (k.kind === 'input') {
        const v = coerce(raw)
        if (v !== undefined) inputs[k.name] = v
      } else {
        const v = coerce(raw)
        if (v !== undefined) expected[k.name] = v
      }
    })
    if (Object.keys(inputs).length || name) out.push({ name, entity, inputs, expected, lane: 'in', status: 'staged' })
  }
  return out
}

type Column = { kind: 'label' | 'entity' | 'input' | 'expect'; name: string } | null
function classifyColumn(h: string, inputNames: Set<string>): Column {
  const low = h.toLowerCase()
  if (low === 'case' || low === 'name' || low === 'fall' || low === '#' || low === '') return { kind: 'label', name: '' }
  if (low === 'entity' || low === 'entität' || low === 'entitaet' || low === 'subjekt' || low === 'subject') return { kind: 'entity', name: '' }
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
    const entity = typeof o.entity === 'string' ? o.entity : typeof o.subject === 'string' ? o.subject : ''
    if (Object.keys(input).length || o.name) out.push({ name, entity, inputs: input, expected: expect, lane: 'in', status: 'staged' })
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
  const { name: _n, entity: _e, subject: _s, ...rest } = o
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
function computePass(expected: Record<string, unknown>, values: Record<string, unknown>): boolean | undefined {
  const keys = Object.keys(expected)
  if (!keys.length) return undefined
  return keys.every((k) => looseEqual(values[k], expected[k]))
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
  const parts = [`${done.length.toLocaleString('de-CH')} gelaufen`]
  if (asserted.length) parts.push(`${passed}/${asserted.length} bestanden`)
  if (errs) parts.push(`${errs} Fehler`)
  return parts.join(' · ')
}

// fmtDuration renders an elapsed millisecond span compactly (sub-second in ms).
function fmtDuration(ms: number): string {
  return ms < 1000 ? Math.round(ms) + ' ms' : (ms / 1000).toFixed(2) + ' s'
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
