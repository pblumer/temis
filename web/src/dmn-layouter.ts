import BaseLayouter from 'diagram-js/lib/layout/BaseLayouter'
import type { Connection, Shape } from 'diagram-js/lib/model/Types'

type Point = { x: number; y: number }

// DmnLayouter routes a requirement edge as a straight line docked at both node
// borders (the DMN DRD convention). diagram-js's default layouter connects node
// *centres*, so after a move (or on an appended edge) the line — and its hit area
// — runs through the nodes; this keeps every edge cropped to the borders on every
// (re)layout: load, move, connect (ADR-0016).
class DmnLayouter extends BaseLayouter {
  layoutConnection(connection: Connection, hints?: object): Point[] {
    const s = connection.source as Shape | null
    const t = connection.target as Shape | null
    if (!s || !t) return super.layoutConnection(connection, hints)
    return [borderPoint(s, t), borderPoint(t, s)]
  }
}

// borderPoint returns the point on node's border on the line from its centre
// toward other's centre.
function borderPoint(node: Shape, other: Shape): Point {
  const cx = (node.x ?? 0) + (node.width ?? 0) / 2
  const cy = (node.y ?? 0) + (node.height ?? 0) / 2
  const ox = (other.x ?? 0) + (other.width ?? 0) / 2
  const oy = (other.y ?? 0) + (other.height ?? 0) / 2
  const dx = ox - cx
  const dy = oy - cy
  if (dx === 0 && dy === 0) return { x: cx, y: cy }
  const t = 1 / Math.max(Math.abs(dx) / ((node.width ?? 0) / 2 || 1), Math.abs(dy) / ((node.height ?? 0) / 2 || 1))
  return { x: cx + dx * t, y: cy + dy * t }
}

export const dmnLayouterModule = {
  layouter: ['type', DmnLayouter],
}
