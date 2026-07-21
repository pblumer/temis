import { APP_NAME } from './build-info'
import { listModels, getGraph, getModel, createModel, createBlankModel, renameModel, deleteModel, saveGraph, listTypes, getStatus, evaluateGraph, type ModelSummary, type Status } from './api'
import { buildInputPills, type InputPills } from './inputpills'
import { promptDialog, confirmDialog } from './dialog'
import { layout, type Orientation } from './layout'
import { renderGraph, type ModelerHandle } from './canvas'
import { renderEvaluatePanel, type EvalRun } from './evaluate'
import { mountOperate } from './operate'
import { mountClioReplay } from './clio-replay'
import { mountImport } from './testimport'
import { mountFlows } from './flows'
import { mountFlowEditor } from './flow-editor'
import type { FlowDetail, GraphEvalResult, ModelDetail } from './api'
import { openTableOverlay } from './table'
import { openLiteralOverlay } from './literal'
import { openBKMOverlay } from './bkm'
import { openBoxed, BOXED_TYPES } from './boxededitors'
import { openTypeManager } from './typemanager'
import { mountAssist } from './assist'
import { makeResizable } from './resizable'
import { FEEL_TYPES } from './feeltypes'
import './style.css'

// The modeler shell (ADR-0016): a VS-Code-style left sidebar lists the server's
// models — grouped by name, with each model's older saved revisions tucked under
// the current one as a collapsible history — and the editor (toolbar + canvas +
// evaluate panel) fills the rest. Selecting a model or a past revision loads its
// decision requirements graph, drawn by our own DMN renderers on the diagram-js
// core (no dmn-js).
async function boot(root: HTMLElement): Promise<void> {
  root.innerHTML = `
    <div class="app-shell">
      <aside class="sidebar">
        <div class="sidebar-title">${APP_NAME}</div>
        <div class="side-group side-group-flows" id="groupFlows">
          <div class="sidebar-section">
            <button class="section-title" id="flowsToggle" type="button" aria-expanded="true"><span class="section-chev">▾</span>Flows <span class="section-layer" title="Schicht L2a — komponiert Modelle">L2a</span></button>
            <span class="sidebar-actions">
              <button id="newFlow" class="icon-btn" type="button" title="Neuen Flow entwerfen"><svg width="14" height="14" viewBox="0 0 18 18"><circle cx="4.5" cy="4.5" r="2" fill="none" stroke="currentColor" stroke-width="1.3"/><circle cx="4.5" cy="13.5" r="2" fill="none" stroke="currentColor" stroke-width="1.3"/><circle cx="13.5" cy="9" r="2" fill="none" stroke="currentColor" stroke-width="1.3"/><path d="M6.5 4.5h3.5a1.5 1.5 0 0 1 1.5 1.5v1M6.5 13.5h3.5a1.5 1.5 0 0 0 1.5-1.5v-1" fill="none" stroke="currentColor" stroke-width="1.2"/></svg></button>
              <button id="flowRefresh" class="icon-btn" type="button" title="Flows neu laden">⟳</button>
            </span>
          </div>
          <div id="flowList" class="model-list flow-list"></div>
        </div>
        <div class="side-group side-group-models" id="groupModels">
          <div class="sidebar-section">
            <button class="section-title" id="modelsToggle" type="button" aria-expanded="true"><span class="section-chev">▾</span>Modelle <span class="section-layer" title="Schicht L1 — Domänen-Decisions">L1</span></button>
            <span class="sidebar-actions">
              <button id="newFolder" class="icon-btn" type="button" title="Neuer Ordner"><svg width="14" height="14" viewBox="0 0 18 18"><path d="M2 5h4l1.5 2H16v7H2z" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linejoin="round"/><path d="M9 9.5v3.5M7.25 11.25h3.5" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/></svg></button>
              <button id="newModel" class="icon-btn" type="button" title="Neues Modell anlegen (leer)"><svg width="14" height="14" viewBox="0 0 18 18"><path d="M4 2h6l4 4v10H4z" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linejoin="round"/><path d="M10 2v4h4" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linejoin="round"/><path d="M9 8.5v5M6.5 11h5" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/></svg></button>
              <button id="open" class="icon-btn" type="button" title="DMN-Datei laden (.dmn/.xml)">↑</button>
            </span>
          </div>
          <input id="file" type="file" accept=".dmn,.xml,application/xml,text/xml" hidden>
          <div class="model-search">
            <svg class="model-search-icon" width="14" height="14" viewBox="0 0 18 18" aria-hidden="true"><circle cx="7.5" cy="7.5" r="4.5" fill="none" stroke="currentColor" stroke-width="1.4"/><path d="M11 11l3.5 3.5" stroke="currentColor" stroke-width="1.4" stroke-linecap="round"/></svg>
            <input id="modelSearch" class="model-search-input" type="search" placeholder="Modelle suchen…" autocomplete="off" spellcheck="false" aria-label="Modelle suchen">
            <button id="modelSearchClear" class="model-search-clear" type="button" title="Suche zurücksetzen" hidden>✕</button>
          </div>
          <div id="modelList" class="model-list"></div>
        </div>
        <p class="sidebar-hint">
          Flows (L2a) komponieren Modelle (L1) — Modell öffnen zum Bearbeiten,
          Flow öffnen zum Ansehen & Auswerten.
        </p>
      </aside>
      <div class="resizer resizer-col" id="sidebarResizer" title="Sidebar-Breite ziehen (Doppelklick: zurücksetzen)"></div>
      <main class="editor">
        <div class="toolbar">
          <span class="mode-toggle">
            <button id="modeDesign" class="mode-btn is-active" type="button" title="Bearbeiten">Design</button>
            <button id="modeOperate" class="mode-btn" type="button" title="Auswerten & beobachten">Operate</button>
            <button id="modeImport" class="mode-btn" type="button" title="Testfälle importieren & durchlaufen lassen">Import</button>
          </span>
          <span class="design-only toolbar-group">
            <button id="undo" class="tbtn" type="button" disabled title="Rückgängig (Strg/Cmd+Z)">↶</button>
            <button id="redo" class="tbtn" type="button" disabled title="Wiederholen (Strg/Cmd+Umschalt+Z)">↷</button>
            <button id="save" class="tbtn" type="button" disabled title="Änderungen speichern (Strg/Cmd+S)">Speichern</button>
            <button id="types" class="tbtn" type="button" title="Eigene Typen verwalten">Typen</button>
          </span>
          <span class="zoom-group">
            <button id="zoomOut" class="tbtn" type="button" title="Verkleinern">−</button>
            <button id="zoomFit" class="tbtn" type="button" title="Einpassen">⤢</button>
            <button id="zoomIn" class="tbtn" type="button" title="Vergrößern">+</button>
            <button id="orient" class="tbtn design-only" type="button" title="Anordnung umschalten: Eingaben unten (Pfeile nach oben) ↔ Eingaben oben (Pfeile nach unten)">↥ Bottom-up</button>
            <button id="juice" class="tbtn" type="button" title="Effekte beim Auswerten: Datenfluss-Animation, Partikel & Combo ein-/ausschalten">⚡ Effekte</button>
          </span>
          <span id="typeEditor" class="type-editor design-only" style="display:none">
            <label for="datatype">Typ</label>
            <select id="datatype"></select>
          </span>
          <button id="assistBtn" class="tbtn" type="button" title="Modellierungs-Assistent">✦ Assistent</button>
          <span id="status" class="status"></span>
          <button id="modelIdChip" class="model-id-chip" type="button" hidden title="Modell-ID kopieren"></button>
          <span id="clioStatus" class="conn-badge" hidden><span class="conn-dot"></span><span class="conn-label"></span></span>
        </div>
        <div id="opHistory" class="op-history"></div>
        <div class="canvas-wrap">
          <div id="canvas" class="canvas"></div>
          <div id="opOverlays" class="op-overlays"></div>
        </div>
        <section class="eval-panel">
          <h2 class="eval-title">Auswerten</h2>
          <div id="eval"></div>
        </section>
        <section id="clioReplay" class="clio-replay"></section>
        <section id="importCockpit" class="import-cockpit"></section>
        <section id="flowStudio" class="flow-studio"></section>
        <section id="flowEditor" class="flow-editor"></section>
      </main>
      <aside id="assist" class="assist-panel"></aside>
    </div>`

  const appShell = root.querySelector<HTMLElement>('.app-shell')
  const modelList = root.querySelector<HTMLElement>('#modelList')
  const canvas = root.querySelector<HTMLElement>('#canvas')
  const status = root.querySelector<HTMLElement>('#status')
  const modelIdChip = root.querySelector<HTMLButtonElement>('#modelIdChip')
  const modeDesignBtn = root.querySelector<HTMLButtonElement>('#modeDesign')
  const modeOperateBtn = root.querySelector<HTMLButtonElement>('#modeOperate')
  const modeImportBtn = root.querySelector<HTMLButtonElement>('#modeImport')
  const importHost = root.querySelector<HTMLElement>('#importCockpit')
  const flowListHost = root.querySelector<HTMLElement>('#flowList')
  const flowStudioHost = root.querySelector<HTMLElement>('#flowStudio')
  const flowEditorHost = root.querySelector<HTMLElement>('#flowEditor')
  const newFlowBtn = root.querySelector<HTMLButtonElement>('#newFlow')
  const opHistoryHost = root.querySelector<HTMLElement>('#opHistory')
  const opOverlayHost = root.querySelector<HTMLElement>('#opOverlays')
  const undoBtn = root.querySelector<HTMLButtonElement>('#undo')
  const redoBtn = root.querySelector<HTMLButtonElement>('#redo')
  const saveBtn = root.querySelector<HTMLButtonElement>('#save')
  const openBtn = root.querySelector<HTMLButtonElement>('#open')
  const newModelBtn = root.querySelector<HTMLButtonElement>('#newModel')
  const newFolderBtn = root.querySelector<HTMLButtonElement>('#newFolder')
  const fileInput = root.querySelector<HTMLInputElement>('#file')
  const modelSearch = root.querySelector<HTMLInputElement>('#modelSearch')
  const modelSearchClear = root.querySelector<HTMLButtonElement>('#modelSearchClear')
  const evalHost = root.querySelector<HTMLElement>('#eval')
  const clioReplayHost = root.querySelector<HTMLElement>('#clioReplay')
  const typesBtn = root.querySelector<HTMLButtonElement>('#types')
  const typeEditor = root.querySelector<HTMLElement>('#typeEditor')
  const datatype = root.querySelector<HTMLSelectElement>('#datatype')
  if (!appShell || !modelList || !canvas || !status || !modelIdChip || !modeDesignBtn || !modeOperateBtn || !modeImportBtn || !importHost || !flowListHost || !flowStudioHost || !flowEditorHost || !newFlowBtn || !opHistoryHost || !opOverlayHost || !undoBtn || !redoBtn || !saveBtn || !openBtn || !newModelBtn || !newFolderBtn || !fileInput || !modelSearch || !modelSearchClear || !typesBtn || !evalHost || !clioReplayHost || !typeEditor || !datatype) return

  // The left sidebar sits at a fixed width by default; its divider lets the user
  // drag it wider/narrower (persisted per browser), so long model/flow names get
  // room without crowding the editor.
  const sidebar = root.querySelector<HTMLElement>('.sidebar')
  const sidebarResizer = root.querySelector<HTMLElement>('#sidebarResizer')
  if (sidebar && sidebarResizer) {
    makeResizable({
      handle: sidebarResizer,
      edge: 'left',
      initial: 264,
      min: 200,
      max: 560,
      storageKey: 'temis.modeler.sidebarWidth',
      apply: (w) => {
        sidebar.style.flex = `0 0 ${w}px`
        sidebar.style.width = `${w}px`
      },
    })
  }

  // The type options offered in the InputData/table/literal pickers: the built-in
  // FEEL types plus the current model's custom item definitions (refreshed per
  // model in show()).
  let typeOptions: string[] = FEEL_TYPES
  const renderTypeEditor = (selected?: string): void => {
    const opts = selected && !typeOptions.includes(selected) ? [...typeOptions, selected] : typeOptions
    // Build <option>s via the DOM rather than innerHTML: option text carries
    // server-supplied custom item-definition names, which must not be able to
    // inject markup (audit finding H2). textContent/value are escape-safe.
    datatype.replaceChildren(
      ...opts.map((t) => {
        const o = document.createElement('option')
        o.value = t
        o.textContent = t || '— beliebig —'
        return o
      }),
    )
    if (selected !== undefined) datatype.value = selected
  }
  renderTypeEditor()
  datatype.addEventListener('change', () => handle?.setSelectedType(datatype.value))

  let handle: ModelerHandle | null = null
  let dirty = false
  // Auto-layout orientation for the DRD: 'bottomUp' keeps leaf inputs at the
  // bottom feeding decisions upward (the default), 'topDown' flips it. Toggled
  // by the toolbar's orientation button and applied to every model shown.
  let orientation: Orientation = 'bottomUp'
  // The model currently loaded in the editor (a specific revision's id).
  let currentId = ''
  // Design (edit) vs Operate (read-only runtime view): in Operate the user runs
  // evaluations and inspects the results — decision values and the hit rule(s)
  // highlighted on the nodes and in the table — with a session history of runs.
  let mode: 'design' | 'operate' | 'import' | 'flows' | 'flow-edit' = 'design'
  let runs: EvalRun[] = []
  let activeRun: EvalRun | null = null
  // The model detail currently loaded (schema + decisions), shared with the
  // Import cockpit so it can build a matching test template and run cases.
  let currentModel: ModelDetail | null = null
  // The on-canvas input pills (Operate) and the debounce timer that turns an edit
  // into a whole-graph evaluation. Rebuilt whenever Operate opens for a model.
  let inputPills: InputPills | null = null
  let pillEvalTimer: number | undefined
  // Juice (Stage 3): the evaluation-animation switch (off under reduced motion) and
  // the combo streak — consecutive quick evaluations build a multiplier that the
  // final decision celebrates. lastRunAt gates the streak window.
  let juice = !window.matchMedia('(prefers-reduced-motion: reduce)').matches
  let runCombo = 0
  let lastRunAt = 0
  const syncButtons = (): void => {
    undoBtn.disabled = !handle?.canUndo()
    redoBtn.disabled = !handle?.canRedo()
    saveBtn.disabled = !dirty
  }
  undoBtn.addEventListener('click', () => handle?.undo())
  redoBtn.addEventListener('click', () => handle?.redo())

  // save persists the current diagram's edits, then switches to the server's new
  // revision (its content hash, hence its modelId, changed). persistGraph posts
  // the live canvas graph (moved/renamed/retyped nodes AND nodes/edges added or
  // removed, ADR-0016) and returns the saved model's id. It is a no-op returning
  // modelId unchanged when there is nothing to save.
  //
  // force skips the `dirty` fast-path and always posts the live canvas. The
  // element-must-exist flows below (create a decision's logic, open a BKM) use it:
  // they need the target element to be present server-side, and `dirty` is only a
  // best-effort hint — if it is stale (e.g. a freshly created node that never
  // flipped it, or a revision swapped in underneath), the fast-path would skip the
  // save and the follow-up create-* request would 400 for an id the server model
  // does not carry ("cannot create a table for decision … (unknown …)"). Re-posting
  // the identical canvas is cheap (content-addressed; grouped by name in the list),
  // so forcing here trades a redundant revision for a guaranteed-present element.
  const persistGraph = async (modelId: string, force = false): Promise<string> => {
    if (!handle || (!dirty && !force)) return modelId
    const { nodes, edges } = handle.graph()
    const saved = await saveGraph(modelId, {
      nodes: nodes.map((n) => ({ ...n, dataType: n.type === 'inputData' ? (n.dataType ?? '') : undefined })),
      edges,
    })
    return saved.modelId
  }

  const save = async (): Promise<void> => {
    if (!handle || !dirty || !currentId) return
    saveBtn.disabled = true
    status.textContent = 'speichert …'
    try {
      await reselect(await persistGraph(currentId))
      status.textContent = 'gespeichert ✓'
    } catch (e) {
      status.textContent = (e as Error).message
      syncButtons()
    }
  }

  // namesFor gathers the in-scope variable names for a decision's expression (the
  // other nodes' names) and the decision's own title, from the live graph.
  const namesFor = (decisionId: string): { names: string[]; title: string } => {
    const nodes = handle?.graph().nodes ?? []
    const self = nodes.find((n) => n.id === decisionId)
    const names = nodes.filter((n) => n.id !== decisionId).map((n) => n.name ?? '').filter((s) => s !== '')
    return { names, title: self?.name ?? '' }
  }
  // wiredInputsFor lists the inputs the decision is wired to in the live graph (its
  // incoming information requirements), as {expression, typeRef} column candidates.
  // The table editor uses them to surface a requirement added after the table was
  // created — otherwise that input never becomes a column and is missing from the
  // table (a table's columns are only derived from requirements at creation time).
  const wiredInputsFor = (decisionId: string): { expression: string; typeRef?: string }[] => {
    const graph = handle?.graph()
    if (!graph) return []
    const byId = new Map(graph.nodes.map((n) => [n.id, n]))
    const out: { expression: string; typeRef?: string }[] = []
    for (const e of graph.edges) {
      if (e.type !== 'informationRequirement' || e.target !== decisionId) continue
      const src = byId.get(e.source)
      const name = src?.name?.trim()
      if (name) out.push({ expression: name, typeRef: src?.dataType })
    }
    return out
  }
  const openLiteral = (modelId: string, decisionId: string, fresh = false): void => {
    const { names, title } = namesFor(decisionId)
    void openLiteralOverlay(modelId, decisionId, title, names, (newId) => void reselect(newId), { fresh, typeOptions, readOnly: mode === 'operate' && !fresh })
  }

  // openLogic opens the editor for a decision's boxed logic of the given kind
  // (WP-142, one entry point for all kinds). table and literal keep their special
  // openers (mode-aware trace, fresh flag); every other boxed kind goes through the
  // shared openBoxed dispatch, anchored at the decision — editable in Design,
  // read-only in Operate.
  const openLogic = (kind: string, modelId: string, decisionId: string): void => {
    if (kind === 'literal') {
      openLiteral(modelId, decisionId)
      return
    }
    if (kind === 'table') {
      openTable(modelId, decisionId)
      return
    }
    const { names } = namesFor(decisionId)
    openBoxed(kind, { modelId, anchor: { kind: 'decision', id: decisionId }, names, onSaved: (newId) => void reselect(newId), typeOptions, readOnly: mode === 'operate' })
  }

  // createLogic gives an undecided decision a fresh boxed logic of the given kind
  // (WP-142): persist pending structural edits first (so the decision exists
  // server-side), create it via the registry endpoint, switch to the saved
  // revision and open it. Literal has no create endpoint — it is materialized on
  // save via createLiteral.
  const createLogic = async (kind: string, decisionId: string): Promise<void> => {
    if (!currentId) return
    if (kind === 'literal') {
      await createLiteral(decisionId)
      return
    }
    const bt = BOXED_TYPES.find((b) => b.kind === kind)
    if (!bt?.create) return
    status.textContent = bt.statusCreating
    try {
      const created = await bt.create(await persistGraph(currentId, true), decisionId)
      await reselect(created.modelId)
      status.textContent = bt.statusCreated
      openLogic(kind, created.modelId, decisionId)
    } catch (e) {
      status.textContent = (e as Error).message
    }
  }

  // openBKM edits a business knowledge model's encapsulated function. A freshly
  // dropped BKM lives only in the live graph, so persist any pending structural
  // edits first (mirroring the create-* handlers) and switch to the saved
  // revision — otherwise GET .../bkm/{id} 404s and the overlay can't open.
  const openBKM = async (bkmId: string): Promise<void> => {
    if (!currentId) return
    try {
      const savedId = await persistGraph(currentId, true)
      if (savedId !== currentId) await reselect(savedId)
      void openBKMOverlay(savedId, bkmId, (newId) => void reselect(newId), typeOptions)
    } catch (e) {
      status.textContent = (e as Error).message
    }
  }

  // Typen: open the custom-type manager; each save/delete switches to the saved
  // revision (which refreshes typeOptions via show()).
  typesBtn.addEventListener('click', () => {
    if (currentId) void openTypeManager(currentId, (newId) => reselect(newId))
  })

  // Zoom controls.
  root.querySelector('#zoomOut')?.addEventListener('click', () => handle?.zoom('out'))
  root.querySelector('#zoomFit')?.addEventListener('click', () => handle?.zoom('fit'))
  root.querySelector('#zoomIn')?.addEventListener('click', () => handle?.zoom('in'))

  // Orientation toggle: flip whether inputs feed decisions from below (bottom-up)
  // or from above (top-down), re-arrange the live diagram, and remember the
  // choice for the next model shown. Re-arranging changes node positions, so it
  // marks the model dirty (savable). The label shows the current orientation.
  const orientBtn = root.querySelector<HTMLButtonElement>('#orient')
  const syncOrientBtn = (): void => {
    if (orientBtn) orientBtn.textContent = orientation === 'bottomUp' ? '↥ Bottom-up' : '↧ Top-down'
  }
  syncOrientBtn()
  orientBtn?.addEventListener('click', () => {
    orientation = orientation === 'bottomUp' ? 'topDown' : 'bottomUp'
    syncOrientBtn()
    if (handle) {
      handle.arrange(orientation)
      dirty = true
      syncButtons()
    }
  })

  // The juice toggle turns the evaluation animation (dataflow, particles, combo) on
  // or off. It starts off under reduced-motion, so the button reflects that.
  const juiceBtn = root.querySelector<HTMLButtonElement>('#juice')
  const syncJuiceBtn = (): void => {
    juiceBtn?.classList.toggle('juice-off', !juice)
    if (juiceBtn) juiceBtn.textContent = juice ? '⚡ Effekte' : '⚡ Effekte aus'
  }
  syncJuiceBtn()
  juiceBtn?.addEventListener('click', () => {
    juice = !juice
    syncJuiceBtn()
  })
  // createLiteral persists pending structural edits (so the decision exists), then
  // opens an empty literal editor for it; saving creates the expression.
  const createLiteral = async (decisionId: string): Promise<void> => {
    if (!currentId) return
    status.textContent = 'legt Ausdruck an …'
    try {
      const baseId = await persistGraph(currentId, true)
      await reselect(baseId)
      status.textContent = ''
      openLiteral(baseId, decisionId, true)
    } catch (e) {
      status.textContent = (e as Error).message
    }
  }
  saveBtn.addEventListener('click', () => void save())

  // reselect refreshes the model list and switches to modelId (e.g. after a save
  // or an upload created/changed a cached model).
  const reselect = async (modelId: string): Promise<void> => {
    models = await listModels()
    await showModel(models.some((m) => m.modelId === modelId) ? modelId : (models[0]?.modelId ?? ''))
  }

  // Modeling assistant (ADR-0024): a docked chat where an LLM drives temis's
  // tools to help build decisions. It gets the open model's id as context and,
  // when it creates or changes a model, we reload that revision.
  const assistHost = root.querySelector<HTMLElement>('#assist')
  const assistBtn = root.querySelector<HTMLButtonElement>('#assistBtn')
  if (assistHost && assistBtn) {
    const assist = mountAssist(assistHost, {
      currentModelId: () => currentId,
      onModelChanged: (id) => void reselect(id),
    })
    assistBtn.addEventListener('click', () => assist.toggle())
  }

  // clio connection indicator (ADR-0030): a small toolbar badge that shows, at a
  // glance, whether the tamper-evident decision log (clio) is reachable. It polls
  // GET /v1/status; the badge never shows a secret. Green = reachable, red =
  // configured but unreachable, grey = not configured (or hidden behind the audit
  // scope). Absent endpoint (older server) simply hides the badge.
  const clioBadge = root.querySelector<HTMLElement>('#clioStatus')
  const clioDot = clioBadge?.querySelector<HTMLElement>('.conn-dot')
  const clioLabel = clioBadge?.querySelector<HTMLElement>('.conn-label')
  const renderClioStatus = (st: Status | null): void => {
    if (!clioBadge || !clioDot || !clioLabel) return
    clioBadge.classList.remove('conn-ok', 'conn-bad', 'conn-off')
    if (!st) {
      // No /v1/status endpoint (older server) or a network error: hide the badge
      // rather than assert anything about clio.
      clioBadge.hidden = true
      return
    }
    clioBadge.hidden = false
    const c = st.clio
    if (st.gated) {
      clioBadge.classList.add('conn-off')
      clioLabel.textContent = 'clio ?'
      clioBadge.title = 'clio-Status ist audit-/admin-geschützt — mit einem Key mit dem Scope „audit" sichtbar.'
      return
    }
    if (!c.enabled) {
      clioBadge.classList.add('conn-off')
      clioLabel.textContent = 'clio aus'
      clioBadge.title = 'Kein clio-Audit-Sink konfiguriert. Anschalten: TEMIS_CLIO_TOKEN setzen (oder -clio-url auf die eigene clio).'
      return
    }
    const where = c.url ? ' — ' + c.url : ''
    const counts = `ok ${c.writesOk ?? 0}, Fehler ${c.writesFailed ?? 0}, idempotent ${c.idempotentSkips ?? 0}`
    if (c.reachable) {
      clioBadge.classList.add('conn-ok')
      clioLabel.textContent = 'clio verbunden'
      clioBadge.title = `clio erreichbar${where} (${c.mode ?? 'best-effort'}). Writes: ${counts}.`
    } else {
      clioBadge.classList.add('conn-bad')
      clioLabel.textContent = 'clio getrennt'
      const why = c.lastError ? '\nLetzter Fehler: ' + c.lastError : ''
      clioBadge.title = `clio nicht erreichbar${where} (${c.mode ?? 'best-effort'}). Writes: ${counts}.${why}`
    }
  }
  const refreshClioStatus = async (): Promise<void> => {
    try {
      renderClioStatus(await getStatus())
    } catch {
      renderClioStatus(null)
    }
  }
  void refreshClioStatus()
  window.setInterval(() => void refreshClioStatus(), 20000)
  // Refresh promptly when the operator returns to the tab, so a clio outage that
  // happened while the tab was hidden shows up without waiting for the next poll.
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible') void refreshClioStatus()
  })

  // Neues Modell… scaffolds an empty decision model server-side and switches to
  // it, so a user can build a decision from scratch on a blank canvas (via the
  // palette + save) instead of only uploading an existing .dmn file (ADR-0016).
  newModelBtn.addEventListener('click', () => {
    void (async () => {
      const name = await promptDialog({
        title: 'Neues Modell',
        label: 'Name des Modells',
        value: 'Neues Modell',
        okLabel: 'Anlegen',
        hint: dupHint(),
      })
      if (!name) return
      status.textContent = 'legt Modell an …'
      try {
        const m = await createBlankModel(name)
        await reselect(m.modelId)
        status.textContent = 'Modell angelegt ✓ — Elemente über die Palette (links) hinzufügen und speichern.'
      } catch (e) {
        status.textContent = (e as Error).message
      }
    })()
  })

  // Open… deploys a chosen .dmn/.xml file to the engine and switches to it.
  openBtn.addEventListener('click', () => fileInput.click())
  fileInput.addEventListener('change', () => {
    const file = fileInput.files?.[0]
    if (!file) return
    status.textContent = 'lädt Datei …'
    void file
      .text()
      .then((xml) => createModel(xml))
      .then((m) => reselect(m.modelId))
      .then(() => {
        status.textContent = 'geladen ✓'
      })
      .catch((e: Error) => {
        status.textContent = e.message
      })
      .finally(() => {
        fileInput.value = '' // allow re-loading the same file
      })
  })
  document.addEventListener('keydown', (e) => {
    if (!(e.ctrlKey || e.metaKey)) return
    const k = e.key.toLowerCase()
    if (k === 's') {
      e.preventDefault()
      void save()
    } else if (k === 'z' && !e.shiftKey) {
      e.preventDefault()
      handle?.undo()
    } else if ((k === 'z' && e.shiftKey) || k === 'y') {
      e.preventDefault()
      handle?.redo()
    }
  })

  // expanded holds the group names whose revision history is unfolded, kept across
  // re-renders so a save (which rebuilds the list) doesn't collapse the view.
  const expanded = new Set<string>()

  // Live text filter over the model list. The more models on the server, the more
  // the search earns its keep — so it is diacritic-insensitive and term-based:
  // whitespace splits the query into terms that must ALL appear (in any order), so
  // "alter demo" finds "Alterskette (Demo)". Matching runs over the model name and,
  // for a filed model, its folder name, so a folder's name pulls up its contents.
  let modelQuery = ''
  const foldText = (s: string): string =>
    s
      .toLowerCase()
      .normalize('NFD')
      .replace(/[\u0300-\u036f]/g, '')
  const queryTerms = (): string[] => foldText(modelQuery).split(/\s+/).filter(Boolean)
  const matchesTerms = (haystack: string, terms: string[]): boolean => {
    const h = foldText(haystack)
    return terms.every((t) => h.includes(t))
  }

  let models: ModelSummary[] = []
  try {
    models = await listModels()
  } catch (e) {
    status.textContent = (e as Error).message
    return
  }
  // Note: an empty server is NOT an early return — boot continues so every action
  // (new model/flow/folder, search, flows catalog) is wired. renderModelList
  // renders the "no models" empty state, and the initial selection below is
  // guarded for the empty case (H3).

  // groupModels buckets revisions by display name and orders each bucket
  // newest-first (highest seq). Unnamed models each form their own bucket.
  type Group = { name: string; revisions: ModelSummary[] }
  const groupModels = (): Group[] => {
    const byName = new Map<string, ModelSummary[]>()
    for (const m of models) {
      const key = m.name || '(' + m.modelId.slice(7, 15) + ')'
      const list = byName.get(key)
      if (list) list.push(m)
      else byName.set(key, [m])
    }
    const groups: Group[] = []
    for (const [name, revisions] of byName) {
      revisions.sort((a, b) => (b.seq ?? 0) - (a.seq ?? 0))
      groups.push({ name, revisions })
    }
    groups.sort((a, b) => a.name.localeCompare(b.name))
    return groups
  }

  const el = (tag: string, cls: string, ...kids: (string | Node)[]): HTMLElement => {
    const n = document.createElement(tag)
    if (cls) n.className = cls
    n.append(...kids)
    return n
  }

  // Folders organise the model list. A model is filed by NAME (its stable
  // identity across revisions), and the assignment is persisted in the browser
  // (localStorage) — per browser, since the server's model cache is content-
  // addressed and ephemeral. Drag a model onto a folder to file it; drop it on
  // empty space to unfile it.
  const FOLDERS_KEY = 'temis.modeler.folders'
  type FolderState = { folders: string[]; assign: Record<string, string> }
  const loadFolders = (): FolderState => {
    try {
      const s = JSON.parse(localStorage.getItem(FOLDERS_KEY) ?? '') as FolderState
      if (Array.isArray(s.folders) && s.assign && typeof s.assign === 'object') return { folders: s.folders.filter((f) => typeof f === 'string'), assign: s.assign }
    } catch {
      /* no/invalid stored folders */
    }
    return { folders: [], assign: {} }
  }
  const folderState = loadFolders()
  const collapsedFolders = new Set<string>()
  const saveFolders = (): void => {
    try {
      localStorage.setItem(FOLDERS_KEY, JSON.stringify(folderState))
    } catch {
      /* storage unavailable (private mode) — folders just won't persist */
    }
  }
  const assignModel = (name: string, folder: string | null): void => {
    if (folder) folderState.assign[name] = folder
    else delete folderState.assign[name]
    saveFolders()
    renderModelList()
  }
  const createFolder = (): void => {
    void (async () => {
      const name = await promptDialog({
        title: 'Neuer Ordner',
        label: 'Name des Ordners',
        placeholder: 'z. B. Kunde A',
        okLabel: 'Anlegen',
        hint: (v) => (v && folderState.folders.includes(v) ? 'Ein Ordner mit diesem Namen existiert bereits.' : null),
      })
      if (!name || folderState.folders.includes(name)) return
      folderState.folders.push(name)
      saveFolders()
      renderModelList()
    })()
  }
  const deleteFolder = (name: string): void => {
    folderState.folders = folderState.folders.filter((f) => f !== name)
    for (const k of Object.keys(folderState.assign)) if (folderState.assign[k] === name) delete folderState.assign[k]
    saveFolders()
    renderModelList()
  }
  newFolderBtn.addEventListener('click', createFolder)

  // Wire the live filter: typing re-renders the list, and the clear button (and
  // Escape) empties it. renderModelList reads modelQuery on every pass.
  const applyQuery = (v: string): void => {
    modelQuery = v
    modelSearchClear.hidden = v.trim() === ''
    renderModelList()
  }
  modelSearch.addEventListener('input', () => applyQuery(modelSearch.value))
  modelSearch.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && modelSearch.value) {
      e.stopPropagation()
      modelSearch.value = ''
      applyQuery('')
    }
  })
  modelSearchClear.addEventListener('click', () => {
    modelSearch.value = ''
    applyQuery('')
    modelSearch.focus()
  })
  // Dropping a model on the list background (not on a folder) unfiles it.
  modelList.addEventListener('dragover', (e) => e.preventDefault())
  modelList.addEventListener('drop', (e) => {
    const name = e.dataTransfer?.getData('text/plain')
    if (name) assignModel(name, null)
  })

  // existingNames is the set of distinct non-empty model names on the server. A
  // dupHint(exclude) warns — without blocking — when a new or renamed model would
  // land on a name already in use (the two would merge into one history group).
  const existingNames = (): Set<string> => new Set(models.map((m) => m.name ?? '').filter((s) => s !== ''))
  const dupHint =
    (exclude?: string) =>
    (v: string): string | null =>
      v && v !== exclude && existingNames().has(v) ? 'Ein Modell mit diesem Namen existiert bereits — die beiden werden zusammengeführt.' : null

  // renameGroup renames every revision of a named model so its whole history
  // stays together under the new name: it re-stores each revision (oldest-first,
  // to keep the seq order) under the new name, drops the old-named revisions,
  // carries the folder assignment over and selects the renamed current revision.
  const renameGroup = async (group: Group, newName: string): Promise<void> => {
    const current = group.revisions[0]
    const ordered = group.revisions.slice().sort((a, b) => (a.seq ?? 0) - (b.seq ?? 0))
    status.textContent = 'benennt um …'
    try {
      let newCurrentId = current.modelId
      for (const rev of ordered) {
        const saved = await renameModel(rev.modelId, newName)
        if (rev.modelId === current.modelId) newCurrentId = saved.modelId
        if (saved.modelId !== rev.modelId) await deleteModel(rev.modelId)
      }
      const folder = folderState.assign[group.name]
      if (folder) {
        delete folderState.assign[group.name]
        folderState.assign[newName] = folder
        saveFolders()
      }
      await reselect(newCurrentId)
      status.textContent = 'umbenannt ✓'
    } catch (e) {
      status.textContent = (e as Error).message
    }
  }

  // deleteGroup removes a whole named model — every revision — from the server
  // cache, drops its folder assignment and selects another model (or shows the
  // empty state when none remain).
  const deleteGroup = async (group: Group): Promise<void> => {
    status.textContent = 'löscht …'
    try {
      for (const rev of group.revisions) await deleteModel(rev.modelId)
      if (folderState.assign[group.name]) {
        delete folderState.assign[group.name]
        saveFolders()
      }
      models = await listModels()
      if (models.length) {
        await showModel(models.some((m) => m.modelId === currentId) ? currentId : models[0].modelId)
        status.textContent = 'gelöscht ✓'
      } else {
        currentId = ''
        modelList.innerHTML = '<p class="model-empty">Keine Modelle auf dem Server.</p>'
        status.textContent = 'gelöscht ✓'
      }
    } catch (e) {
      status.textContent = (e as Error).message
    }
  }

  // Sidebar row-action icons (rename/delete), shown on hover of a model row.
  const ICON_RENAME =
    '<svg width="13" height="13" viewBox="0 0 18 18"><path d="M3 12.9 12 3.9l2.1 2.1-9 9H3z" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linejoin="round"/><path d="M10.8 6.1 12 7.3" stroke="currentColor" stroke-width="1.3" stroke-linecap="round"/></svg>'
  const ICON_DELETE =
    '<svg width="13" height="13" viewBox="0 0 18 18"><path d="M4 5h10M7 5V3.6h4V5M5.6 5l.6 9.4h5.6L12.4 5" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"/></svg>'

  // highlightName renders a model name into a .model-name span, wrapping the parts
  // that match the active search terms in <mark> so the reason a row is shown is
  // obvious. With no query it is a plain text span. Highlighting is case-insensitive
  // over the raw text (matching itself, in matchesTerms, is also diacritic-folded).
  const highlightName = (name: string, terms: string[]): HTMLElement => {
    const span = el('span', 'model-name')
    if (!terms.length) {
      span.textContent = name
      return span
    }
    const lower = name.toLowerCase()
    const marks: Array<[number, number]> = []
    for (const t of terms) {
      let from = 0
      for (;;) {
        const i = lower.indexOf(t, from)
        if (i < 0) break
        marks.push([i, i + t.length])
        from = i + t.length
      }
    }
    if (!marks.length) {
      span.textContent = name
      return span
    }
    marks.sort((a, b) => a[0] - b[0])
    let cursor = 0
    for (const [start, end] of marks) {
      if (end <= cursor) continue
      const s = Math.max(start, cursor)
      if (s > cursor) span.append(name.slice(cursor, s))
      span.append(el('mark', 'model-name-hit', name.slice(s, end)))
      cursor = end
    }
    if (cursor < name.length) span.append(name.slice(cursor))
    return span
  }

  // renderGroup draws one model (its current revision + a collapsible history of
  // older ones) into container. The current row is draggable so it can be dropped
  // onto a folder (drag by model name — the stable identity across revisions).
  const renderGroup = (group: Group, container: HTMLElement, terms: string[] = []): void => {
    const current = group.revisions[0]
    const older = group.revisions.slice(1)
    const total = group.revisions.length
    if (older.some((m) => m.modelId === currentId)) expanded.add(group.name)

    const row = el('div', 'model-item' + (current.modelId === currentId ? ' is-current' : ''))
    row.append(highlightName(group.name, terms))
    if (total > 1) row.append(el('span', 'model-rev', 'v' + total))

    // Per-model actions (rename / delete the whole named model incl. history),
    // revealed on row hover. stopPropagation keeps a click off the row's select.
    const actions = el('span', 'model-actions')
    const renameBtn = el('button', 'model-act') as HTMLButtonElement
    renameBtn.type = 'button'
    renameBtn.title = 'Modell umbenennen'
    renameBtn.innerHTML = ICON_RENAME
    renameBtn.addEventListener('click', (e) => {
      e.stopPropagation()
      void (async () => {
        const cur = current.name ?? ''
        const newName = await promptDialog({
          title: 'Modell umbenennen',
          label: 'Neuer Name',
          value: cur,
          okLabel: 'Umbenennen',
          hint: dupHint(cur),
        })
        if (newName && newName !== cur) await renameGroup(group, newName)
      })()
    })
    const delBtn = el('button', 'model-act model-act-del') as HTMLButtonElement
    delBtn.type = 'button'
    delBtn.title = 'Modell löschen'
    delBtn.innerHTML = ICON_DELETE
    delBtn.addEventListener('click', (e) => {
      e.stopPropagation()
      void (async () => {
        const ok = await confirmDialog({
          title: 'Modell löschen',
          message: total > 1 ? `„${group.name}" und den gesamten Verlauf (${total} Revisionen) unwiderruflich löschen?` : `„${group.name}" unwiderruflich löschen?`,
          okLabel: 'Löschen',
          danger: true,
        })
        if (ok) await deleteGroup(group)
      })()
    })
    actions.append(renameBtn, delBtn)
    row.append(actions)

    row.draggable = true
    row.addEventListener('dragstart', (e) => e.dataTransfer?.setData('text/plain', group.name))
    row.addEventListener('click', () => void showModel(current.modelId))
    container.append(row)

    if (older.length) {
      const open = expanded.has(group.name)
      const toggle = el('button', 'model-history-toggle', (open ? '▾ ' : '▸ ') + 'Verlauf (' + older.length + ')')
      toggle.addEventListener('click', () => {
        if (expanded.has(group.name)) expanded.delete(group.name)
        else expanded.add(group.name)
        renderModelList()
      })
      container.append(toggle)
      if (open) {
        older.forEach((rev, i) => {
          const v = total - 1 - i
          const hrow = el('div', 'model-history-item' + (rev.modelId === currentId ? ' is-current' : ''))
          hrow.append(el('span', 'model-rev', 'v' + v), el('span', 'model-hist-id', rev.modelId.slice(-6)))
          hrow.addEventListener('click', () => void showModel(rev.modelId))
          container.append(hrow)
        })
      }
    }
  }

  const renderModelList = (): void => {
    modelList.textContent = ''
    const terms = queryTerms()
    const searching = terms.length > 0
    const groups = groupModels()
    const known = new Set(folderState.folders)
    const byFolder = new Map<string, Group[]>()
    const unassigned: Group[] = []
    for (const g of groups) {
      const f = folderState.assign[g.name]
      if (f && known.has(f)) {
        const list = byFolder.get(f) ?? []
        list.push(g)
        byFolder.set(f, list)
      } else {
        unassigned.push(g)
      }
    }

    // While searching, keep only matching groups. A model matches on its own name;
    // a filed model also matches on its folder name (so a folder name surfaces its
    // whole contents). Folders with no match are hidden and matches are force-open.
    let shown = 0
    const keep = (g: Group, folder?: string): boolean => (searching ? matchesTerms(folder ? g.name + ' ' + folder : g.name, terms) : true)

    for (const folder of folderState.folders) {
      const allMembers = byFolder.get(folder) ?? []
      const members = allMembers.filter((g) => keep(g, folder))
      if (searching && !members.length) continue
      const open = searching || !collapsedFolders.has(folder)
      const head = el('div', 'folder-head')
      const count = searching ? `${members.length}/${allMembers.length}` : String(allMembers.length)
      head.append(el('span', 'folder-twisty', open ? '▾' : '▸'), el('span', 'folder-name', folder), el('span', 'folder-count', count))
      const del = el('button', 'folder-del', '✕')
      del.title = 'Ordner löschen (Modelle bleiben erhalten)'
      del.addEventListener('click', (e) => {
        e.stopPropagation()
        deleteFolder(folder)
      })
      head.append(del)
      head.addEventListener('click', () => {
        if (collapsedFolders.has(folder)) collapsedFolders.delete(folder)
        else collapsedFolders.add(folder)
        renderModelList()
      })
      // Drop a dragged model onto the folder to file it here.
      head.addEventListener('dragover', (e) => {
        e.preventDefault()
        head.classList.add('drop-over')
      })
      head.addEventListener('dragleave', () => head.classList.remove('drop-over'))
      head.addEventListener('drop', (e) => {
        e.preventDefault()
        e.stopPropagation()
        head.classList.remove('drop-over')
        const name = e.dataTransfer?.getData('text/plain')
        if (name) assignModel(name, folder)
      })
      modelList.append(head)

      if (open) {
        const body = el('div', 'folder-body')
        for (const g of members) {
          renderGroup(g, body, terms)
          shown++
        }
        if (!members.length) body.append(el('p', 'folder-empty', 'leer — Modelle hierher ziehen'))
        modelList.append(body)
      } else {
        shown += members.length
      }
    }

    for (const g of unassigned) {
      if (!keep(g)) continue
      renderGroup(g, modelList, terms)
      shown++
    }

    if (searching && shown === 0) {
      modelList.append(el('p', 'model-empty', `Keine Modelle für „${modelQuery.trim()}".`))
    } else if (!searching && shown === 0) {
      // Fresh server, no models yet. Render the empty state here rather than
      // bailing out of boot() early, so the rest of the shell (new-model / new-flow
      // / new-folder actions, search, flows catalog) still gets wired (H3).
      modelList.append(el('p', 'model-empty', 'Keine Modelle auf dem Server.'))
    }
  }

  // hitRulesOf maps each decision to the rule numbers (1-based) that fired, from
  // the run's per-decision traces — for the on-node hit-rule badges.
  const hitRulesOf = (result: GraphEvalResult): Record<string, number[]> => {
    const out: Record<string, number[]> = {}
    for (const [name, tr] of Object.entries(result.traces ?? {})) {
      const rules: number[] = []
      for (const t of tr.tables ?? []) for (const m of t.matched ?? []) rules.push(m + 1)
      if (rules.length) out[name] = rules
    }
    return out
  }

  // applyRun makes a run the active one and overlays its values + hit rules on
  // the diagram nodes (the green result pills).
  const applyRun = (run: EvalRun, opts?: { animate?: boolean; combo?: number }): void => {
    activeRun = run
    handle?.showResults(run.result.values, hitRulesOf(run.result))
    // Light up the requirement edges with the values that travelled them, so the
    // dependency dataflow is visible on the diagram itself — not just in the panel.
    // A fresh evaluation animates the wave (opts); history navigation shows it calm.
    handle?.illuminate(run.inputs, run.result.values, opts)
  }

  // The Operate cockpit: a keyboard-navigable run history above the diagram and
  // frosted summary overlays over it (operate.ts). It reads the live session
  // state via getters; selecting a run makes it active, repaints the diagram
  // pills and refreshes the operate chrome.
  const operate = mountOperate({
    historyHost: opHistoryHost,
    overlayHost: opOverlayHost,
    getRuns: () => runs,
    getActive: () => activeRun,
    onActivate: (run) => {
      applyRun(run)
      // Reflect the picked run's inputs in the on-node pills, so the diagram's
      // editable inputs match the run whose results it is showing.
      inputPills?.setValues(run.inputs)
      operate.render()
    },
  })

  // The clio replay panel (ADR-0033 read side): the Operate-view counterpart to
  // the "Auswerten" form — it reads decisions temis already filed in clio (under
  // a user-defined subject + event-type mapping) and replays each recorded input
  // through the open model, recording the outcome as a normal Operate run.
  const clioReplay = mountClioReplay({
    host: clioReplayHost,
    getModel: () => currentModel,
    onReplay: (run) => recordRun(run),
  })

  // The Import cockpit: a batch test-runner shaped like a conveyor belt. It reads
  // the loaded model via getModel to build a matching CSV/JSON template and runs
  // imported test cases against the same whole-graph evaluate endpoint, animating
  // each record from the Eingang lane through Evaluation into the clio Store.
  const importView = mountImport({
    host: importHost,
    getModel: () => currentModel,
  })

  // The Flows view (WP-97): a catalog of registered decision flows in the sidebar
  // and a studio (graph + run panel) in the editor area. It is self-contained —
  // it fetches flows over /v1/flows and evaluates them independently of the model
  // the modeler has open.
  const flowView = mountFlows({
    catalogHost: flowListHost,
    studioHost: flowStudioHost,
    onOpenFlow: () => setMode('flows'),
    onEditFlow: (detail: FlowDetail) => {
      // Show the designer first, so its canvas host is visible (non-zero size)
      // when the live-preview diagram renders and fits — a hidden container cannot
      // be fit (same ordering the studio uses when opening a flow).
      setMode('flow-edit')
      flowEditor.edit(detail)
    },
  })

  // The Flow Designer (WP-116): create/design a flow visually (structured inspector
  // + live graph preview + inline test). Registering (POST /v1/flows) is ephemeral
  // and content-addressed; Git stays the durable source of truth (ADR-0032), so the
  // designer also exports the descriptor as a *.flow.json artifact. onRegistered
  // refreshes the catalog and opens the new flow in the studio.
  const flowEditor = mountFlowEditor({
    host: flowEditorHost,
    onClose: () => setMode(currentId ? 'design' : 'flows'),
    onRegistered: (flowId: string) => {
      flowView.render()
      flowView.open(flowId)
    },
  })
  newFlowBtn.addEventListener('click', () => {
    // Show the designer first (see onEditFlow) so the preview canvas can fit.
    setMode('flow-edit')
    flowEditor.create()
  })

  // recordRun is called after each evaluation: keep it in the session history
  // (newest first), highlight it, and refresh the Operate cockpit.
  const recordRun = (run: EvalRun): void => {
    runs.unshift(run)
    // A run within the streak window bumps the combo; a longer pause resets it.
    const now = performance.now()
    runCombo = now - lastRunAt < 2600 ? runCombo + 1 : 1
    lastRunAt = now
    applyRun(run, { animate: juice, combo: runCombo })
    if (mode === 'operate') operate.render()
  }

  // runFromPills turns the current on-node input values into a live whole-graph
  // evaluation, debounced so typing doesn't fire a request per keystroke. A half-
  // typed invalid value just fails quietly; the next edit retries. Each successful
  // run flows through recordRun, so the result pills and edge illumination update.
  const runFromPills = (): void => {
    if (!currentId || !inputPills) return
    const inputs = inputPills.collect()
    const id = currentId
    window.clearTimeout(pillEvalTimer)
    pillEvalTimer = window.setTimeout(() => {
      void evaluateGraph(id, inputs, true, true)
        .then((res) => recordRun({ inputs, result: res }))
        .catch(() => {})
    }, 400)
  }

  // mountInputPills builds an editable pill for each leaf input, mounted on its
  // inputData node, so the whole graph's inputs can be filled on the diagram
  // itself (Operate) instead of only in the side panel. Prefills from the active
  // run. Without a loaded schema/handle it simply shows no pills.
  const mountInputPills = (): void => {
    if (!handle || !currentModel) return
    const nodeIdByName = new Map<string, string>()
    for (const n of handle.graph().nodes) if (n.type === 'inputData' && n.name) nodeIdByName.set(n.name, n.id)
    inputPills = buildInputPills(currentModel, nodeIdByName, runFromPills)
    if (activeRun) inputPills.setValues(activeRun.inputs)
    handle.showInputPills(inputPills.items)
  }

  // openTable opens a decision's table — editable in Design, read-only with the
  // active run's hit rule(s) highlighted in Operate.
  const openTable = (modelId: string, decisionId: string): void => {
    if (mode === 'operate') {
      const name = handle?.graph().nodes.find((n) => n.id === decisionId)?.name ?? ''
      const tr = activeRun?.result.traces?.[name]
      const matched: number[] = []
      for (const t of tr?.tables ?? []) for (const m of t.matched ?? []) matched.push(m)
      // The first table's trace drives the decision-path view (a decision table
      // decision has exactly one table; matched still spans all, for safety).
      void openTableOverlay(modelId, decisionId, undefined, typeOptions, { readOnly: true, matched, trace: tr?.tables?.[0] })
    } else {
      // Pass the decision's wired inputs (so requirements added after the table
      // was created surface as columns) and its in-scope variables (so input-column
      // expressions can reference and complete them — otherwise a reference like
      // `Name` reads as an unknown variable).
      const { names } = namesFor(decisionId)
      void openTableOverlay(modelId, decisionId, (newId) => void reselect(newId), typeOptions, { wiredInputs: wiredInputsFor(decisionId), scope: names })
    }
  }

  const setMode = (m: 'design' | 'operate' | 'import' | 'flows' | 'flow-edit'): void => {
    mode = m
    appShell.dataset.mode = m
    // Design/Operate/Import are activities on the open model (L1); Flows (view) and
    // flow-edit (the designer) are entered from the FLOWS sidebar section, so they
    // have no toolbar tab of their own.
    modeDesignBtn.classList.toggle('is-active', m === 'design')
    modeOperateBtn.classList.toggle('is-active', m === 'operate')
    modeImportBtn.classList.toggle('is-active', m === 'import')
    // The model-id chip belongs to the open L1 model; flows have no single model.
    if (m === 'flows' || m === 'flow-edit') modelIdChip.hidden = true
    if (m === 'operate') {
      operate.render()
      clioReplay.render()
      // Fill the leaf inputs directly on the diagram (Operate); each edit re-runs
      // the whole graph and re-illuminates it.
      mountInputPills()
      // Focus the history so the run list is immediately keyboard-navigable.
      if (runs.length) operate.focusHistory()
    } else {
      // The input pills belong to Operate; leaving it takes them off the diagram.
      handle?.clearInputPills()
      inputPills = null
      if (m === 'import') importView.render()
    }
  }
  modeDesignBtn.addEventListener('click', () => setMode('design'))
  modeOperateBtn.addEventListener('click', () => setMode('operate'))
  modeImportBtn.addEventListener('click', () => setMode('import'))

  // Sidebar section collapse (VS-Code-style accordion) and flow-catalog refresh.
  const wireToggle = (btnId: string, groupId: string): void => {
    const btn = root.querySelector<HTMLButtonElement>('#' + btnId)
    const group = root.querySelector<HTMLElement>('#' + groupId)
    if (!btn || !group) return
    btn.addEventListener('click', () => {
      const collapsed = group.dataset.collapsed === 'true'
      group.dataset.collapsed = collapsed ? 'false' : 'true'
      btn.setAttribute('aria-expanded', collapsed ? 'true' : 'false')
    })
  }
  wireToggle('flowsToggle', 'groupFlows')
  wireToggle('modelsToggle', 'groupModels')
  root.querySelector<HTMLButtonElement>('#flowRefresh')?.addEventListener('click', () => flowView.render())

  // The toolbar chip shows the currently-open model's content-addressed id and
  // copies the full id (with the sha256: prefix) on click — the exact string the
  // HTTP/MCP surfaces expect, so it can be pasted straight into an API or agent
  // call. The label is shortened for the toolbar; the full id is the title.
  let chipResetTimer = 0
  const setModelIdChip = (modelId: string): void => {
    if (chipResetTimer) window.clearTimeout(chipResetTimer)
    if (!modelId) {
      modelIdChip.hidden = true
      modelIdChip.textContent = ''
      return
    }
    const hex = modelId.startsWith('sha256:') ? modelId.slice('sha256:'.length) : modelId
    const short = hex.length > 12 ? hex.slice(0, 8) + '…' + hex.slice(-4) : hex
    modelIdChip.hidden = false
    modelIdChip.classList.remove('is-copied')
    modelIdChip.dataset.modelId = modelId
    modelIdChip.textContent = 'ID ' + short
    modelIdChip.title = 'Modell-ID kopieren: ' + modelId
  }
  modelIdChip.addEventListener('click', () => {
    const id = modelIdChip.dataset.modelId
    if (!id) return
    void navigator.clipboard.writeText(id).then(() => {
      modelIdChip.classList.add('is-copied')
      const short = modelIdChip.textContent ?? ''
      modelIdChip.textContent = '✓ kopiert'
      if (chipResetTimer) window.clearTimeout(chipResetTimer)
      chipResetTimer = window.setTimeout(() => {
        modelIdChip.classList.remove('is-copied')
        modelIdChip.textContent = short
      }, 1400)
    })
  })

  const showModel = async (modelId: string): Promise<void> => {
    if (!modelId) return
    // Opening a model (L1) leaves the flow studio/designer and returns to the modeler.
    if (mode === 'flows' || mode === 'flow-edit') setMode('design')
    currentId = modelId
    setModelIdChip(modelId)
    renderModelList()
    status.textContent = 'lädt …'
    dirty = false
    // A fresh model view starts an empty run history (its decisions differ) and
    // an empty Import belt (the leaf inputs — hence any template — differ too).
    runs = []
    activeRun = null
    currentModel = null
    importView.reset()
    try {
      // Refresh the type options for this model (built-in + its custom types).
      try {
        typeOptions = [...FEEL_TYPES, ...(await listTypes(modelId)).map((t) => t.name)]
      } catch {
        typeOptions = FEEL_TYPES
      }
      const graph = await getGraph(modelId)
      handle = renderGraph(canvas, layout(graph, { orientation, ortho: true }))
      handle.onChange(() => {
        dirty = true
        syncButtons()
      })
      // One generic pair drives every boxed kind (WP-142): the canvas fires
      // dmn.openLogic/dmn.createLogic with the kind, resolved through the registry.
      handle.onOpenLogic((kind, decisionId) => openLogic(kind, modelId, decisionId))
      handle.onCreateLogic((kind, decisionId) => void createLogic(kind, decisionId))
      handle.onOpenBKM((bkmId) => void openBKM(bkmId))
      handle.onBoxed(() => {
        status.textContent = 'Boxed-Ausdruck dieses Typs — im Modeler noch nicht editierbar.'
      })
      handle.onSelect((sel) => {
        if (sel) {
          typeEditor.style.display = ''
          renderTypeEditor(sel.dataType ?? '')
        } else {
          typeEditor.style.display = 'none'
        }
      })
      syncButtons()
      status.textContent = `${graph.nodes.length} Knoten · ${graph.edges.length} Kanten`
      // Evaluate panel needs the typed per-decision schema, and the model detail
      // also carries the compile diagnostics, which mark the affected nodes and a
      // summary in the status bar — the editor validates against the engine that
      // runs the model (ADR-0016).
      try {
        const detail = await getModel(modelId)
        const diags = detail.diagnostics ?? []
        handle.showDiagnostics(diags)
        const errors = diags.filter((d) => d.severity === 'error').length
        const warnings = diags.filter((d) => d.severity === 'warning').length
        if (errors || warnings) {
          const parts: string[] = []
          if (errors) parts.push(`${errors} Fehler`)
          if (warnings) parts.push(`${warnings} ${warnings === 1 ? 'Warnung' : 'Warnungen'}`)
          status.textContent += ' · ⚠ ' + parts.join(', ')
          status.title = diags.map((d) => (d.severity === 'error' ? '✕ ' : d.severity === 'warning' ? '⚠ ' : 'ℹ ') + d.message).join('\n')
          status.classList.add('status-problem')
        } else {
          status.title = ''
          status.classList.remove('status-problem')
        }
        renderEvaluatePanel(evalHost, detail, (run) => recordRun(run))
        // Share the loaded model with the Import cockpit (template + run source).
        currentModel = detail
        if (mode === 'import') importView.render()
        if (mode === 'operate') clioReplay.render()
      } catch {
        evalHost.textContent = ''
      }
      if (mode === 'operate') {
        operate.render()
        mountInputPills()
      }
    } catch (e) {
      status.textContent = (e as Error).message
    }
  }

  // Default to a clean demo DRG if present, else the first group's newest model.
  // On an empty server there is nothing to select — just render the empty list and
  // leave the canvas blank; the shell stays fully interactive (H3).
  const preferred = ['Pricing', 'Routing', 'Alterskette (Demo)']
  const groups = groupModels()
  const best = groups.find((g) => preferred.includes(g.name)) ?? groups[0]
  if (best) {
    await showModel(best.revisions[0].modelId)
  } else {
    renderModelList()
  }

  // Populate the Flows (L2a) catalog in the sidebar; it stays visible in every
  // mode. Opening a flow from it switches the editor to the flow studio.
  flowView.render()
}

const root = document.getElementById('app')
if (root) void boot(root)
