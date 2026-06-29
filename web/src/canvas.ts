import Diagram from 'diagram-js'
import type Canvas from 'diagram-js/lib/core/Canvas'
import type ElementFactory from 'diagram-js/lib/core/ElementFactory'
import type { Shape } from 'diagram-js/lib/model/Types'
import 'diagram-js/assets/diagram-js.css'
import { dmnRendererModule } from './dmn-renderer'

type NodeDef = { id: string; type: string; name: string; x: number; y: number; w: number; h: number }
type EdgeDef = { id: string; type: string; source: string; target: string; waypoints: { x: number; y: number }[] }

// A small, representative DRG rendered with temis' OWN DMN renderers (WP-65) on
// the diagram-js core — no dmn-js. Positions are hand-set for this sample; real
// model loading + DMNDI layout (or auto-layout) follow in the full WP-65.
const NODES: NodeDef[] = [
  { id: 'preis', type: 'dmn:inputData', name: 'Preis', x: 60, y: 300, w: 110, h: 50 },
  { id: 'saison', type: 'dmn:inputData', name: 'Saison', x: 230, y: 300, w: 110, h: 50 },
  { id: 'tabelle', type: 'dmn:businessKnowledgeModel', name: 'Rabatt-Tabelle', x: 410, y: 286, w: 150, h: 64 },
  { id: 'rabatt', type: 'dmn:decision', name: 'Rabatt', x: 230, y: 160, w: 140, h: 70 },
  { id: 'kaufen', type: 'dmn:decision', name: 'darfIchKaufen', x: 230, y: 30, w: 140, h: 70 },
]
const EDGES: EdgeDef[] = [
  { id: 'e1', type: 'dmn:informationRequirement', source: 'preis', target: 'rabatt', waypoints: [{ x: 115, y: 300 }, { x: 275, y: 232 }] },
  { id: 'e2', type: 'dmn:informationRequirement', source: 'saison', target: 'rabatt', waypoints: [{ x: 285, y: 300 }, { x: 300, y: 232 }] },
  { id: 'e3', type: 'dmn:knowledgeRequirement', source: 'tabelle', target: 'rabatt', waypoints: [{ x: 410, y: 312 }, { x: 372, y: 210 }] },
  { id: 'e4', type: 'dmn:informationRequirement', source: 'rabatt', target: 'kaufen', waypoints: [{ x: 300, y: 158 }, { x: 300, y: 102 }] },
]

export function mountCanvas(container: HTMLElement): void {
  const diagram = new Diagram({ canvas: { container }, modules: [dmnRendererModule] })
  const canvas = diagram.get<Canvas>('canvas')
  const factory = diagram.get<ElementFactory>('elementFactory')

  const byId: Record<string, Shape> = {}
  for (const n of NODES) {
    // name/type are DMN extras carried on the element for our renderer to read.
    const shape = factory.createShape({ id: n.id, x: n.x, y: n.y, width: n.w, height: n.h, type: n.type, name: n.name } as never)
    canvas.addShape(shape)
    byId[n.id] = shape
  }
  for (const e of EDGES) {
    const conn = factory.createConnection({ id: e.id, type: e.type, source: byId[e.source], target: byId[e.target], waypoints: e.waypoints } as never)
    canvas.addConnection(conn)
  }

  canvas.zoom('fit-viewport')
}
