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

type Named = { name?: string; dataType?: string; varName?: string }
const SUBTLE = '#7a8597'

const FONT = 'system-ui, -apple-system, sans-serif'

function drawText(parent: SVGElement, content: string, cx: number, cy: number, size: number, color: string, weight: string): void {
  const t = create('text')
  attr(t, {
    x: cx, y: cy, 'text-anchor': 'middle', 'dominant-baseline': 'central',
    'font-family': FONT, 'font-size': String(size),
    'font-weight': weight, fill: color,
  })
  t.textContent = content
  append(parent, t)
}

// Accurate text measurement via an offscreen canvas, so labels can be wrapped and
// shrunk to fit the node rather than overflowing it.
const measureCtx = typeof document !== 'undefined' ? document.createElement('canvas').getContext('2d') : null
function textWidth(s: string, size: number, weight: string): number {
  if (!measureCtx) return s.length * size * 0.55
  measureCtx.font = `${weight} ${size}px ${FONT}`
  return measureCtx.measureText(s).width
}

// wrapAll word-wraps text to maxW, hard-breaking any single word that is wider
// than the line, and returns every resulting line.
function wrapAll(text: string, maxW: number, size: number, weight: string): string[] {
  const lines: string[] = []
  let cur = ''
  const fits = (s: string): boolean => textWidth(s, size, weight) <= maxW
  for (const word of text.split(/\s+/).filter(Boolean)) {
    if (fits(cur ? cur + ' ' + word : word)) {
      cur = cur ? cur + ' ' + word : word
      continue
    }
    if (cur && fits(word)) {
      lines.push(cur)
      cur = word
      continue
    }
    if (cur) {
      lines.push(cur)
      cur = ''
    }
    let chunk = ''
    for (const ch of word) {
      if (fits(chunk + ch) || chunk === '') chunk += ch
      else {
        lines.push(chunk)
        chunk = ch
      }
    }
    cur = chunk
  }
  if (cur) lines.push(cur)
  return lines
}

// ellipsizeToWidth trims s and appends an ellipsis until it fits maxW.
function ellipsizeToWidth(s: string, maxW: number, size: number, weight: string): string {
  if (textWidth(s, size, weight) <= maxW) return s
  let t = s
  while (t && textWidth(t + '…', size, weight) > maxW) t = t.slice(0, -1)
  return t + '…'
}

// fitName lays out a name within maxW: it picks the largest font (down to a floor)
// at which the name wraps into at most maxLines without splitting a word, so long
// single words (e.g. "MonatlichesEinkommen") shrink rather than break; if even the
// floor size can't fit, it hard-breaks and ellipsizes the last line.
function fitName(name: string, maxW: number, maxLines: number): { size: number; lines: string[] } {
  const words = name.split(/\s+/).filter(Boolean)
  for (let size = 13; size >= 10; size -= 0.5) {
    const lines = wrapAll(name, maxW, size, '500')
    const noBreak = words.every((w) => textWidth(w, size, '500') <= maxW)
    if (lines.length <= maxLines && noBreak) return { size, lines }
  }
  const size = 10
  let lines = wrapAll(name, maxW, size, '500')
  if (lines.length > maxLines) {
    lines = lines.slice(0, maxLines)
    lines[maxLines - 1] = ellipsizeToWidth(lines[maxLines - 1] + '…', maxW, size, '500')
  }
  return { size, lines }
}

// label draws the element name and, below it, a subtle second line carrying the
// data contract: the type on an InputData, the output variable (name : type) on
// a Decision — so it's visible how a decision's result is referenced (ADR-0016).
// Long names are wrapped/shrunk to fit the node instead of overflowing it.
function label(parent: SVGElement, shape: Shape & Named, w: number, h: number): void {
  const name = shape.name ?? shape.id
  let sub = ''
  if (shape.type === 'dmn:inputData') {
    // Show the FEEL identifier only when it differs from the display label (a
    // free-form name), so the modeler sees how to reference the input; otherwise
    // just the type.
    const vn = shape.varName && shape.varName !== name ? shape.varName : ''
    sub = vn ? '▸ ' + vn + (shape.dataType ? ' : ' + shape.dataType : '') : shape.dataType ?? ''
  } else if (shape.type === 'dmn:decision') {
    const vn = shape.varName ?? name
    sub = '▸ ' + vn + (shape.dataType ? ' : ' + shape.dataType : '')
  } else if (shape.dataType) {
    sub = shape.dataType
  }

  // The label sits at the vertical centre, where even a pill (InputData) is at
  // full width, so a small uniform padding keeps text off the borders.
  const maxW = w - 18
  const { size, lines } = fitName(name, maxW, sub ? 2 : 3)
  const lineH = size + 3
  const subH = sub ? 14 : 0
  let y = h / 2 - (lines.length * lineH + subH) / 2 + lineH / 2
  for (const ln of lines) {
    drawText(parent, ln, w / 2, y, size, TEXT, '500')
    y += lineH
  }
  if (sub) drawText(parent, ellipsizeToWidth(sub, maxW, 10.5, '400'), w / 2, y + 1, 10.5, SUBTLE, '400')
}

