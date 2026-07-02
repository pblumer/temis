import type { ModelDetail, ClioEvent } from './api'
import { fetchClioEvents, evaluateGraph, InputValidationError, ClioReadForbiddenError } from './api'
import type { EvalRun } from './evaluate'

// The clio replay panel (ADR-0033 read side): the Operate-view counterpart to the
// "Auswerten" form. Where "Auswerten" runs inputs the user types, this reads the
// decisions temis already filed in clio — filtered by a SUBJECT + EVENT-TYPE
// mapping the user defines here — and replays each recorded input through the
// open model, so a past decision can be re-run and inspected on the diagram.
//
// The mapping (subject subtree + event type + limit) is the "where do the events
// live in clio" definition the user asked for. It is persisted per model (keyed
// by the model's name, so it survives content-addressed re-saves) in
// localStorage, and pre-filled from the sink's own subject mapping on first load.

export type ClioReplayOptions = {
  // Where the panel is rendered (an Operate-only section under the diagram).
  host: HTMLElement
  // The model currently open in the modeler (for its id + graph evaluation), or
  // null when none is open.
  getModel: () => ModelDetail | null
  // Called with each replayed run so the shell records it in the Operate history
  // and overlays it on the diagram — exactly like a live "Auswerten" run.
  onReplay: (run: EvalRun) => void
}

export type ClioReplayView = {
  // render (re)builds the panel from the current model + persisted mapping. The
  // shell calls it on entering Operate and on model change.
  render: () => void
}

type Mapping = { subject: string; type: string; limit: number }

const DEFAULT_LIMIT = 100

export function mountClioReplay(opts: ClioReplayOptions): ClioReplayView {
  const { host, getModel, onReplay } = opts
  // The last event set loaded, kept so a re-render (e.g. after a replay) can
  // redraw the list without re-querying clio.
  let loaded: ClioEvent[] | null = null
  let lastModelKey = ''

  const render = (): void => {
    const model = getModel()
    const key = modelKey(model)
    // A different model resets the loaded events (its decisions/subjects differ).
    if (key !== lastModelKey) {
      loaded = null
      lastModelKey = key
    }
    host.textContent = ''

    host.append(el('h2', 'clio-title', 'Aus clio nachspielen'))
    if (!model) {
      host.append(el('p', 'clio-note', 'Erst ein Modell öffnen — dann lassen sich hier die in clio protokollierten Entscheidungen einlesen und gegen dieses Modell nachspielen.'))
      return
    }
    host.append(el('p', 'clio-note', 'Definiere, wo in clio die Events liegen (Subject-Teilbaum + Event-Typ), lies sie ein und spiele jede protokollierte Eingabe erneut durch dieses Modell — das Ergebnis erscheint als Lauf oben und auf dem Diagramm.'))

    const map = loadMapping(key)
    const form = el('div', 'clio-form')

    const subjectIn = input('text', map.subject, 'z. B. /decisions oder /decisions/42')
    subjectIn.classList.add('clio-field')
    const typeSel = document.createElement('select')
    typeSel.className = 'clio-field clio-type'
    // Options are filled once we know the server's type list; until then offer the
    // stored value (and a blank "alle Typen") so the control is usable immediately.
    fillTypeOptions(typeSel, KNOWN_TYPES, map.type)
    const limitIn = input('number', String(map.limit), '')
    limitIn.classList.add('clio-field', 'clio-limit')
    limitIn.min = '1'
    limitIn.max = '1000'

    form.append(
      field('Subject', subjectIn, 'Der clio-Pfad, unter dem die Entscheidungen abgelegt sind. Leer = der serverseitig konfigurierte Präfix.'),
      field('Event-Typ', typeSel, 'Nur Events dieses CloudEvents-Typs. „alle Typen" liest den ganzen Teilbaum.'),
      field('Limit', limitIn, 'Höchstzahl eingelesener Events (max. 1000).'),
    )

    const loadBtn = el('button', 'tbtn clio-load', 'Events laden') as HTMLButtonElement
    loadBtn.type = 'button'
    form.append(el('div', 'clio-actions', loadBtn))
    host.append(form)

    const listHost = el('div', 'clio-list')
    const statusLine = el('div', 'clio-status-line')
    host.append(statusLine, listHost)

    // Persist the mapping on any edit so it is there next time this model opens.
    const persist = (): void => saveMapping(key, readForm(subjectIn, typeSel, limitIn))
    subjectIn.addEventListener('change', persist)
    typeSel.addEventListener('change', persist)
    limitIn.addEventListener('change', persist)

    const load = async (): Promise<void> => {
      persist()
      const m = readForm(subjectIn, typeSel, limitIn)
      loadBtn.disabled = true
      statusLine.className = 'clio-status-line'
      statusLine.textContent = 'liest aus clio …'
      listHost.textContent = ''
      try {
        const res = await fetchClioEvents({ subject: m.subject, type: m.type, limit: m.limit })
        if (!res.enabled) {
          statusLine.className = 'clio-status-line clio-off'
          statusLine.textContent = 'Kein clio-Audit-Sink konfiguriert — es können keine Events eingelesen werden. Anschalten: TEMIS_CLIO_TOKEN am Server setzen.'
          return
        }
        // Refresh the type dropdown with the server's authoritative list and
        // pre-fill the subject with the sink's prefix if the field is still empty.
        fillTypeOptions(typeSel, res.types.length ? res.types : KNOWN_TYPES, m.type)
        if (!subjectIn.value.trim() && res.subjectPrefix) subjectIn.value = res.subjectPrefix
        loaded = res.events
        renderList(listHost, statusLine, res, doReplay)
      } catch (e) {
        statusLine.className = 'clio-status-line clio-error'
        if (e instanceof ClioReadForbiddenError) statusLine.textContent = e.message
        else statusLine.textContent = (e as Error).message
      } finally {
        loadBtn.disabled = false
      }
    }
    loadBtn.addEventListener('click', () => void load())

    // Replaying an event: run its recorded input through the open model's whole
    // graph and hand the run to the shell. Returns a short status for the row.
    const doReplay = async (ev: ClioEvent): Promise<{ ok: boolean; message: string }> => {
      const input = ev.input ?? {}
      try {
        const result = await evaluateGraph(model.modelId, input, true, true)
        onReplay({ inputs: input, result })
        return { ok: true, message: 'nachgespielt — als Lauf oben' }
      } catch (e) {
        if (e instanceof InputValidationError) {
          return { ok: false, message: e.problems.map((p) => p.input + ': ' + p.message).join(' · ') }
        }
        return { ok: false, message: (e as Error).message }
      }
    }

    // If a previous load in this session already fetched events for this model,
    // redraw them without hitting clio again.
    if (loaded) {
      renderList(listHost, statusLine, { enabled: true, types: KNOWN_TYPES, events: loaded }, doReplay)
    }
  }

  return { render }
}

