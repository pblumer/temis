import { APP_NAME } from './build-info'
import { listModels, getGraph, saveModel, type ModelSummary, type NodeEdit } from './api'
import { layout } from './layout'
import { renderGraph, type ModelerHandle } from './canvas'
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
        <button id="undo" class="tbtn" type="button" disabled title="Rückgängig (Strg/Cmd+Z)">↶</button>
        <button id="redo" class="tbtn" type="button" disabled title="Wiederholen (Strg/Cmd+Umschalt+Z)">↷</button>
        <button id="save" class="tbtn" type="button" disabled title="Änderungen speichern (Strg/Cmd+S)">Speichern</button>
        <span id="typeEditor" class="type-editor" style="display:none">
          <label for="datatype">Typ</label>
          <select id="datatype"></select>
        </span>
        <span id="status" class="status"></span>
      </div>
      <div id="canvas" class="canvas"></div>
      <p class="hint">
        WP-65: Die DRG kommt über <code>/v1/models/{id}/graph</code> aus der Engine
        (authored DMNDI-Layout, wo vorhanden, sonst Auto-Layout). <strong>Knoten sind
        anklickbar und verschiebbar</strong>; jede Änderung läuft über den Command-Stack,
        also Undo/Redo (Buttons oder Strg/Cmd+Z). Connect/Rules/Palette folgen.
      </p>
    </main>`

  const select = root.querySelector<HTMLSelectElement>('#model')
  const canvas = root.querySelector<HTMLElement>('#canvas')
  const status = root.querySelector<HTMLElement>('#status')
  const undoBtn = root.querySelector<HTMLButtonElement>('#undo')
  const redoBtn = root.querySelector<HTMLButtonElement>('#redo')
  const saveBtn = root.querySelector<HTMLButtonElement>('#save')
  const typeEditor = root.querySelector<HTMLElement>('#typeEditor')
  const datatype = root.querySelector<HTMLSelectElement>('#datatype')
  if (!select || !canvas || !status || !undoBtn || !redoBtn || !saveBtn || !typeEditor || !datatype) return

  // Built-in FEEL types for the InputData type editor; "" clears the type.
  const FEEL_TYPES = ['', 'string', 'number', 'boolean', 'date', 'time', 'date and time', 'days and time duration', 'years and months duration']
  datatype.innerHTML = FEEL_TYPES.map((t) => `<option value="${t}">${t || '— Typ —'}</option>`).join('')
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
  const save = async (): Promise<void> => {
    if (!handle || !dirty) return
    const current = models[Number(select.value)]
    if (!current) return
    const edits: NodeEdit[] = handle.nodes().map((n) => ({
      id: n.id,
      name: n.name,
      // type only applies to InputData server-side; sending it for others is a no-op.
      dataType: n.type === 'inputData' ? (n.dataType ?? '') : undefined,
      x: n.x,
      y: n.y,
    }))
    saveBtn.disabled = true
    status.textContent = 'speichert …'
    try {
      const newId = await saveModel(current.modelId, edits)
      models = await listModels()
      models.sort((a, b) => (a.name ?? a.modelId).localeCompare(b.name ?? b.modelId))
      renderOptions()
      const idx = models.findIndex((m) => m.modelId === newId)
      select.value = String(idx < 0 ? 0 : idx)
      await show(Number(select.value))
      status.textContent = 'gespeichert ✓'
    } catch (e) {
      status.textContent = (e as Error).message
      syncButtons()
    }
  }
  saveBtn.addEventListener('click', () => void save())
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
      const graph = await getGraph(model.modelId)
      handle = renderGraph(canvas, layout(graph))
      handle.onChange(() => {
        dirty = true
        syncButtons()
      })
      handle.onSelect((sel) => {
        if (sel) {
          typeEditor.style.display = ''
          datatype.value = sel.dataType ?? ''
        } else {
          typeEditor.style.display = 'none'
        }
      })
      syncButtons()
      status.textContent = `${graph.nodes.length} Knoten · ${graph.edges.length} Kanten`
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
