// The "Flows" view (WP-97, WP-98): browse and run decision flows (ADR-0026). A
// catalog in the sidebar lists the registered flows; selecting one draws its steps
// as an auto-laid-out graph (the cross-model DRG) and shows a run panel — fill the
// flow's inputs, evaluate, and the canvas *illuminates*: each step lights up with
// its result, every wire shows the value that travelled it, and an
// "Entscheidungspfad" panel lists — from the real evaluation trace — which rules
// fired, in order.

import { listFlows, getFlow, evaluateFlow } from './api'
import type { EvalResult, FlowDetail, Graph, GraphEdge, GraphNode, Trace } from './api'
import { layout } from './layout'
import type { Laid } from './layout'
import { renderFlowGraph } from './flow-canvas'
import type { FlowCanvas, NodeLight, EdgeLight } from './flow-canvas'

// reveal cadence: each step of dependency depth lights up this many ms after the
// previous, so the illumination visibly flows from inputs to output.
export const STEP_MS = 140

// FlowView is the mounted view: render refreshes the catalog (and reopens the
// current flow); open() opens a specific flow into the studio (used after the
// designer registers a new flow).
export type FlowView = { render: () => void; open: (id: string) => void }

// FlowMounts are the two hosts the view fills: the catalog list (in the sidebar)
// and the studio (canvas + run panel, in the editor area). onOpenFlow is called
// when the user opens a flow, so the shell can switch the editor to the studio;
// onEditFlow is called when the user asks to edit the open flow, so the shell can
// switch to the flow designer prefilled from it.
export type FlowMounts = {
  catalogHost: HTMLElement
  studioHost: HTMLElement
  onOpenFlow?: () => void
  onEditFlow?: (detail: FlowDetail) => void
}

