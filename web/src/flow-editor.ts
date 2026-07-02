// The Flow Designer (WP-116): create, design and test decision flows (ADR-0026)
// visually, not just browse and run them (WP-97). A structured inspector edits the
// flow descriptor — its declared inputs, its steps (each picking a model + decision
// and wiring that decision's inputs with FEEL references) and its output mapping —
// while a live graph preview (the same read-only flow canvas the studio uses)
// redraws the cross-model DRG as you go. "Testen" evaluates the draft inline
// (POST /v1/flow/evaluate, no registration) and illuminates the preview; "Prüfen"
// and "Registrieren" validate against the loaded models; "Export" downloads the
// *.flow.json artifact for the git repo (Git stays the durable source of truth —
// ADR-0032 — so the server store is only the ephemeral, content-addressed catalog).

import { createFlow, evaluateFlowInline, getModel, listModels } from './api'
import type { EvalResult, FlowDescriptor, FlowDetail, InputField, ModelDetail, ModelSummary } from './api'
import { FEEL_TYPES } from './feeltypes'
import { layout } from './layout'
import type { Laid } from './layout'
import { renderFlowGraph } from './flow-canvas'
import type { FlowCanvas } from './flow-canvas'
import { applyIllumination, buildGraph, coerce, depthMap, esc, fmt, renderTrace } from './flows'
import { attachJsonEditor } from './json-editor'

// --- working draft (a mutable, editor-friendly form of flow.Descriptor) ---

// A single input-wiring row: a target input name and the FEEL reference feeding it.
type WireRow = { key: string; expr: string }
// One step draft: a chosen model + decision and the wiring of that decision's inputs.
type StepDraft = { id: string; model: string; decision: string; in: WireRow[] }
// One declared flow input: its name and optional FEEL type.
type InputDraft = { name: string; type: string }
// One output-mapping row: a result key and the FEEL reference it assembles from.
type OutputRow = { key: string; expr: string }
// The whole editable flow.
type Draft = {
  flow: string
  version: string
  inputs: InputDraft[]
  steps: StepDraft[]
  output: OutputRow[]
}

// FlowEditorMounts wires the designer into the shell: onClose leaves it, and
// onRegistered fires with the new content-addressed flowId after a successful
// register, so the shell can refresh the catalog and open the flow in the studio.
export type FlowEditorMounts = {
  host: HTMLElement
  onClose: () => void
  onRegistered: (flowId: string) => void
}

// FlowEditor is the mounted designer: create a blank flow, or edit an existing one
// prefilled from its detail.
export type FlowEditor = { create: () => void; edit: (detail: FlowDetail) => void }

// slug turns a flow name into a safe file stem for the .flow.json export.
function slug(name: string): string {
  const s = name.trim().toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '')
  return s || 'flow'
}

// blankDraft is an empty flow with one blank step to start from.
function blankDraft(): Draft {
  return { flow: '', version: '', inputs: [], steps: [{ id: 'step1', model: '', decision: '', in: [] }], output: [] }
}

// fromDetail turns a registered flow's detail into an editable draft (the "edit
// existing" path — the descriptor round-trips through the same shape the studio
// draws).
function fromDetail(d: FlowDetail): Draft {
  return {
    flow: d.name ?? '',
    version: d.version ?? '',
    inputs: (d.inputs ?? []).map((i) => ({ name: i.name, type: i.type ?? '' })),
    steps: (d.steps ?? []).map((s) => ({
      id: s.id,
      model: s.model,
      decision: s.decision,
      in: Object.entries(s.in ?? {}).map(([key, expr]) => ({ key, expr })),
    })),
    output: Object.entries(d.output ?? {}).map(([key, expr]) => ({ key, expr })),
  }
}

