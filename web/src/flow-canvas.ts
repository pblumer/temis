// A read-only diagram-js canvas for decision flows (WP-97). It reuses temis' own
// DMN renderer on the diagram-js MIT core — the same boxes and labels as the DMN
// modeler — but with a trimmed, view-only module set (no palette, modeling,
// context-pad or connect): a flow is browsed and run, not edited here. It renders
// a laid-out graph and overlays each step's result after an evaluation.

import Diagram from 'diagram-js'
import MoveCanvasModule from 'diagram-js/lib/navigation/movecanvas'
import ZoomScrollModule from 'diagram-js/lib/navigation/zoomscroll'
import OverlaysModule from 'diagram-js/lib/features/overlays'
import type Overlays from 'diagram-js/lib/features/overlays/Overlays'
import type Canvas from 'diagram-js/lib/core/Canvas'
import type ElementFactory from 'diagram-js/lib/core/ElementFactory'
import type ElementRegistry from 'diagram-js/lib/core/ElementRegistry'
import type { Shape } from 'diagram-js/lib/model/Types'
import 'diagram-js/assets/diagram-js.css'
import { dmnRendererModule } from './dmn-renderer'
import type { Laid } from './layout'

// FlowCanvas is the handle to a rendered flow graph: overlay per-step results, or
// clear them.
export type FlowCanvas = {
  showResults: (byNode: Record<string, string>) => void
  clearResults: () => void
}

// current is the mounted flow diagram, destroyed when the next one is built so its
// listeners do not linger. It is independent of the DMN modeler's own instance.
let current: Diagram | null = null

// fit fits the whole diagram into the viewport.
function fit(canvas: Canvas): void {
  ;(canvas as unknown as { zoom: (mode: string) => number }).zoom('fit-viewport')
}

// renderFlowGraph draws laid into container and returns a handle for result
// overlays. Step nodes are drawn as decisions, flow-input nodes as input data.
export function renderFlowGraph(container: HTMLElement, laid: Laid): FlowCanvas {
  if (current) current.destroy()
  container.innerHTML = ''
  const diagram = new Diagram({
    canvas: { container },
    modules: [dmnRendererModule, MoveCanvasModule, ZoomScrollModule, OverlaysModule],
  })
  current = diagram
  const canvas = diagram.get<Canvas>('canvas')
  const factory = diagram.get<ElementFactory>('elementFactory')
  const overlays = diagram.get<Overlays>('overlays')
  const registry = diagram.get<ElementRegistry>('elementRegistry')

  const byId: Record<string, Shape> = {}
  for (const n of laid.nodes) {
    const shape = factory.createShape({
      id: n.id, x: n.x, y: n.y, width: n.w, height: n.h,
      type: 'dmn:' + n.type, name: n.name, varName: n.varName, dataType: n.dataType,
    } as never)
    canvas.addShape(shape)
    byId[n.id] = shape
  }
  for (const e of laid.edges) {
    if (!byId[e.source] || !byId[e.target]) continue
    canvas.addConnection(factory.createConnection({
      id: e.id, type: 'dmn:' + e.type, source: byId[e.source], target: byId[e.target], waypoints: e.waypoints,
    } as never))
  }
  fit(canvas)

  return {
    showResults: (byNode) => {
      overlays.remove({ type: 'eval-result' })
      for (const el of registry.getAll()) {
        const s = el as Shape & { type?: string }
        const v = byNode[s.id]
        if (s.type !== 'dmn:decision' || v === undefined) continue
        const badge = document.createElement('div')
        badge.className = 'node-result'
        badge.append(Object.assign(document.createElement('span'), { className: 'node-result-val', textContent: v }))
        badge.title = s.id + ' = ' + v
        overlays.add(s.id, 'eval-result', { position: { bottom: -4, left: 6 }, html: badge })
      }
    },
    clearResults: () => overlays.remove({ type: 'eval-result' }),
  }
}
