import type { EvalRun } from './evaluate'
import type { TableTrace } from './api'

// The Operate view (ADR-0016): a runtime cockpit, deliberately distinct from the
// Design editor. It has three parts, all fed from the same session run history:
//   1. a keyboard-navigable history strip ABOVE the diagram (this module),
//   2. frosted, floating overlays OVER the diagram summarising the active run's
//      inputs and results (this module), and
//   3. hover graphics — a decision's table drawn as a matrix with the hit rule
//      highlighted, and numeric values as mini bars (this module).
// The diagram itself and the on-node result pills are drawn by canvas.ts; this
// module only renders the surrounding operate chrome and drives run selection
// via onActivate (which the shell turns into a canvas overlay update).

export type OperateOptions = {
  // Where the run-history listbox is rendered (above the diagram).
  historyHost: HTMLElement
  // Where the floating summary overlays are rendered (over the diagram).
  overlayHost: HTMLElement
  // The session runs, newest-first (index 0 is the newest).
  getRuns: () => EvalRun[]
  // The currently active run (whose values the diagram is showing), or null.
  getActive: () => EvalRun | null
  // Called when the user picks/navigates to a run: the shell makes it active and
  // repaints the diagram + on-node pills, then calls render() to refresh here.
  onActivate: (run: EvalRun) => void
}

export type OperateView = {
  // render redraws the history strip and overlays from the current runs/active.
  render: () => void
  // focusHistory moves keyboard focus into the history listbox.
  focusHistory: () => void
}

