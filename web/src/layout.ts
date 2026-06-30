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
  const byId = new Map(graph.nodes.map((n) => [n.id, n]))
  const req = new Map<string, string[]>() // node -> nodes it requires (below it)
  const reqBy = new Map<string, string[]>() // node -> nodes that require it (above it)
  for (const n of graph.nodes) {
    req.set(n.id, [])
    reqBy.set(n.id, [])
  }
  for (const e of graph.edges) {
    req.get(e.target)?.push(e.source)
    reqBy.get(e.source)?.push(e.target)
  }

  // Row = longest requirement chain (leaves/inputs at the bottom, row 0).
  const memo = new Map<string, number>()
  const rowOf = (id: string, seen: Set<string>): number => {
    const cached = memo.get(id)
    if (cached !== undefined) return cached
    if (seen.has(id)) return 0 // cycle guard (the engine forbids cycles)
    seen.add(id)
    const reqs = req.get(id) ?? []
    const row = reqs.length ? 1 + Math.max(...reqs.map((s) => rowOf(s, seen))) : 0
    seen.delete(id)
    memo.set(id, row)
    return row
  }

  const rowIds = new Map<number, string[]>()
  let maxRow = 0
  for (const n of graph.nodes) {
    const r = rowOf(n.id, new Set())
    maxRow = Math.max(maxRow, r)
    const bucket = rowIds.get(r) ?? []
    bucket.push(n.id)
    rowIds.set(r, bucket)
  }

  const rowWidth = (ids: string[]) => ids.reduce((acc, id) => acc + sizeOf(byId.get(id)!.type).w + COL_GAP, -COL_GAP)
  const maxWidth = Math.max(0, ...[...rowIds.values()].map(rowWidth))

  // pack lays out each row left-to-right in its current order, centred.
  const pack = (): void => {
    for (const [r, ids] of rowIds) {
      let x = PAD + (maxWidth - rowWidth(ids)) / 2
      const y = PAD + (maxRow - r) * (ROW_GAP + 70)
      for (const id of ids) {
        const n = byId.get(id)!
        const s = sizeOf(n.type)
        pos.set(id, { id, type: n.type, name: n.name, dataType: n.dataType, varName: n.varName, hasTable: n.hasTable, hasLiteral: n.hasLiteral, x, y, w: s.w, h: s.h })
        x += s.w + COL_GAP
      }
    }
  }
  pack()

  // Order each row by the barycentre of its neighbours' horizontal centres, so a
  // node sits under/over the elements it connects to (e.g. an input lands beneath
  // the decisions it feeds) instead of wherever it happened to be listed. A few
  // sweeps settle it; unconnected nodes keep their relative position.
  const cx = (id: string): number => {
    const p = pos.get(id)
    return p ? p.x + p.w / 2 : 0
  }
  for (let iter = 0; iter < 8; iter++) {
    for (const ids of rowIds.values()) {
      const want = new Map<string, number>()
      for (const id of ids) {
        const nb = [...(req.get(id) ?? []), ...(reqBy.get(id) ?? [])]
        want.set(id, nb.length ? nb.reduce((a, n) => a + cx(n), 0) / nb.length : cx(id))
      }
      ids.sort((a, b) => want.get(a)! - want.get(b)! || cx(a) - cx(b))
    }
    pack()
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