// renderList draws the loaded events and their replay controls. A per-event
// button replays one; a header button replays all in order.
function renderList(
  listHost: HTMLElement,
  statusLine: HTMLElement,
  res: { enabled: boolean; types: string[]; events: ClioEvent[]; subject?: string },
  doReplay: (ev: ClioEvent) => Promise<{ ok: boolean; message: string }>,
): void {
  listHost.textContent = ''
  const events = res.events
  if (!events.length) {
    statusLine.className = 'clio-status-line'
    statusLine.textContent = 'Keine Events unter diesem Subject/Typ gefunden.'
    return
  }
  statusLine.className = 'clio-status-line clio-ok'
  const where = res.subject ? ' unter ' + res.subject : ''
  statusLine.textContent = events.length + ' Event(s)' + where + ' eingelesen.'

  const head = el('div', 'clio-list-head')
  const replayAll = el('button', 'tbtn clio-replay-all', 'Alle nachspielen') as HTMLButtonElement
  replayAll.type = 'button'
  head.append(el('span', 'clio-list-count', events.length + ' Event(s)'), replayAll)
  listHost.append(head)

  const rows: { ev: ClioEvent; setStatus: (ok: boolean, msg: string) => void }[] = []
  for (const ev of events) {
    const row = el('div', 'clio-ev')
    const meta = el('div', 'clio-ev-meta')
    meta.append(
      el('span', 'clio-ev-type', shortType(ev.type)),
      el('span', 'clio-ev-subject', ev.subject),
    )
    if (ev.decision) meta.append(el('span', 'clio-ev-decision', ev.decision))
    if (ev.time) meta.append(el('span', 'clio-ev-time', formatTime(ev.time)))

    const io = el('div', 'clio-ev-io')
    io.append(el('span', 'clio-ev-in', 'in: ' + summarize(ev.input)))
    if (ev.outputs && Object.keys(ev.outputs).length) io.append(el('span', 'clio-ev-out', 'out: ' + summarize(ev.outputs)))

    const rowStatus = el('span', 'clio-ev-status')
    const btn = el('button', 'tbtn clio-ev-replay', 'Nachspielen') as HTMLButtonElement
    btn.type = 'button'
    const setStatus = (ok: boolean, msg: string): void => {
      rowStatus.className = 'clio-ev-status ' + (ok ? 'clio-ok' : 'clio-error')
      rowStatus.textContent = msg
    }
    btn.addEventListener('click', () => {
      btn.disabled = true
      void doReplay(ev).then((r) => {
        setStatus(r.ok, r.message)
        btn.disabled = false
      })
    })
    row.append(meta, io, el('div', 'clio-ev-actions', btn, rowStatus))
    listHost.append(row)
    rows.push({ ev, setStatus })
  }

  replayAll.addEventListener('click', () => {
    replayAll.disabled = true
    // Replay sequentially so the run history stays ordered oldest→newest applied.
    void (async () => {
      for (const { ev, setStatus } of rows) {
        const r = await doReplay(ev)
        setStatus(r.ok, r.message)
      }
      replayAll.disabled = false
    })()
  })
}

