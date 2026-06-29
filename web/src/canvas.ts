import Diagram from 'diagram-js'
import type Canvas from 'diagram-js/lib/core/Canvas'
import type ElementFactory from 'diagram-js/lib/core/ElementFactory'
import type { Shape } from 'diagram-js/lib/model/Types'
import 'diagram-js/assets/diagram-js.css'
import { dmnRendererModule } from './dmn-renderer'
import type { Laid } from './layout'

// Render a laid-out DRG into the container with temis' own DMN renderers on the
// diagram-js MIT core (ADR-0016) — no dmn-js. A fresh diagram is built per call
// (the viewer has no modeling/undo module yet), so the container is cleared
// first; editing interactions land in the full WP-65.
export function renderGraph(container: HTMLElement, laid: Laid): void {
  container.innerHTML = ''
  const diagram = new Diagram({ canvas: { container }, modules: [dmnRendererModule] })
  const canvas = diagram.get<Canvas>('canvas')
  const factory = diagram.get<ElementFactory>('elementFactory')

  const byId: Record<string, Shape> = {}
  for (const n of laid.nodes) {
    // The /v1 graph uses bare type names ("inputData", …); our renderer keys on
    // the "dmn:" vocabulary. name/type are carried on the element for it to read.
    const shape = factory.createShape({ id: n.id, x: n.x, y: n.y, width: n.w, height: n.h, type: 'dmn:' + n.type, name: n.name } as never)
    canvas.addShape(shape)
    byId[n.id] = shape
  }
  for (const e of laid.edges) {
    if (!byId[e.source] || !byId[e.target]) continue
    const conn = factory.createConnection({ id: e.id, type: 'dmn:' + e.type, source: byId[e.source], target: byId[e.target], waypoints: e.waypoints } as never)
    canvas.addConnection(conn)
  }

  canvas.zoom('fit-viewport')
}