// escapeRe escapes a name for use as a literal in a RegExp.
function escapeRe(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

// esc escapes text for safe inclusion in innerHTML.
export function esc(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

// references reports whether a mapping expression refers to name — as a whole
// word, so "risk" matches "risk.Risk Level" and "get value(risk, …)" but not
// "risky", and "Credit Score" matches "Credit Score - 50".
function references(expr: string, name: string): boolean {
  return new RegExp('\\b' + escapeRe(name) + '\\b').test(expr)
}

// buildGraph turns a flow's steps and inputs into a Graph the layouter positions:
// step nodes (drawn as decisions), input nodes (drawn as input data), and edges
// derived from which steps/inputs each step's mappings reference.
export function buildGraph(detail: FlowDetail): Graph {
  const stepIds = new Set(detail.steps.map((s) => s.id))
  const inputNames = (detail.inputs ?? []).map((i) => i.name)

  const nodes: GraphNode[] = []
  for (const name of inputNames) nodes.push({ id: 'in:' + name, type: 'inputData', name })
  for (const s of detail.steps) nodes.push({ id: s.id, type: 'decision', name: s.decision, varName: s.id })

  const seen = new Set<string>()
  const edges: GraphEdge[] = []
  const add = (source: string, target: string): void => {
    const key = source + ' ' + target
    if (seen.has(key)) return
    seen.add(key)
    edges.push({ type: 'informationRequirement', source, target })
  }
  for (const s of detail.steps) {
    for (const expr of Object.values(s.in ?? {})) {
      for (const dep of stepIds) if (dep !== s.id && references(expr, dep)) add(dep, s.id)
      for (const name of inputNames) if (references(expr, name)) add('in:' + name, s.id)
    }
  }
  return { nodes, edges }
}

// depthMap returns each node's dependency depth: inputs (no incoming edge) are 0,
// every other node is one deeper than the deepest node it requires. It drives the
// staggered reveal so a step never lights up before its inputs.
export function depthMap(graph: Graph): Record<string, number> {
  const incoming = new Map<string, string[]>()
  for (const n of graph.nodes) incoming.set(n.id, [])
  for (const e of graph.edges) incoming.get(e.target)?.push(e.source)
  const memo = new Map<string, number>()
  const of = (id: string, seen: Set<string>): number => {
    const c = memo.get(id)
    if (c !== undefined) return c
    if (seen.has(id)) return 0 // cycle guard (a sound flow is acyclic)
    seen.add(id)
    const ins = incoming.get(id) ?? []
    const d = ins.length ? 1 + Math.max(...ins.map((s) => of(s, seen))) : 0
    seen.delete(id)
    memo.set(id, d)
    return d
  }
  const out: Record<string, number> = {}
  for (const n of graph.nodes) out[n.id] = of(n.id, new Set())
  return out
}

// coerce turns a form string into a value: JSON when it parses (numbers, booleans,
// lists), otherwise the raw string; blank means "omitted".
export function coerce(raw: string): unknown {
  const s = raw.trim()
  if (s === '') return undefined
  try {
    return JSON.parse(s)
  } catch {
    return raw
  }
}

// fmt renders a value for a badge, wire label or the output table.
export function fmt(v: unknown): string {
  if (v === null || v === undefined) return 'null'
  if (typeof v === 'string') return v
  return JSON.stringify(v)
}

// renderTrace turns a flow's evaluation trace into the "Entscheidungspfad": one
// block per decision table that ran, in evaluation order — the values it tested
// and the rule(s) that fired. When the table count matches the flow's steps the
// blocks are labelled by step decision (the common one-table-per-step case);
// otherwise they fall back to a plain ordinal, so the panel never over-claims.
export function renderTrace(trace: Trace | undefined, detail: FlowDetail): string {
  const tables = trace?.tables ?? []
  if (!tables.length) return ''
  const aligned = tables.length === detail.steps.length
  const blocks = tables.map((t, i) => {
    const label = aligned ? detail.steps[i].decision : 'Auswertung ' + (i + 1)
    const inputs = t.inputs
      .map((inp) => `<div class="flow-trace-io"><span class="flow-trace-k">${esc(inp.expression)}</span><span class="flow-trace-v">${esc(fmt(inp.value))}</span></div>`)
      .join('')
    const matched = t.matched
      .map((mi) => {
        const rule = t.rules.find((r) => r.index === mi)
        const outs = (rule?.outputs ?? []).map((o) => fmt(o)).join(', ')
        return `<span class="flow-trace-rule">R${mi + 1}${outs ? ' → ' + esc(outs) : ''}</span>`
      })
      .join('')
    return `<div class="flow-trace-step"><div class="flow-trace-head"><span class="flow-trace-name">${esc(label)}</span><span class="flow-trace-hp" title="Hit Policy">${esc(t.hitPolicy)}</span></div>${inputs}<div class="flow-trace-matched">${matched || '—'}</div></div>`
  })
  return `<h3 class="flow-trace-title">Entscheidungspfad</h3>${blocks.join('')}`
}

// applyIllumination lights the flow canvas after a run: a result badge on each
// step node and the value that travelled each wire, staggered by dependency depth
// so the evaluation visibly propagates from inputs to output. Shared by the flow
// runner (studio) and the designer's live test (flow-editor).
export function applyIllumination(
  fc: FlowCanvas,
  detail: FlowDetail,
  laid: Laid,
  depth: Record<string, number>,
  input: Record<string, unknown>,
  res: EvalResult,
): void {
  // Each step's result: res.decisions is keyed "stepId.output".
  const nodes: Record<string, NodeLight> = {}
  for (const s of detail.steps) {
    let val: string | undefined
    for (const [k, v] of Object.entries(res.decisions ?? {})) {
      if (k === s.id + '.' + s.decision || k.startsWith(s.id + '.')) val = fmt(v)
    }
    if (val !== undefined) nodes[s.id] = { value: val, delay: (depth[s.id] ?? 0) * STEP_MS }
  }
  // Each wire's value: a flow input carries what the user entered; a step→step
  // wire carries the source step's result.
  const edges: Record<string, EdgeLight> = {}
  for (const e of laid.edges) {
    let val: string | undefined
    if (e.source.startsWith('in:')) {
      const name = e.source.slice(3)
      if (name in input) val = fmt(input[name])
    } else {
      val = nodes[e.source]?.value
    }
    if (val !== undefined) edges[e.id] = { value: val, delay: (depth[e.target] ?? 0) * STEP_MS }
  }
  fc.illuminate({ nodes, edges })
}

export function mountFlows(opts: FlowMounts): FlowView {
  const { catalogHost, studioHost, onOpenFlow, onEditFlow } = opts

  studioHost.innerHTML = `
    <div class="flow-canvas" id="flowCanvas"></div>
    <aside class="flow-run">
      <div class="flow-run-head">
        <h2 class="eval-title">Flow auswerten</h2>
        <button id="flowEdit" class="tbtn flow-edit-btn" type="button" disabled title="Diesen Flow im Designer bearbeiten">✎ Bearbeiten</button>
      </div>
      <div class="flow-meta" id="flowMeta"></div>
      <div class="flow-inputs eval-inputs" id="flowInputs"></div>
      <button id="flowRun" class="tbtn" type="button" disabled>Auswerten</button>
      <div class="flow-out" id="flowOut"></div>
      <div class="flow-trace" id="flowTrace"></div>
    </aside>`
  const canvasHost = studioHost.querySelector<HTMLElement>('#flowCanvas')!
  const meta = studioHost.querySelector<HTMLElement>('#flowMeta')!
  const inputsHost = studioHost.querySelector<HTMLElement>('#flowInputs')!
  const runBtn = studioHost.querySelector<HTMLButtonElement>('#flowRun')!
  const editBtn = studioHost.querySelector<HTMLButtonElement>('#flowEdit')!
  const outHost = studioHost.querySelector<HTMLElement>('#flowOut')!
  const traceHost = studioHost.querySelector<HTMLElement>('#flowTrace')!
  editBtn.addEventListener('click', () => {
    if (currentDetail) onEditFlow?.(currentDetail)
  })

  let currentId = ''
  let currentDetail: FlowDetail | null = null
  let currentLaid: Laid | null = null
  let currentDepth: Record<string, number> = {}
  let fc: FlowCanvas | null = null

  const run = async (): Promise<void> => {
    if (!currentDetail || !currentLaid) return
    const input: Record<string, unknown> = {}
    for (const field of inputsHost.querySelectorAll<HTMLInputElement>('.flow-input')) {
      const v = coerce(field.value)
      if (v !== undefined) input[field.dataset.name ?? ''] = v
    }
    outHost.textContent = 'Werte aus…'
    traceHost.innerHTML = ''
    try {
      const res = await evaluateFlow(currentId, input, true)
      if (fc) applyIllumination(fc, currentDetail, currentLaid, currentDepth, input, res)

      const rows = Object.entries(res.outputs ?? {})
        .map(([k, v]) => `<tr><td>${esc(k)}</td><td class="flow-out-val">${esc(fmt(v))}</td></tr>`)
        .join('')
      outHost.innerHTML = rows ? `<table class="flow-out-table"><tbody>${rows}</tbody></table>` : 'Kein Ergebnis.'
      traceHost.innerHTML = renderTrace(res.trace, currentDetail)
    } catch (e) {
      fc?.clear()
      outHost.innerHTML = `<div class="flow-error">${esc((e as Error).message)}</div>`
    }
  }
  runBtn.addEventListener('click', () => void run())

  const showFlow = async (id: string): Promise<void> => {
    currentId = id
    // Switch the editor to the flow studio *first*, so its host is visible when
    // the diagram renders (a hidden container has zero size and cannot fit).
    onOpenFlow?.()
    renderCatalog(lastFlows)
    let detail: FlowDetail
    try {
      detail = await getFlow(id)
    } catch (e) {
      meta.innerHTML = `<div class="flow-error">${esc((e as Error).message)}</div>`
      return
    }
    currentDetail = detail
    const graph = buildGraph(detail)
    currentDepth = depthMap(graph)
    currentLaid = layout(graph)
    fc = renderFlowGraph(canvasHost, currentLaid)
    outHost.textContent = ''
    traceHost.innerHTML = ''

    const diags = detail.diagnostics ?? []
    const warn = diags.length
      ? `<div class="flow-warn" title="${esc(diags.map((d) => d.code + ': ' + d.message).join('\n'))}">⚠ ${diags.length} Diagnostic${diags.length === 1 ? '' : 's'} — sind die referenzierten Modelle geladen?</div>`
      : ''
    meta.innerHTML = `<div class="flow-name-lg">${esc(detail.name ?? '(unbenannt)')}</div><div class="flow-sub">${detail.steps.length} Steps</div>${warn}`

    inputsHost.innerHTML = ''
    for (const inp of detail.inputs ?? []) {
      const wrap = document.createElement('label')
      wrap.className = 'eval-field-wrap'
      const lbl = document.createElement('span')
      lbl.className = 'eval-field-label'
      lbl.textContent = inp.type ? `${inp.name} : ${inp.type}` : inp.name
      const field = document.createElement('input')
      field.className = 'eval-field flow-input'
      field.dataset.name = inp.name
      field.placeholder = inp.type ?? ''
      wrap.append(lbl, field)
      inputsHost.append(wrap)
    }
    runBtn.disabled = false
    editBtn.disabled = !onEditFlow
    editBtn.style.display = onEditFlow ? '' : 'none'
  }

  let lastFlows: { flowId: string; name?: string; steps: number }[] = []

  const renderCatalog = (flows: { flowId: string; name?: string; steps: number }[]): void => {
    catalogHost.innerHTML = ''
    if (!flows.length) {
      catalogHost.innerHTML = '<div class="model-empty">Keine Flows registriert. Über <code>POST /v1/flows</code> anlegen.</div>'
      return
    }
    for (const f of flows) {
      const row = document.createElement('div')
      row.className = 'model-item flow-item' + (f.flowId === currentId ? ' is-current' : '')
      row.dataset.flowId = f.flowId
      const name = document.createElement('span')
      name.className = 'model-name'
      name.textContent = f.name || f.flowId
      const rev = document.createElement('span')
      rev.className = 'model-rev'
      rev.textContent = f.steps + ' Steps'
      row.append(name, rev)
      row.addEventListener('click', () => void showFlow(f.flowId))
      catalogHost.append(row)
    }
  }

  const render = (): void => {
    void (async () => {
      let flows
      try {
        flows = await listFlows()
      } catch (e) {
        catalogHost.innerHTML = `<div class="flow-error">${esc((e as Error).message)}</div>`
        return
      }
      lastFlows = flows
      renderCatalog(flows)
      // The catalog lives permanently in the sidebar; render() only lists (and
      // highlights the open flow via currentId). Opening a flow into the studio
      // is an explicit user click, so a refresh never hijacks the editor.
    })()
  }

  return { render, open: (id: string) => void showFlow(id) }
}
