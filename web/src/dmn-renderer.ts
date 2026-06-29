import BaseRenderer from 'diagram-js/lib/draw/BaseRenderer'
import type { Element, Shape, Connection } from 'diagram-js/lib/model/Types'
import type EventBus from 'diagram-js/lib/core/EventBus'
import { append, attr, create } from 'tiny-svg'

type Point = { x: number; y: number }

// Custom diagram-js renderers for the DMN DRG vocabulary (ADR-0016, WP-65),
// drawn directly with tiny-svg — no dmn-js. This is the start of temis owning
// how DMN elements look: Decision (rectangle), InputData (stadium/oval),
// BusinessKnowledgeModel (clipped corners), plus requirement edges (information
// = solid arrow, knowledge/authority = dashed). Real model loading + DMNDI
// layout follow in the full WP-65 / WP-62-JS.

const HIGH_PRIORITY = 1500

// A small, cohesive DMN palette — dark consistent borders, lightly tinted fills
// per element kind, one muted accent for edges. Tuned for a clean, intentional
// look rather than wireframe boxes.
const STROKE = '#2b313c'
const EDGE = '#5b6675'
const FILL_DECISION = '#ffffff'
const FILL_INPUT = '#eef3ff'
const FILL_BKM = '#edfaf1'
const TEXT = '#1f2632'

type Named = { name?: string }

function text(parent: SVGElement, content: string, w: number, h: number): void {
  const t = create('text')
  attr(t, {
    x: w / 2, y: h / 2, 'text-anchor': 'middle', 'dominant-baseline': 'central',
    'font-family': 'system-ui, -apple-system, sans-serif', 'font-size': '13',
    'font-weight': '500', fill: TEXT,
  })
  t.textContent = content
  append(parent, t)
}

function arrowHead(from: Point, to: Point): SVGElement {
  const a = Math.atan2(to.y - from.y, to.x - from.x)
  const s = 9
  const spread = 0.4
  const p = (off: number) => `${to.x - s * Math.cos(a - off)},${to.y - s * Math.sin(a - off)}`
  const head = create('polygon')
  attr(head, { points: `${to.x},${to.y} ${p(spread)} ${p(-spread)}`, fill: EDGE, stroke: EDGE })
  return head
}

export default class DmnRenderer extends BaseRenderer {
  static $inject = ['eventBus']

  constructor(eventBus: EventBus) {
    super(eventBus, HIGH_PRIORITY)
  }

  canRender(element: Element): boolean {
    return typeof element.type === 'string' && element.type.indexOf('dmn:') === 0
  }

  drawShape(parent: SVGElement, shape: Shape): SVGElement {
    const w = shape.width ?? 0
    const h = shape.height ?? 0
    let visual: SVGElement

    if (shape.type === 'dmn:inputData') {
      visual = create('rect')
      attr(visual, { x: 0, y: 0, width: w, height: h, rx: h / 2, ry: h / 2, stroke: STROKE, 'stroke-width': 1.6, fill: FILL_INPUT })
    } else if (shape.type === 'dmn:businessKnowledgeModel') {
      const c = 14
      visual = create('path')
      attr(visual, { d: `M${c},0 L${w},0 L${w},${h - c} L${w - c},${h} L0,${h} L0,${c} Z`, stroke: STROKE, 'stroke-width': 1.6, fill: FILL_BKM })
    } else {
      // dmn:decision (default) — sharp DMN rectangle, just softened corners
      visual = create('rect')
      attr(visual, { x: 0, y: 0, width: w, height: h, rx: 3, ry: 3, stroke: STROKE, 'stroke-width': 1.6, fill: FILL_DECISION })
    }

    append(parent, visual)
    text(parent, (shape as Shape & Named).name ?? shape.id, w, h)
    return visual
  }

  drawConnection(parent: SVGElement, connection: Connection): SVGElement {
    const wps: Point[] = connection.waypoints ?? []
    const line = create('polyline')
    const dashed = connection.type !== 'dmn:informationRequirement'
    attr(line, {
      points: wps.map((p) => `${p.x},${p.y}`).join(' '),
      stroke: EDGE, 'stroke-width': 1.5, fill: 'none',
      ...(dashed ? { 'stroke-dasharray': '6 4' } : {}),
    })
    append(parent, line)
    if (wps.length >= 2) {
      append(parent, arrowHead(wps[wps.length - 2], wps[wps.length - 1]))
    }
    return line
  }
}

// didi module: register the renderer at high priority so it wins over the
// diagram-js DefaultRenderer for every dmn:* element.
export const dmnRendererModule = {
  __init__: ['dmnRenderer'],
  dmnRenderer: ['type', DmnRenderer],
}