// KNOWN_TYPES is the fallback event-type list offered before the server's own
// list is known (it mirrors service.clioReplayTypes). "" is "alle Typen".
const KNOWN_TYPES = ['com.temis.decision.evaluated.v1', 'com.temis.flow.evaluated.v1', 'com.temis.decision.requested.v1']

function fillTypeOptions(sel: HTMLSelectElement, types: string[], selected: string): void {
  sel.textContent = ''
  const blank = document.createElement('option')
  blank.value = ''
  blank.textContent = 'alle Typen'
  sel.append(blank)
  const all = selected && !types.includes(selected) ? [...types, selected] : types
  for (const t of all) {
    const o = document.createElement('option')
    o.value = t
    o.textContent = shortType(t)
    sel.append(o)
  }
  sel.value = selected
}

// shortType renders a CloudEvents type as a friendlier label.
function shortType(t: string): string {
  switch (t) {
    case 'com.temis.decision.evaluated.v1':
      return 'Decision ausgewertet'
    case 'com.temis.flow.evaluated.v1':
      return 'Flow ausgewertet'
    case 'com.temis.decision.requested.v1':
      return 'Decision angefragt (Command)'
    default:
      return t
  }
}

// --- mapping persistence (per model name, survives content-addressed re-saves) ---

function modelKey(model: ModelDetail | null): string {
  if (!model) return ''
  return model.name?.trim() || model.modelId
}

function storageKey(key: string): string {
  return 'temis.clio-mapping:' + key
}

function loadMapping(key: string): Mapping {
  const fallback: Mapping = { subject: '', type: KNOWN_TYPES[0], limit: DEFAULT_LIMIT }
  if (!key) return fallback
  try {
    const raw = localStorage.getItem(storageKey(key))
    if (!raw) return fallback
    const m = JSON.parse(raw) as Partial<Mapping>
    return {
      subject: typeof m.subject === 'string' ? m.subject : '',
      type: typeof m.type === 'string' ? m.type : KNOWN_TYPES[0],
      limit: typeof m.limit === 'number' && m.limit > 0 ? m.limit : DEFAULT_LIMIT,
    }
  } catch {
    return fallback
  }
}

function saveMapping(key: string, m: Mapping): void {
  if (!key) return
  try {
    localStorage.setItem(storageKey(key), JSON.stringify(m))
  } catch {
    // localStorage may be unavailable (private mode) — the panel still works,
    // the mapping just won't persist.
  }
}

function readForm(subjectIn: HTMLInputElement, typeSel: HTMLSelectElement, limitIn: HTMLInputElement): Mapping {
  const limit = Number.parseInt(limitIn.value, 10)
  return {
    subject: subjectIn.value.trim(),
    type: typeSel.value,
    limit: Number.isFinite(limit) && limit > 0 ? Math.min(limit, 1000) : DEFAULT_LIMIT,
  }
}

// --- formatting helpers ---

function summarize(obj: Record<string, unknown> | undefined): string {
  if (!obj) return '(leer)'
  const parts = Object.entries(obj).map(([k, v]) => k + '=' + fmt(v))
  return parts.length ? parts.join(', ') : '(leer)'
}

function formatTime(iso: string): string {
  // Show a compact local time; fall back to the raw string if unparseable.
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString()
}

function fmt(v: unknown): string {
  if (v === null || v === undefined) return 'null'
  if (typeof v === 'string') return v
  return JSON.stringify(v)
}

// field wraps a labelled control with an optional hint below it.
function field(label: string, control: HTMLElement, hint: string): HTMLElement {
  const wrap = el('div', 'clio-field-wrap')
  wrap.append(el('label', 'clio-field-label', label), control)
  if (hint) wrap.append(el('div', 'clio-field-hint', hint))
  return wrap
}

function input(type: string, value: string, placeholder: string): HTMLInputElement {
  const n = document.createElement('input')
  n.type = type
  n.value = value
  if (placeholder) n.placeholder = placeholder
  return n
}

// el is a tiny DOM builder: tag, class, then string/Node children.
function el(tag: string, cls: string, ...kids: (string | Node)[]): HTMLElement {
  const n = document.createElement(tag)
  if (cls) n.className = cls
  n.append(...kids)
  return n
}