// toDescriptor builds the wire/disk descriptor from the draft, dropping blank rows
// and omitting empty optional sections so the JSON stays clean.
function toDescriptor(d: Draft): FlowDescriptor {
  const inputs = d.inputs
    .filter((i) => i.name.trim())
    .map((i) => (i.type.trim() ? { name: i.name.trim(), type: i.type.trim() } : { name: i.name.trim() }))
  const steps = d.steps.map((s) => {
    const inObj: Record<string, string> = {}
    for (const w of s.in) if (w.key.trim() && w.expr.trim()) inObj[w.key.trim()] = w.expr.trim()
    const step: FlowDescriptor['steps'][number] = { id: s.id.trim(), model: s.model.trim(), decision: s.decision.trim() }
    if (Object.keys(inObj).length) step.in = inObj
    return step
  })
  const output: Record<string, string> = {}
  for (const o of d.output) if (o.key.trim() && o.expr.trim()) output[o.key.trim()] = o.expr.trim()
  const desc: FlowDescriptor = { flow: d.flow.trim() || 'unbenannter-flow', steps }
  if (d.version.trim()) desc.version = d.version.trim()
  if (inputs.length) desc.inputs = inputs
  if (Object.keys(output).length) desc.output = output
  return desc
}

// toDetail adapts a draft into a FlowDetail-shaped object so the shared graph
// helpers (buildGraph/depthMap/applyIllumination/renderTrace) can draw the preview.
function toDetail(d: Draft): FlowDetail {
  const desc = toDescriptor(d)
  return { flowId: '', name: desc.flow, version: desc.version, inputs: desc.inputs, steps: desc.steps, output: desc.output }
}

