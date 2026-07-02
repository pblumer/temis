import BaseLayouter from 'diagram-js/lib/layout/BaseLayouter'
import type { Connection, Shape } from 'diagram-js/lib/model/Types'
import { orthoRoute, type Box } from './ortho'

type Point = { x: number; y: number }

// DmnLayouter re-routes a requirement edge on every (re)layout — load, move,
// connect (ADR-0016). The DRD is a vertically layered graph, so when the two
// nodes are stacked (the requiring element clearly above or below the required
// one) the edge is routed as an orthogonal, right-angle connector docked on the
// facing top/bottom edges — matching the auto-layout and keeping the picture
// clean after a drag. When the nodes end up side by side (horizontal move
// dominates), an orthogonal top/bottom route would look wrong, so it falls back
// to a straight line docked at both borders, which works for any placement.
//
// The modeler can override an edge's shape per edge (context pad): 'direct'
// forces the straight border-to-border route whatever the placement, while
// 'ortho' (eckig, the default) and 'curved' (gerundet) share the same
// right-angle waypoints — only the renderer differs (sharp vs. rounded corners),
// so both use the orthogonal route here.
class DmnLayouter extends BaseLayouter {
  layoutConnection(connection: Connection, hints?: object): Point[] {
    const s = connection.source as Shape | null
    const t = connection.target as Shape | null
    if (!s || !t) return super.layoutConnection(connection, hints)
    const style = (connection as { connectionStyle?: string }).connectionStyle
    if (style === 'direct') return [borderPoint(s, t), borderPoint(t, s)]
    const sc = centre(s)
    const tc = centre(t)
    if (Math.abs(tc.y - sc.y) >= Math.abs(tc.x - sc.x)) return orthoRoute(box(s), box(t))
    return [borderPoint(s, t), borderPoint(t, s)]
  }
}

function centre(n: Shape): Point {
  return { x: (n.x ?? 0) + (n.width ?? 0) / 2, y: (n.y ?? 0) + (n.height ?? 0) / 2 }
}

function box(n: Shape): Box {
  return { x: n.x ?? 0, y: n.y ?? 0, w: n.width ?? 0, h: n.height ?? 0 }
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
