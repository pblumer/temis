import { APP_NAME } from './build-info'
import { listModels, getGraph, type ModelSummary } from './api'
import { layout } from './layout'
import { renderGraph } from './canvas'
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
        <span id="status" class="status"></span>
      </div>
      <div id="canvas" class="canvas"></div>
      <p class="hint">
        WP-65: Die DRG wird über <code>/v1/models/{id}/graph</code> aus der Engine
        geladen und mit eigenen Renderern gezeichnet — mit dem <strong>authored
        DMNDI-Layout</strong>, wo vorhanden (sonst Auto-Layout). Selektion/Move/Connect
        und Modellier-Interaktionen folgen.
      </p>
    </main>`

  const select = root.querySelector<HTMLSelectElement>('#model')
  const canvas = root.querySelector<HTMLElement>('#canvas')
  const status = root.querySelector<HTMLElement>('#status')
  if (!select || !canvas || !status) return

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
  select.innerHTML = models
    .map((m, i) => `<option value="${i}">${m.name ?? m.modelId.slice(0, 18)}</option>`)
    .join('')

  const show = async (index: number): Promise<void> => {
    const model = models[index]
    status.textContent = 'lädt …'
    try {
      const graph = await getGraph(model.modelId)
      renderGraph(canvas, layout(graph))
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