export function mountFlowEditor(opts: FlowEditorMounts): FlowEditor {
  const { host, onClose, onRegistered } = opts

  host.innerHTML = `
    <div class="flow-editor-head">
      <h2 class="flow-editor-title">Flow-Designer</h2>
      <div class="flow-editor-actions">
        <button id="feTest" class="tbtn" type="button" title="Entwurf testweise auswerten (ohne zu registrieren)">▷ Testen</button>
        <button id="feCheck" class="tbtn" type="button" title="Gegen die geladenen Modelle prüfen">✓ Prüfen</button>
        <button id="feExport" class="tbtn" type="button" title="Als .flow.json herunterladen (für den Git-Commit)">⭳ Export</button>
        <button id="feRegister" class="tbtn tbtn-accent" type="button" title="Registrieren und im Flow Studio öffnen">Registrieren & Öffnen</button>
        <button id="feClose" class="tbtn" type="button" title="Designer schließen">✕</button>
      </div>
    </div>
    <div class="flow-editor-body">
      <div class="flow-editor-canvas flow-canvas" id="feCanvas"></div>
      <div class="flow-editor-inspector" id="feInspector"></div>
    </div>
    <div class="flow-editor-foot" id="feFoot"></div>`

  const canvasHost = host.querySelector<HTMLElement>('#feCanvas')!
  const inspector = host.querySelector<HTMLElement>('#feInspector')!
  const foot = host.querySelector<HTMLElement>('#feFoot')!

  let draft: Draft = blankDraft()
  let models: ModelSummary[] = []
  const modelDetails = new Map<string, ModelDetail>()
  let fc: FlowCanvas | null = null
  let laid: Laid | null = null
  let depth: Record<string, number> = {}
  let graphTimer: number | undefined

  // --- graph preview ---

  const refreshGraph = (): void => {
    const detail = toDetail(draft)
    const graph = buildGraph(detail)
    depth = depthMap(graph)
    laid = layout(graph)
    fc = renderFlowGraph(canvasHost, laid)
  }
  const scheduleGraph = (): void => {
    if (graphTimer !== undefined) clearTimeout(graphTimer)
    graphTimer = window.setTimeout(refreshGraph, 250)
  }

  // --- reference suggestions ---

  // refOptions is the set of references a wiring expression can start from: the
  // declared flow inputs and every other step's id (its outputs are addressed as
  // "stepId.<output>"). Offered as a datalist so authoring is guided but free.
  const refOptions = (exceptStep?: string): string[] => {
    const opts: string[] = []
    for (const i of draft.inputs) if (i.name.trim()) opts.push(i.name.trim())
    for (const s of draft.steps) if (s.id.trim() && s.id !== exceptStep) opts.push(s.id.trim() + '.')
    return opts
  }

  // decisionsOf returns a model's decision names (from the catalog summary — no
  // fetch needed) for the decision picker.
  const decisionsOf = (modelId: string): string[] => models.find((m) => m.modelId === modelId)?.decisions ?? []

  // suggestWiring loads the target decision's input schema and returns one wiring
  // row per input, auto-referencing a same-named flow input where one exists — the
  // helpful starting point for a fresh step.
  const suggestWiring = async (modelId: string, decision: string): Promise<WireRow[]> => {
    let detail = modelDetails.get(modelId)
    if (!detail) {
      try {
        detail = await getModel(modelId)
        modelDetails.set(modelId, detail)
      } catch {
        return []
      }
    }
    const fields: InputField[] = detail.schema?.[decision] ?? []
    const inputNames = new Set(draft.inputs.map((i) => i.name.trim()))
    return fields.map((f) => ({ key: f.name, expr: inputNames.has(f.name) ? f.name : '' }))
  }

  // --- inspector rendering ---

  const render = (): void => {
    const feelTypeOpts = FEEL_TYPES.map((t) => `<option value="${esc(t)}">${esc(t || '(kein Typ)')}</option>`).join('')
    const modelOpts = (sel: string): string => {
      const blank = `<option value=""${sel ? '' : ' selected'}>— Modell wählen —</option>`
      const rest = models
        .map((m) => `<option value="${esc(m.modelId)}"${m.modelId === sel ? ' selected' : ''}>${esc(m.name || m.modelId)}</option>`)
        .join('')
      return blank + rest
    }
    const decisionOpts = (modelId: string, sel: string): string => {
      const ds = decisionsOf(modelId)
      const blank = `<option value=""${sel ? '' : ' selected'}>— Decision wählen —</option>`
      const rest = ds.map((d) => `<option value="${esc(d)}"${d === sel ? ' selected' : ''}>${esc(d)}</option>`).join('')
      // A prefilled decision no longer in the model list is still shown so editing
      // a flow whose models are not loaded does not silently drop it.
      const orphan = sel && !ds.includes(sel) ? `<option value="${esc(sel)}" selected>${esc(sel)} (nicht geladen)</option>` : ''
      return blank + orphan + rest
    }

    const inputsHtml = draft.inputs
      .map(
        (inp, i) => `
      <div class="fe-row" data-kind="input" data-i="${i}">
        <input class="fe-in fe-name" data-i="${i}" placeholder="Input-Name" value="${esc(inp.name)}">
        <select class="fe-in fe-type" data-i="${i}">${feelTypeOpts}</select>
        <button class="icon-btn fe-del-input" data-i="${i}" type="button" title="Input entfernen">✕</button>
      </div>`,
      )
      .join('')

    const stepsHtml = draft.steps
      .map((s, si) => {
        const wires = s.in
          .map(
            (w, wi) => `
        <div class="fe-wire" data-si="${si}" data-wi="${wi}">
          <input class="fe-wire-key" data-si="${si}" data-wi="${wi}" placeholder="Ziel-Input" value="${esc(w.key)}" list="feRefsKeys${si}">
          <span class="fe-wire-arrow">←</span>
          <input class="fe-wire-expr" data-si="${si}" data-wi="${wi}" placeholder="FEEL-Referenz (z. B. Credit Score, risk.Risk Level)" value="${esc(w.expr)}" list="feRefs${si}">
          <button class="icon-btn fe-del-wire" data-si="${si}" data-wi="${wi}" type="button" title="Verdrahtung entfernen">✕</button>
        </div>`,
          )
          .join('')
        const refs = refOptions(s.id)
          .map((r) => `<option value="${esc(r)}"></option>`)
          .join('')
        return `
      <div class="fe-step" data-si="${si}">
        <div class="fe-step-head">
          <input class="fe-step-id" data-si="${si}" placeholder="Step-ID" value="${esc(s.id)}" title="Eindeutige Step-ID (kein Punkt)">
          <button class="icon-btn fe-del-step" data-si="${si}" type="button" title="Step entfernen">✕</button>
        </div>
        <label class="fe-field"><span class="fe-label">Modell</span>
          <select class="fe-step-model" data-si="${si}">${modelOpts(s.model)}</select>
        </label>
        <label class="fe-field"><span class="fe-label">Decision</span>
          <select class="fe-step-decision" data-si="${si}">${decisionOpts(s.model, s.decision)}</select>
        </label>
        <div class="fe-wires-head">
          <span class="fe-label">Input-Verdrahtung</span>
          <span class="fe-wire-actions">
            <button class="link-btn fe-suggest" data-si="${si}" type="button" title="Inputs der Decision übernehmen">↻ aus Decision</button>
            <button class="link-btn fe-add-wire" data-si="${si}" type="button">+ Verdrahtung</button>
          </span>
        </div>
        <div class="fe-wires">${wires || '<div class="fe-empty">Keine Verdrahtung — „aus Decision" übernimmt die Inputs.</div>'}</div>
        <datalist id="feRefs${si}">${refs}</datalist>
        <datalist id="feRefsKeys${si}"></datalist>
      </div>`
      })
      .join('')

    const outputHtml = draft.output
      .map(
        (o, i) => `
      <div class="fe-wire" data-kind="output" data-i="${i}">
        <input class="fe-out-key" data-i="${i}" placeholder="Ergebnis-Schlüssel" value="${esc(o.key)}">
        <span class="fe-wire-arrow">←</span>
        <input class="fe-out-expr" data-i="${i}" placeholder="FEEL-Referenz (z. B. decide.Loan Decision)" value="${esc(o.expr)}" list="feOutRefs">
        <button class="icon-btn fe-del-output" data-i="${i}" type="button" title="Ausgabe entfernen">✕</button>
      </div>`,
      )
      .join('')
    const outRefs = refOptions()
      .map((r) => `<option value="${esc(r)}"></option>`)
      .join('')

    inspector.innerHTML = `
      <section class="fe-section">
        <h3 class="fe-h">Flow</h3>
        <label class="fe-field"><span class="fe-label">Name</span>
          <input id="feFlowName" placeholder="z. B. loan-decisioning" value="${esc(draft.flow)}"></label>
        <label class="fe-field"><span class="fe-label">Version</span>
          <input id="feFlowVersion" placeholder="optional, z. B. 1" value="${esc(draft.version)}"></label>
      </section>
      <section class="fe-section">
        <div class="fe-wires-head"><h3 class="fe-h">Inputs</h3><button class="link-btn" id="feAddInput" type="button">+ Input</button></div>
        <div class="fe-inputs">${inputsHtml || '<div class="fe-empty">Keine deklarierten Inputs.</div>'}</div>
      </section>
      <section class="fe-section">
        <div class="fe-wires-head"><h3 class="fe-h">Steps</h3><button class="link-btn" id="feAddStep" type="button">+ Step</button></div>
        <div class="fe-steps">${stepsHtml}</div>
      </section>
      <section class="fe-section">
        <div class="fe-wires-head"><h3 class="fe-h">Output</h3><button class="link-btn" id="feAddOutput" type="button">+ Ausgabe</button></div>
        <div class="fe-outputs">${outputHtml || '<div class="fe-empty">Ohne Mapping = Outputs des letzten Steps.</div>'}</div>
        <datalist id="feOutRefs">${outRefs}</datalist>
      </section>`

    // Restore the FEEL-type selects (option lists cannot carry a selected value per
    // row via innerHTML alone when the value is "").
    for (const sel of inspector.querySelectorAll<HTMLSelectElement>('.fe-type')) {
      const i = Number(sel.dataset.i)
      sel.value = draft.inputs[i]?.type ?? ''
    }

    wire()
  }

  // --- event wiring (rebound after each structural render) ---

  const wire = (): void => {
    const q = <T extends HTMLElement>(s: string): NodeListOf<T> => inspector.querySelectorAll<T>(s)

    // Flow meta
    inspector.querySelector<HTMLInputElement>('#feFlowName')?.addEventListener('input', (e) => {
      draft.flow = (e.target as HTMLInputElement).value
    })
    inspector.querySelector<HTMLInputElement>('#feFlowVersion')?.addEventListener('input', (e) => {
      draft.version = (e.target as HTMLInputElement).value
    })

    // Inputs
    inspector.querySelector<HTMLButtonElement>('#feAddInput')?.addEventListener('click', () => {
      draft.inputs.push({ name: '', type: '' })
      render()
      scheduleGraph()
    })
    for (const el of q<HTMLInputElement>('.fe-name')) {
      el.addEventListener('input', () => {
        draft.inputs[Number(el.dataset.i)].name = el.value
      })
      el.addEventListener('blur', scheduleGraph)
    }
    for (const el of q<HTMLSelectElement>('.fe-type')) {
      el.addEventListener('change', () => {
        draft.inputs[Number(el.dataset.i)].type = el.value
      })
    }
    for (const el of q<HTMLButtonElement>('.fe-del-input')) {
      el.addEventListener('click', () => {
        draft.inputs.splice(Number(el.dataset.i), 1)
        render()
        scheduleGraph()
      })
    }

    // Steps
    inspector.querySelector<HTMLButtonElement>('#feAddStep')?.addEventListener('click', () => {
      draft.steps.push({ id: 'step' + (draft.steps.length + 1), model: '', decision: '', in: [] })
      render()
      scheduleGraph()
    })
    for (const el of q<HTMLInputElement>('.fe-step-id')) {
      el.addEventListener('input', () => {
        draft.steps[Number(el.dataset.si)].id = el.value
      })
      el.addEventListener('blur', () => {
        render()
        scheduleGraph()
      })
    }
    for (const el of q<HTMLButtonElement>('.fe-del-step')) {
      el.addEventListener('click', () => {
        draft.steps.splice(Number(el.dataset.si), 1)
        render()
        scheduleGraph()
      })
    }
    for (const el of q<HTMLSelectElement>('.fe-step-model')) {
      el.addEventListener('change', () => {
        const s = draft.steps[Number(el.dataset.si)]
        s.model = el.value
        s.decision = '' // model changed → decision no longer valid
        render()
        scheduleGraph()
      })
    }
    for (const el of q<HTMLSelectElement>('.fe-step-decision')) {
      el.addEventListener('change', () => {
        const s = draft.steps[Number(el.dataset.si)]
        s.decision = el.value
        // Auto-suggest wiring for a fresh step so the author starts wired.
        if (s.model && s.decision && s.in.length === 0) {
          void suggestWiring(s.model, s.decision).then((rows) => {
            if (rows.length) {
              s.in = rows
              render()
              scheduleGraph()
            }
          })
        } else {
          scheduleGraph()
        }
      })
    }
    for (const el of q<HTMLButtonElement>('.fe-suggest')) {
      el.addEventListener('click', () => {
        const s = draft.steps[Number(el.dataset.si)]
        if (!s.model || !s.decision) {
          note('Erst Modell und Decision wählen.', 'warn')
          return
        }
        void suggestWiring(s.model, s.decision).then((rows) => {
          // Merge: keep existing keys, add missing ones from the decision schema.
          const have = new Set(s.in.map((w) => w.key.trim()))
          for (const r of rows) if (!have.has(r.key)) s.in.push(r)
          render()
          scheduleGraph()
        })
      })
    }
    for (const el of q<HTMLButtonElement>('.fe-add-wire')) {
      el.addEventListener('click', () => {
        draft.steps[Number(el.dataset.si)].in.push({ key: '', expr: '' })
        render()
        scheduleGraph()
      })
    }
    for (const el of q<HTMLInputElement>('.fe-wire-key')) {
      el.addEventListener('input', () => {
        draft.steps[Number(el.dataset.si)].in[Number(el.dataset.wi)].key = el.value
      })
    }
    for (const el of q<HTMLInputElement>('.fe-wire-expr')) {
      el.addEventListener('input', () => {
        draft.steps[Number(el.dataset.si)].in[Number(el.dataset.wi)].expr = el.value
      })
      el.addEventListener('blur', scheduleGraph)
    }
    for (const el of q<HTMLButtonElement>('.fe-del-wire')) {
      el.addEventListener('click', () => {
        draft.steps[Number(el.dataset.si)].in.splice(Number(el.dataset.wi), 1)
        render()
        scheduleGraph()
      })
    }

    // Output
    inspector.querySelector<HTMLButtonElement>('#feAddOutput')?.addEventListener('click', () => {
      draft.output.push({ key: '', expr: '' })
      render()
      scheduleGraph()
    })
    for (const el of q<HTMLInputElement>('.fe-out-key')) {
      el.addEventListener('input', () => {
        draft.output[Number(el.dataset.i)].key = el.value
      })
    }
    for (const el of q<HTMLInputElement>('.fe-out-expr')) {
      el.addEventListener('input', () => {
        draft.output[Number(el.dataset.i)].expr = el.value
      })
      el.addEventListener('blur', scheduleGraph)
    }
    for (const el of q<HTMLButtonElement>('.fe-del-output')) {
      el.addEventListener('click', () => {
        draft.output.splice(Number(el.dataset.i), 1)
        render()
        scheduleGraph()
      })
    }
  }

  // --- footer (diagnostics / test output) ---

  const note = (msg: string, kind: 'ok' | 'warn' | 'error' = 'ok'): void => {
    foot.innerHTML = `<div class="fe-note fe-note-${kind}">${esc(msg)}</div>`
  }

  const showDiagnostics = (diags: { code: string; message: string; step?: string }[], okMsg: string): void => {
    if (!diags.length) {
      note(okMsg, 'ok')
      return
    }
    const rows = diags
      .map((d) => `<li><code>${esc(d.code)}</code>${d.step ? ' <span class="fe-diag-step">@' + esc(d.step) + '</span>' : ''} — ${esc(d.message)}</li>`)
      .join('')
    foot.innerHTML = `<div class="fe-note fe-note-warn">${diags.length} Diagnostic${diags.length === 1 ? '' : 's'}:<ul class="fe-diags">${rows}</ul></div>`
  }

  // --- actions ---

  const doCheck = async (): Promise<void> => {
    note('Prüfe …')
    try {
      const res = await createFlow(toDescriptor(draft))
      showDiagnostics(res.diagnostics ?? [], 'Keine Diagnostics — der Flow ist gültig gegen die geladenen Modelle. ✓')
    } catch (e) {
      note((e as Error).message, 'error')
    }
  }

  const doRegister = async (): Promise<void> => {
    note('Registriere …')
    // Cancel any pending preview refresh so it can't fire after we hand off to the
    // studio (the preview and studio canvases are independent now, but a late
    // refresh of the hidden preview would still be wasted work).
    if (graphTimer !== undefined) clearTimeout(graphTimer)
    try {
      const res = await createFlow(toDescriptor(draft))
      onRegistered(res.flowId)
    } catch (e) {
      note((e as Error).message, 'error')
    }
  }

  const doTest = async (): Promise<void> => {
    if (!laid) refreshGraph()
    const input: Record<string, unknown> = {}
    for (const inp of draft.inputs) {
      const field = testInputs.get(inp.name)
      if (!field) continue
      const v = coerce(field.value)
      if (v !== undefined) input[inp.name] = v
    }
    note('Werte Entwurf aus …')
    let res: EvalResult
    try {
      res = await evaluateFlowInline(toDescriptor(draft), input, true)
    } catch (e) {
      fc?.clear()
      note((e as Error).message, 'error')
      return
    }
    if (fc && laid) applyIllumination(fc, toDetail(draft), laid, depth, input, res)
    const rows = Object.entries(res.outputs ?? {})
      .map(([k, v]) => `<tr><td>${esc(k)}</td><td class="flow-out-val">${esc(fmt(v))}</td></tr>`)
      .join('')
    const outTable = rows ? `<table class="flow-out-table"><tbody>${rows}</tbody></table>` : 'Kein Ergebnis.'
    foot.innerHTML =
      `<div class="fe-test-out"><h3 class="fe-h">Ergebnis</h3>${outTable}</div>` +
      `<div class="flow-trace">${renderTrace(res.trace, toDetail(draft))}</div>`
  }

  // testInputs are the current draft-test input fields, rebuilt each time the test
  // form opens so it always matches the declared inputs.
  const testInputs = new Map<string, HTMLInputElement>()
  const openTestForm = (): void => {
    testInputs.clear()
    if (!draft.inputs.length) {
      foot.innerHTML = '<div class="fe-note fe-note-warn">Keine deklarierten Inputs — füge Inputs hinzu, um den Flow zu testen.</div>'
      // Still allow a no-input evaluation.
    }
    const fields = draft.inputs
      .map((i) => `<label class="eval-field-wrap"><span class="eval-field-label">${esc(i.type ? i.name + ' : ' + i.type : i.name)}</span><input class="eval-field fe-test-in" data-name="${esc(i.name)}" placeholder="${esc(i.type)}"></label>`)
      .join('')
    foot.innerHTML = `<div class="fe-test-form"><div class="fe-test-fields">${fields}</div><button id="feRunTest" class="tbtn tbtn-accent" type="button">Auswerten</button></div>`
    for (const el of foot.querySelectorAll<HTMLInputElement>('.fe-test-in')) {
      testInputs.set(el.dataset.name ?? '', el)
      attachJsonEditor(el, { title: 'JSON — ' + (el.dataset.name ?? 'Input') })
    }
    foot.querySelector<HTMLButtonElement>('#feRunTest')?.addEventListener('click', () => void doTest())
  }

  const doExport = (): void => {
    const desc = toDescriptor(draft)
    const blob = new Blob([JSON.stringify(desc, null, 2)], { type: 'application/vnd.temis.flow+json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = slug(draft.flow) + '.flow.json'
    document.body.append(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
    note('Deskriptor als ' + a.download + ' exportiert. Commit ihn nach flows/ (Git ist die dauerhafte Quelle — ADR-0032).', 'ok')
  }

  host.querySelector<HTMLButtonElement>('#feTest')?.addEventListener('click', openTestForm)
  host.querySelector<HTMLButtonElement>('#feCheck')?.addEventListener('click', () => void doCheck())
  host.querySelector<HTMLButtonElement>('#feExport')?.addEventListener('click', doExport)
  host.querySelector<HTMLButtonElement>('#feRegister')?.addEventListener('click', () => void doRegister())
  host.querySelector<HTMLButtonElement>('#feClose')?.addEventListener('click', () => {
    if (graphTimer !== undefined) clearTimeout(graphTimer)
    onClose()
  })

  // start loads the model catalog (for the pickers) then renders inspector + graph.
  const start = (d: Draft): void => {
    draft = d
    foot.innerHTML = ''
    render()
    refreshGraph()
    void (async () => {
      try {
        models = await listModels()
      } catch {
        models = []
      }
      render() // model dropdowns were empty until now
    })()
  }

  return {
    create: () => start(blankDraft()),
    edit: (detail: FlowDetail) => start(fromDetail(detail)),
  }
}