// mountOperate wires up the operate chrome once; render() (called by the shell on
// every run change / mode switch) refreshes the contents from the live state.
export function mountOperate(opts: OperateOptions): OperateView {
  const { historyHost, overlayHost, getRuns, getActive, onActivate } = opts

  // The overlay host keeps a persistent frame (a show/hide toggle + the panel
  // area) so toggling doesn't fight with render() rebuilding the panels.
  overlayHost.textContent = ''
  const toggle = el('button', 'op-ov-toggle') as HTMLButtonElement
  toggle.type = 'button'
  toggle.title = 'Overlays ein-/ausblenden'
  toggle.textContent = 'Infos'
  const panels = el('div', 'op-ov-panels')
  const pop = el('div', 'op-pop') // shared hover popover, positioned per row
  pop.hidden = true
  overlayHost.append(toggle, panels, pop)
  toggle.addEventListener('click', () => {
    overlayHost.classList.toggle('op-ov-off')
    toggle.classList.toggle('is-off', overlayHost.classList.contains('op-ov-off'))
  })

  // History keyboard navigation: the listbox is the single tab stop; a roving
  // aria-activedescendant marks the active option. Arrows/j/k move by one,
  // Home/End jump to the ends, Enter re-opens (pulses) the active run. All handled
  // keys call preventDefault so the page never scrolls under the keyboard.
  historyHost.addEventListener('keydown', (e) => {
    const runs = getRuns()
    if (!runs.length) return
    const active = getActive()
    const cur = active ? runs.indexOf(active) : 0
    let next = cur
    switch (e.key) {
      case 'ArrowDown':
      case 'ArrowRight':
      case 'j':
        next = Math.min(runs.length - 1, cur + 1)
        break
      case 'ArrowUp':
      case 'ArrowLeft':
      case 'k':
        next = Math.max(0, cur - 1)
        break
      case 'Home':
        next = 0
        break
      case 'End':
        next = runs.length - 1
        break
      case 'Enter':
      case ' ':
        e.preventDefault()
        pulseActive()
        return
      default:
        return
    }
    e.preventDefault()
    if (next !== cur) onActivate(runs[next])
  })

  const pulseActive = (): void => {
    const opt = historyHost.querySelector<HTMLElement>('.op-run.is-active')
    if (!opt) return
    opt.classList.remove('op-run-pulse')
    // Force reflow so re-adding the class restarts the animation.
    void opt.offsetWidth
    opt.classList.add('op-run-pulse')
  }

  const renderHistory = (): void => {
    historyHost.textContent = ''
    const runs = getRuns()
    historyHost.setAttribute('role', 'listbox')
    historyHost.setAttribute('aria-label', 'Läufe dieser Session (mit Pfeiltasten blätterbar)')
    historyHost.tabIndex = 0
    if (!runs.length) {
      historyHost.removeAttribute('aria-activedescendant')
      historyHost.append(el('p', 'op-empty', 'Noch keine Auswertung in dieser Session. Unten Eingaben füllen und „Auswerten" — die Läufe erscheinen hier und lassen sich per Tastatur (↑/↓, j/k, Pos1/Ende) durchblättern.'))
      return
    }
    const active = getActive()
    const list = el('div', 'op-runs')
    runs.forEach((run, i) => {
      const n = runs.length - i // newest run carries the highest number
      const id = 'op-run-' + i
      const row = el('div', 'op-run' + (run === active ? ' is-active' : ''))
      row.id = id
      row.setAttribute('role', 'option')
      row.setAttribute('aria-selected', run === active ? 'true' : 'false')
      row.append(el('span', 'op-run-n', 'Lauf ' + n), el('span', 'op-run-in', summarizeInputs(run.inputs)))
      row.addEventListener('click', () => {
        onActivate(run)
        historyHost.focus()
      })
      list.append(row)
      if (run === active) {
        historyHost.setAttribute('aria-activedescendant', id)
        // Keep the active option in view as the user blätters through.
        requestAnimationFrame(() => row.scrollIntoView({ block: 'nearest', inline: 'nearest' }))
      }
    })
    historyHost.append(list)
  }

  const renderOverlays = (): void => {
    panels.textContent = ''
    pop.hidden = true
    const active = getActive()
    if (!active) return

    // EINGANGSDATEN — top-left frosted panel.
    const inRows = Object.entries(active.inputs)
    const inPanel = kvPanel('op-ov-inputs', 'Eingangsdaten', 'group')
    if (!inRows.length) inPanel.body.append(el('div', 'op-ov-none', '(keine Eingaben)'))
    for (const [k, v] of inRows) inPanel.body.append(kvRow(k, v, numericBar(v, inRows.map(([, x]) => x))))
    panels.append(inPanel.root)

    // ERGEBNISSE — bottom-right frosted panel; each row can reveal its decision
    // table (or a value bar) on hover.
    const rules = matchedRulesOf(active)
    const outRows = Object.entries(active.result.values)
    const outPanel = kvPanel('op-ov-results', 'Ergebnisse', 'group')
    let hoverable = 0
    for (const [name, v] of outRows) {
      const chip = rules[name]?.length ? el('span', 'op-ov-rule', 'Regel ' + rules[name].join(', ')) : undefined
      const row = kvRow(name, v, numericBar(v, outRows.map(([, x]) => x)), chip)
      const tables = active.result.traces?.[name]?.tables ?? []
      if (tables.length || typeof v === 'number') {
        hoverable++
        // The ⊞ marker (see CSS) signals the row reveals a graphic on hover.
        row.classList.add('op-ov-hoverable')
        row.addEventListener('pointerenter', () => showPop(row, name, v, tables))
        row.addEventListener('pointerleave', () => { pop.hidden = true })
      }
      outPanel.body.append(row)
    }
    if (!outRows.length) outPanel.body.append(el('div', 'op-ov-none', '(keine Ergebnisse)'))
    if (hoverable) outPanel.body.append(el('div', 'op-ov-tip', 'Tipp: Zeilen mit ⊞ zeigen beim Überfahren die Tabelle mit der getroffenen Regel. Doppelklick auf die Decision im Diagramm öffnet die volle Ansicht.'))
    panels.append(outPanel.root)
  }

  // showPop draws the hover graphic for a result row into the shared popover and
  // positions it next to the row. Decision-table traces render as a matrix with
  // the hit rule highlighted; a bare numeric result renders as a labelled bar.
  const showPop = (row: HTMLElement, name: string, value: unknown, tables: TableTrace[]): void => {
    pop.textContent = ''
    pop.append(el('div', 'op-pop-title', name))
    if (tables.length) {
      tables.forEach((tt, i) => pop.append(miniTable(tt, tables.length > 1 ? i + 1 : 0)))
    } else if (typeof value === 'number') {
      pop.append(valueGauge(value))
    }
    pop.hidden = false
    // The popover is position:fixed, so anchor it in viewport coordinates
    // (independent of where the overlay host sits) — above the row, or below if
    // there's no room, clamped to the viewport so it never lands off-screen.
    const r = row.getBoundingClientRect()
    const top = r.top - pop.offsetHeight - 10
    const left = Math.max(8, Math.min(r.left, window.innerWidth - pop.offsetWidth - 8))
    pop.style.left = left + 'px'
    pop.style.top = (top < 8 ? r.bottom + 10 : top) + 'px'
  }

  const render = (): void => {
    renderHistory()
    renderOverlays()
  }

  return {
    render,
    focusHistory: () => historyHost.focus(),
  }
}

// kvPanel builds a titled frosted overlay panel and returns its root + body.
function kvPanel(cls: string, title: string, role: string): { root: HTMLElement; body: HTMLElement } {
  const root = el('div', 'op-ov ' + cls)
  root.setAttribute('role', role)
  root.setAttribute('aria-label', title)
  root.append(el('div', 'op-ov-title', title))
  const body = el('div', 'op-ov-body')
  root.append(body)
  return { root, body }
}

