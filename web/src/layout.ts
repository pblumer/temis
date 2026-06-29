import type { Graph } from './api'

// Positioning a DRG for the canvas (ADR-0016, WP-65). When the model carries
// DMNDI, the engine hands us the authored bounds and we use them verbatim;
// otherwise we compute a simple top-down layered layout (row = longest
// requirement chain). Either way, requirement edges are routed border-to-border
// between the node boxes, so they work for any positions.

export type LaidNode = { id: string; type: string; name: string; dataType?: string; varName?: string; hasTable?: boolean; hasLiteral?: boolean; x: number; y: number; w: number; h: number }
export type LaidEdge = { id: string; type: string; source: string; target: string; waypoints: { x: number; y: number }[] }
export type Laid = { nodes: LaidNode[]; edges: LaidEdge[] }

const SIZE: Record<string, { w: number; h: number }> = {
  inputData: { w: 120, h: 50 },
  decision: { w: 150, h: 70 },
  businessKnowledgeModel: { w: 150, h: 64 },
}
const sizeOf = (type: string) => SIZE[type] ?? SIZE.decision

const COL_GAP = 44
const ROW_GAP = 80
const PAD = 24

// borderPoint returns the point on a node's border on the line from its centre
// toward (tx, ty) — used to dock an edge at the box edge rather than its centre.
function borderPoint(n: LaidNode, tx: number, ty: number): { x: number; y: number } {
  const cx = n.x + n.w / 2
  const cy = n.y + n.h / 2
  const dx = tx - cx
  const dy = ty - cy
  if (dx === 0 && dy === 0) return { x: cx, y: cy }
  const t = 1 / Math.max(Math.abs(dx) / (n.w / 2), Math.abs(dy) / (n.h / 2))
  return { x: cx + dx * t, y: cy + dy * t }
}

function autoLayout(graph: Graph, pos: Map<string, LaidNode>): void {
  const sources = new Map<string, string[]>() // node -> nodes it requires
  for (const n of graph.nodes) sources.set(n.id, [])
  for (const e of graph.edges) sources.get(e.target)?.push(e.source)

  const memo = new Map<string, number>()
  const rowOf = (id: string, seen: Set<string>): number => {
    const cached = memo.get(id)
    if (cached !== undefined) return cached
    if (seen.has(id)) return 0 // cycle guard (the engine forbids cycles)
    seen.add(id)
    const reqs = sources.get(id) ?? []
    const row = reqs.length ? 1 + Math.max(...reqs.map((s) => rowOf(s, seen))) : 0
    seen.delete(id)
    memo.set(id, row)
    return row
  }

  const rows = new Map<number, Graph['nodes']>()
  let maxRow = 0
  for (const n of graph.nodes) {
    const r = rowOf(n.id, new Set())
    maxRow = Math.max(maxRow, r)
    const bucket = rows.get(r) ?? []
    bucket.push(n)
    rows.set(r, bucket)
  }

  const rowWidth = (r: number) =>
    (rows.get(r) ?? []).reduce((acc, n) => acc + sizeOf(n.type).w + COL_GAP, -COL_GAP)
  const maxWidth = Math.max(0, ...Array.from({ length: maxRow + 1 }, (_, r) => rowWidth(r)))

  for (let r = 0; r <= maxRow; r++) {
    const bucket = rows.get(r) ?? []
    let x = PAD + (maxWidth - rowWidth(r)) / 2
    const y = PAD + (maxRow - r) * (ROW_GAP + 70)
    for (const n of bucket) {
      const s = sizeOf(n.type)
      pos.set(n.id, { id: n.id, type: n.type, name: n.name, dataType: n.dataType, varName: n.varName, hasTable: n.hasTable, hasLiteral: n.hasLiteral, x, y, w: s.w, h: s.h })
      x += s.w + COL_GAP
    }
  }
}

export function layout(graph: Graph): Laid {
  const pos = new Map<string, LaidNode>()

  // Use authored DMNDI bounds when every node has them, else auto-layout.
  const hasLayout =
    graph.nodes.length > 0 &&
    graph.nodes.every((n) => (n.width ?? 0) > 0 && (n.height ?? 0) > 0)
  if (hasLayout) {
    for (const n of graph.nodes) {
      pos.set(n.id, { id: n.id, type: n.type, name: n.name, dataType: n.dataType, varName: n.varName, hasTable: n.hasTable, hasLiteral: n.hasLiteral, x: n.x ?? 0, y: n.y ?? 0, w: n.width ?? 0, h: n.height ?? 0 })
    }
  } else {
    autoLayout(graph, pos)
  }

  const edges: LaidEdge[] = []
  graph.edges.forEach((e, i) => {
    const s = pos.get(e.source)
    const t = pos.get(e.target)
    if (!s || !t) return
    const sc = { x: s.x + s.w / 2, y: s.y + s.h / 2 }
    const tc = { x: t.x + t.w / 2, y: t.y + t.h / 2 }
    edges.push({
      id: 'edge' + i,
      type: e.type,
      source: e.source,
      target: e.target,
      waypoints: [borderPoint(s, tc.x, tc.y), borderPoint(t, sc.x, sc.y)],
    })
  })

  return { nodes: [...pos.values()], edges }
}
