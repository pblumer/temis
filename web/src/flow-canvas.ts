// A read-only diagram-js canvas for decision flows (WP-97, WP-98). It reuses
// temis' own DMN renderer on the diagram-js MIT core — the same boxes and labels
// as the DMN modeler — but with a trimmed, view-only module set (no palette,
// modeling, context-pad or connect): a flow is browsed and run, not edited here.
// After an evaluation it *illuminates* the flow: each step lights up with its
// result and every wire shows the value that travelled it, staggered in
// evaluation order so the decision visibly propagates from inputs to output.

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

// NodeLight is one step's result and its reveal delay (ms) — later steps light up
// after the ones they depend on, so the wave runs in evaluation order.
export type NodeLight = { value: string; delay: number }
// EdgeLight is the value that travelled a wire, with the same staggered delay.
export type EdgeLight = { value: string; delay: number }
// FlowIllum is a whole run's illumination: results per step node and values per
// edge (keyed by the laid edge id).
export type FlowIllum = { nodes: Record<string, NodeLight>; edges: Record<string, EdgeLight> }

// FlowCanvas is the handle to a rendered flow graph: illuminate it with a run's
// results, or clear the illumination.
export type FlowCanvas = {
  illuminate: (ill: FlowIllum) => void
  clear: () => void
}

// mounted tracks the flow diagram per container, so re-rendering a container tears
// down only its own previous diagram. Keying by container (not a single global)
// lets the studio (#flowCanvas) and the designer's live preview (#feCanvas) hold
// independent diagrams — a late preview refresh must never destroy the studio's
// diagram, and vice versa. Independent of the DMN modeler's own instance.
const mounted = new WeakMap<HTMLElement, Diagram>()

// fit fits the whole diagram into the viewport.
function fit(canvas: Canvas): void {
  ;(canvas as unknown as { zoom: (mode: string) => number }).zoom('fit-viewport')
}

// renderFlowGraph draws laid into container and returns a handle for illumination.
// Step nodes are drawn as decisions, flow-input nodes as input data.
export function renderFlowGraph(container: HTMLElement, laid: Laid): FlowCanvas {
  const prev = mounted.get(container)
  if (prev) prev.destroy()
  container.innerHTML = ''
  const diagram = new Diagram({
    canvas: { container },
    modules: [dmnRendererModule, MoveCanvasModule, ZoomScrollModule, OverlaysModule],
  })
  mounted.set(container, diagram)
  const canvas = diagram.get<Canvas>('canvas')
  const factory = diagram.get<ElementFactory>('elementFactory')
  const overlays = diagram.get<Overlays>('overlays')
  const registry = diagram.get<ElementRegistry>('elementRegistry')
  const marker = canvas as unknown as { addMarker: (id: string, m: string) => void; removeMarker: (id: string, m: string) => void }

  const byId: Record<string, Shape> = {}
  for (const n of laid.nodes) {
    const shape = factory.createShape({
      id: n.id, x: n.x, y: n.y, width: n.w, height: n.h,
      type: 'dmn:' + n.type, name: n.name, varName: n.varName, dataType: n.dataType,
    } as never)
    canvas.addShape(shape)
    byId[n.id] = shape
  }
  // For each drawn edge, remember where to anchor its value label: the midpoint of
  // its waypoints, expressed as an offset from the source node's top-left (shape
  // overlays position relative to the element, the proven path the modeler uses).
  const edgeAnchor: Record<string, { nodeId: string; left: number; top: number }> = {}
  for (const e of laid.edges) {
    const src = byId[e.source]
    if (!src || !byId[e.target]) continue
    canvas.addConnection(factory.createConnection({
      id: e.id, type: 'dmn:' + e.type, source: src, target: byId[e.target], waypoints: e.waypoints,
    } as never))
    const wp = e.waypoints
    const mid = { x: (wp[0].x + wp[wp.length - 1].x) / 2, y: (wp[0].y + wp[wp.length - 1].y) / 2 }
    edgeAnchor[e.id] = { nodeId: e.source, left: mid.x - src.x, top: mid.y - src.y }
  }
  // Only fit when the container is actually laid out: fitting a zero-size (hidden)
  // container makes diagram-js divide by an empty viewport. Callers render into a
  // visible host, but this keeps a stray hidden render from throwing.
  if (container.clientWidth > 0 && container.clientHeight > 0) fit(canvas)

  const clear = (): void => {
    overlays.remove({ type: 'eval-result' })
    overlays.remove({ type: 'flow-edge' })
    for (const id of Object.keys(edgeAnchor)) marker.removeMarker(id, 'flow-active')
  }

  return {
    illuminate: (ill) => {
      clear()
      // Step nodes: a result badge, revealed after the steps it depends on.
      for (const el of registry.getAll()) {
        const s = el as Shape & { type?: string }
        const light = ill.nodes[s.id]
        if (s.type !== 'dmn:decision' || !light) continue
        const badge = document.createElement('div')
        badge.className = 'node-result node-lit'
        badge.style.animationDelay = light.delay + 'ms'
        badge.append(Object.assign(document.createElement('span'), { className: 'node-result-val', textContent: light.value }))
        badge.title = s.id + ' = ' + light.value
        overlays.add(s.id, 'eval-result', { position: { bottom: -4, left: 6 }, html: badge })
      }
      // Edges: colour the active wire and float the value that travelled it.
      for (const [edgeId, anchor] of Object.entries(edgeAnchor)) {
        const light = ill.edges[edgeId]
        if (!light) continue
        marker.addMarker(edgeId, 'flow-active')
        const lbl = document.createElement('div')
        lbl.className = 'flow-edge-val'
        lbl.style.animationDelay = light.delay + 'ms'
        lbl.textContent = light.value
        lbl.title = light.value
        overlays.add(anchor.nodeId, 'flow-edge', { position: { left: anchor.left, top: anchor.top }, html: lbl })
      }
    },
    clear,
  }
}