// kvRow is one key/value line in an overlay, with an optional value bar and chip.
function kvRow(key: string, value: unknown, bar?: HTMLElement | null, chip?: HTMLElement): HTMLElement {
  const row = el('div', 'op-ov-row')
  row.append(el('span', 'op-ov-key', key))
  const val = el('span', 'op-ov-val', fmt(value))
  row.append(val)
  if (chip) row.append(chip)
  if (bar) row.append(bar)
  return row
}

// numericBar renders a mini bar for a numeric value, scaled against the largest
// magnitude among the panel's values (so a row is comparable to its neighbours).
// Returns null for non-numeric values (graceful degrade — just the number shows).
function numericBar(value: unknown, siblings: unknown[]): HTMLElement | null {
  if (typeof value !== 'number' || !Number.isFinite(value)) return null
  const max = Math.max(1e-9, ...siblings.filter((v): v is number => typeof v === 'number' && Number.isFinite(v)).map((v) => Math.abs(v)))
  const pct = Math.min(100, (Math.abs(value) / max) * 100)
  const track = el('span', 'op-bar')
  const fill = el('span', 'op-bar-fill' + (value < 0 ? ' is-neg' : ''))
  fill.style.width = pct.toFixed(1) + '%'
  track.append(fill)
  return track
}

// valueGauge is the hover graphic for a bare numeric result: a labelled full bar.
function valueGauge(value: number): HTMLElement {
  const wrap = el('div', 'op-pop-gauge')
  const track = el('span', 'op-bar op-bar-lg')
  const fill = el('span', 'op-bar-fill')
  fill.style.width = '100%'
  track.append(fill)
  wrap.append(track, el('span', 'op-pop-num', fmt(value)))
  return wrap
}

// miniTable draws a decision-table trace as a compact matrix: one row per rule,
// input columns + output, the matched rule highlighted and each cell tinted by
// whether its condition held — the "table as graphic" (Baustein 3).
function miniTable(tt: TableTrace, n: number): HTMLElement {
  const block = el('div', 'op-mtable')
  const matched = tt.matched ?? []
  const policy = tt.hitPolicy + (tt.aggregation ? ' ' + tt.aggregation : '')
  const head = matched.length ? 'Regel ' + matched.map((m) => m + 1).join(', ') : 'keine Regel'
  block.append(el('div', 'op-mtable-head', (n ? 'Tabelle ' + n + ' · ' : '') + head, el('span', 'op-mtable-policy', policy)))

  const grid = el('table', 'op-mgrid')
  const hr = el('tr', '')
  hr.append(el('th', 'op-mcol-idx', '#'))
  for (const in_ of tt.inputs ?? []) hr.append(el('th', '', in_.expression))
  hr.append(el('th', '', '→'))
  grid.append(hr)

  const nIn = (tt.inputs ?? []).length
  for (const r of tt.rules ?? []) {
    const tr = el('tr', r.matched ? 'op-mrule is-hit' : 'op-mrule')
    tr.append(el('td', 'op-mcol-idx', String(r.index + 1)))
    for (let k = 0; k < nIn; k++) {
      const cond = r.conditions?.[k]
      const cls = cond ? (cond.matched ? 'op-mcell is-ok' : 'op-mcell is-no') : 'op-mcell is-skip'
      tr.append(el('td', cls, cond ? cellText(cond.entry) : ''))
    }
    tr.append(el('td', 'op-mout', r.matched && r.outputs ? r.outputs.map(fmt).join(', ') : ''))
    grid.append(tr)
  }
  block.append(grid)
  return block
}

// matchedRulesOf maps each decision name to its fired rule numbers (1-based).
function matchedRulesOf(run: EvalRun): Record<string, number[]> {
  const out: Record<string, number[]> = {}
  for (const [name, tr] of Object.entries(run.result.traces ?? {})) {
    const rules: number[] = []
    for (const t of tr.tables ?? []) for (const m of t.matched ?? []) rules.push(m + 1)
    if (rules.length) out[name] = rules
  }
  return out
}

function summarizeInputs(inputs: Record<string, unknown>): string {
  const parts = Object.entries(inputs).map(([k, v]) => k + '=' + fmt(v))
  return parts.length ? parts.join(', ') : '(keine Eingaben)'
}

function cellText(text: string | undefined): string {
  const s = (text ?? '').trim()
  return s === '' || s === '-' ? '–' : s
}

function fmt(v: unknown): string {
  if (v === null || v === undefined) return 'null'
  if (typeof v === 'string') return v
  return JSON.stringify(v)
}

// el is a tiny DOM builder: tag, class, then string/Node children.
function el(tag: string, cls: string, ...kids: (string | Node)[]): HTMLElement {
  const n = document.createElement(tag)
  if (cls) n.className = cls
  n.append(...kids)
  return n
}
