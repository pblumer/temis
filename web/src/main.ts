import { APP_NAME } from './build-info'
import { listModels, getGraph, getModel, createModel, saveGraph, createDecisionTable, listTypes, type ModelSummary } from './api'
import { layout } from './layout'
import { renderGraph, type ModelerHandle } from './canvas'
import { renderEvaluatePanel } from './evaluate'
import { openTableOverlay } from './table'
import { openLiteralOverlay } from './literal'
import { openTypeManager } from './typemanager'
import { FEEL_TYPES } from './feeltypes'
import './style.css'

// WP-65: the modeler now loads a REAL model from temis and draws its decision
// requirements graph with our own DMN renderers on the diagram-js core — no
// dmn-js, no DMNDI needed (positions are auto-laid-out). A picker switches
// between the server's models. Editing interactions follow in the full WP-65.
async function boot(root: HTMLElement): Promise<void> {
  root.innerHTML = `
    <main>
      <h1>${APP_NAME}</h1>
      <p class="sub">Eigener DMN-Modeler · diagram-js (MIT) + eigene Renderer · echtes Modell aus temis · offline (ADR-0016)</p>
      <div class="toolbar">
        <label for="model">Modell</label>
        <select id="model"></select>
        <button id="open" class="tbtn" type="button" title="DMN-Datei laden (.dmn/.xml)">Öffnen…</button>
        <input id="file" type="file" accept=".dmn,.xml,application/xml,text/xml" hidden>
        <button id="undo" class="tbtn" type="button" disabled title="Rückgängig (Strg/Cmd+Z)">↶</button>
        <button id="redo" class="tbtn" type="button" disabled title="Wiederholen (Strg/Cmd+Umschalt+Z)">↷</button>
        <button id="save" class="tbtn" type="button" disabled title="Änderungen speichern (Strg/Cmd+S)">Speichern</button>
        <button id="types" class="tbtn" type="button" title="Eigene Typen verwalten">Typen</button>
        <span id="typeEditor" class="type-editor" style="display:none">
          <label for="datatype">Typ</label>
          <select id="datatype"></select>
        </span>
        <span id="status" class="status"></span>
      </div>
      <div id="canvas" class="canvas"></div>
      <section class="eval-panel">
        <h2 class="eval-title">Auswerten</h2>
        <div id="eval"></div>
      </section>
      <p class="hint">
        Eigener Modeler ohne dmn-js: DRG über <code>/v1/models/{id}/graph</code>,
        Bearbeiten/Speichern über <code>/save</code>, Auswerten über
        <code>/evaluate</code>. <strong>Öffnen…</strong> lädt eine DMN-Datei in die
        Engine. Knoten sind verschieb-/umbenennbar (Doppelklick); ein
        <strong>Doppelklick auf eine Decision mit Tabelle öffnet die Decision
        Table</strong>. Jede Änderung läuft über den Command-Stack (Undo/Redo,
        Strg/Cmd+Z).
      </p>
    </main>`

  const select = root.querySelector<HTMLSelectElement>('#model')
  const canvas = root.querySelector<HTMLElement>('#canvas')
  const status = root.querySelector<HTMLElement>('#status')
  const undoBtn = root.querySelector<HTMLButtonElement>('#undo')
  const redoBtn = root.querySelector<HTMLButtonElement>('#redo')
  const saveBtn = root.querySelector<HTMLButtonElement>('#save')
  const openBtn = root.querySelector<HTMLButtonElement>('#open')
  const fileInput = root.querySelector<HTMLInputElement>('#file')
  const evalHost = root.querySelector<HTMLElement>('#eval')
  const typesBtn = root.querySelector<HTMLButtonElement>('#types')
  const typeEditor = root.querySelector<HTMLElement>('#typeEditor')
  const datatype = root.querySelector<HTMLSelectElement>('#datatype')
  if (!select || !canvas || !status || !undoBtn || !redoBtn || !saveBtn || !openBtn || !fileInput || !typesBtn || !evalHost || !typeEditor || !datatype) return

  // The type options offered in the InputData/table/literal pickers: the built-in
  // FEEL types plus the current model's custom item definitions (refreshed per
  // model in show()).
  let typeOptions: string[] = FEEL_TYPES
  const renderTypeEditor = (selected?: string): void => {
    const opts = selected && !typeOptions.includes(selected) ? [...typeOptions, selected] : typeOptions
    datatype.innerHTML = opts.map((t) => `<option value="${t}">${t || '— Typ —'}</option>`).join('')
    if (selected !== undefined) datatype.value = selected
  }
  renderTypeEditor()
  datatype.addEventListener('change', () => handle?.setSelectedType(datatype.value))

  let handle: ModelerHandle | null = null
  let dirty = false
  const syncButtons = (): void => {
    undoBtn.disabled = !handle?.canUndo()
    redoBtn.disabled = !handle?.canRedo()
    saveBtn.disabled = !dirty
  }
  undoBtn.addEventListener('click', () => handle?.undo())
  redoBtn.addEventListener('click', () => handle?.redo())

  // save persists the current diagram's edits, then switches the picker to the
  // server's new revision (its content hash, hence its modelId, changed).
  // persistGraph posts the live canvas graph (moved/renamed/retyped nodes AND
  // nodes/edges added or removed, ADR-0016) and returns the saved model's id. It
  // is a no-op returning modelId unchanged when there is nothing to save.
  const persistGraph = async (modelId: string): Promise<string> => {
    if (!handle || !dirty) return modelId
    const { nodes, edges } = handle.graph()
    const saved = await saveGraph(modelId, {
      nodes: nodes.map((n) => ({ ...n, dataType: n.type === 'inputData' ? (n.dataType ?? '') : undefined })),
      edges,
    })
    return saved.modelId
  }

  const save = async (): Promise<void> => {
    if (!handle || !dirty) return
    const current = models[Number(select.value)]
    if (!current) return
    saveBtn.disabled = true
    status.textContent = 'speichert …'
    try {
      await reselect(await persistGraph(current.modelId))
      status.textContent = 'gespeichert ✓'
    } catch (e) {
      status.textContent = (e as Error).message
      syncButtons()
    }
  }

  // createTable gives a table-less decision a fresh table: persist any pending
  // structural edits first (so the decision exists server-side), create the
  // table, switch to the saved revision and open it for editing.
  const createTable = async (decisionId: string): Promise<void> => {
    const current = models[Number(select.value)]
    if (!current) return
    status.textContent = 'legt Tabelle an …'
    try {
      const created = await createDecisionTable(await persistGraph(current.modelId), decisionId)
      await reselect(created.modelId)
      status.textContent = 'Tabelle angelegt ✓'
      void openTableOverlay(created.modelId, decisionId, (newId) => void reselect(newId))
    } catch (e) {
      status.textContent = (e as Error).message
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
  const openLiteral = (modelId: string, decisionId: string, fresh = false): void => {
    const { names, title } = namesFor(decisionId)
    void openLiteralOverlay(modelId, decisionId, title, names, (newId) => void reselect(newId), { fresh, typeOptions })
  }

  // Typen: open the custom-type manager; each save/delete switches to the saved
  // revision (which refreshes typeOptions via show()).
  typesBtn.addEventListener('click', () => {
    const current = models[Number(select.value)]
    if (current) void openTypeManager(current.modelId, (newId) => reselect(newId))
  })
  // createLiteral persists pending structural edits (so the decision exists), then
  // opens an empty literal editor for it; saving creates the expression.
  const createLiteral = async (decisionId: string): Promise<void> => {
    const current = models[Number(select.value)]
    if (!current) return
    status.textContent = 'legt Ausdruck an …'
    try {
      const baseId = await persistGraph(current.modelId)
      await reselect(baseId)
      status.textContent = ''
      openLiteral(baseId, decisionId, true)
    } catch (e) {
      status.textContent = (e as Error).message
    }
  }
  saveBtn.addEventListener('click', () => void save())

  // reselect refreshes the model list and switches the picker to modelId (e.g.
  // after a save or an upload created/changed a cached model).
  const reselect = async (modelId: string): Promise<void> => {
    models = await listModels()
    models.sort((a, b) => (a.name ?? a.modelId).localeCompare(b.name ?? b.modelId))
    renderOptions()
    const idx = models.findIndex((m) => m.modelId === modelId)
    select.value = String(idx < 0 ? 0 : idx)
    await show(Number(select.value))
  }

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

  let models: ModelSummary[] = []
  try {
    models = await listModels()
  } catch (e) {
    status.textContent = (e as Error).message
    return
  }
  if (!models.length) {
    status.textContent = 'Keine Modelle auf dem Server.'
    return
  }
  models.sort((a, b) => (a.name ?? a.modelId).localeCompare(b.name ?? b.modelId))
  const renderOptions = (): void => {
    select.innerHTML = models
      .map((m, i) => `<option value="${i}">${m.name ?? m.modelId.slice(0, 18)}</option>`)
      .join('')
  }
  renderOptions()

  const show = async (index: number): Promise<void> => {
    const model = models[index]
    status.textContent = 'lädt …'
    dirty = false
    try {
      // Refresh the type options for this model (built-in + its custom types).
      try {
        typeOptions = [...FEEL_TYPES, ...(await listTypes(model.modelId)).map((t) => t.name)]
      } catch {
        typeOptions = FEEL_TYPES
      }
      const graph = await getGraph(model.modelId)
      handle = renderGraph(canvas, layout(graph))
      handle.onChange(() => {
        dirty = true
        syncButtons()
      })
      handle.onOpenTable((decisionId) => void openTableOverlay(model.modelId, decisionId, (newId) => void reselect(newId), typeOptions))
      handle.onCreateTable((decisionId) => void createTable(decisionId))
      handle.onOpenLiteral((decisionId) => openLiteral(model.modelId, decisionId))
      handle.onCreateLiteral((decisionId) => void createLiteral(decisionId))
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
      // Evaluate panel: needs the typed per-decision schema, so fetch the detail.
      try {
        renderEvaluatePanel(evalHost, await getModel(model.modelId))
      } catch {
        evalHost.textContent = ''
      }
    } catch (e) {
      status.textContent = (e as Error).message
    }
  }

  select.addEventListener('change', () => void show(Number(select.value)))

  // Default to a clean demo DRG if present, else the first model.
  const preferred = ['Pricing', 'Routing', 'Alterskette (Demo)']
  let best = models.findIndex((m) => preferred.includes(m.name ?? ''))
  if (best < 0) best = 0
  select.value = String(best)
  await show(best)
}

const root = document.getElementById('app')
if (root) void boot(root)