// The kind of logic a decision carries, used to pick its corner icon.
type LogicKind = 'table' | 'expression' | 'undecided'

// decisionIcon draws the small type badge in the top-left corner of a decision
// (the "decision logic" indicator, like dmn-js), with a glyph that distinguishes
// its logic: a grid for a decision table, chevrons for a boxed/literal expression,
// and a muted, empty badge for a decision with no logic yet.
function decisionIcon(parent: SVGElement, kind: LogicKind): void {
  const badge = create('rect')
  attr(badge, { x: 8, y: 8, width: 18, height: 18, rx: 3, fill: kind === 'undecided' ? '#94a3b8' : '#3f74e0' })
  append(parent, badge)
  const d =
    kind === 'table'
      ? 'M12 13 H22 M12 17 H22 M12 21 H22 M15 12.5 V21.5' // table grid
      : kind === 'expression'
        ? 'M15 13 L11.5 17 L15 21 M19 13 L22.5 17 L19 21' // < >  (boxed expression)
        : 'M13.5 17 H20.5' // — (no logic yet)
  const glyph = create('path')
  attr(glyph, { d, stroke: '#ffffff', 'stroke-width': 1.4, fill: 'none', 'stroke-linecap': 'round', 'stroke-linejoin': 'round' })
  append(parent, glyph)
}

// roundedPath turns a polyline into an SVG path that keeps the same corners but
// eases each interior bend with a short quadratic arc (radius r, capped to half
// of each adjacent segment so tight corners can't overshoot). Used to render a
// 'gerundete' (curved) requirement edge from the orthogonal waypoints.
function roundedPath(pts: Point[], r: number): string {
  let d = `M${pts[0].x},${pts[0].y}`
  for (let i = 1; i < pts.length - 1; i++) {
    const p0 = pts[i - 1]
    const p1 = pts[i]
    const p2 = pts[i + 1]
    const len1 = Math.hypot(p1.x - p0.x, p1.y - p0.y) || 1
    const len2 = Math.hypot(p2.x - p1.x, p2.y - p1.y) || 1
    const rr = Math.min(r, len1 / 2, len2 / 2)
    const ax = p1.x - ((p1.x - p0.x) / len1) * rr
    const ay = p1.y - ((p1.y - p0.y) / len1) * rr
    const bx = p1.x + ((p2.x - p1.x) / len2) * rr
    const by = p1.y + ((p2.y - p1.y) / len2) * rr
    d += ` L${ax},${ay} Q${p1.x},${p1.y} ${bx},${by}`
  }
  const last = pts[pts.length - 1]
  d += ` L${last.x},${last.y}`
  return d
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
    if (shape.type === 'dmn:decision' || shape.type === undefined) {
      const s = shape as Shape & { hasTable?: boolean; hasLogic?: boolean }
      decisionIcon(parent, s.hasTable ? 'table' : s.hasLogic ? 'expression' : 'undecided')
    }
    label(parent, shape as Shape & Named, w, h)
    return visual
  }

  drawConnection(parent: SVGElement, connection: Connection): SVGElement {
    const wps: Point[] = connection.waypoints ?? []
    const dashed = connection.type !== 'dmn:informationRequirement'
    const style = (connection as { connectionStyle?: string }).connectionStyle
    const common = { class: 'dmn-edge', stroke: EDGE, 'stroke-width': 1.5, fill: 'none', ...(dashed ? { 'stroke-dasharray': '6 4' } : {}) }
    // 'curved' (gerundet) draws the same waypoints with eased corners; 'eckig'
    // (default) and 'direct' are straight polylines — the layouter already gives
    // 'direct' just two border points.
    let line: SVGElement
    if (style === 'curved' && wps.length >= 3) {
      line = create('path')
      attr(line, { d: roundedPath(wps, 12), ...common })
    } else {
      line = create('polyline')
      attr(line, { points: wps.map((p) => `${p.x},${p.y}`).join(' '), ...common })
    }
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
