import type { Graph } from './api'

// A small top-down layered layout for a DRG (ADR-0016, WP-65): there is no DMNDI
// in the /v1 graph yet, so positions are computed. Each node's row is the length
// of its longest requirement chain; row 0 (pure inputs/leaves) sits at the
// bottom, decisions stack upward — the conventional DMN reading direction.

export type LaidNode = { id: string; type: string; name: string; x: number; y: number; w: number; h: number }
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

export function layout(graph: Graph): Laid {
  const sources = new Map<string, string[]>() // node -> the nodes it requires
  for (const n of graph.nodes) sources.set(n.id, [])
  for (const e of graph.edges) sources.get(e.target)?.push(e.source)

  const memo = new Map<string, number>()
  const rowOf = (id: string, seen: Set<string>): number => {
    const cached = memo.get(id)
    if (cached !== undefined) return cached
    if (seen.has(id)) return 0 // cycle guard (the engine forbids cycles, but be safe)
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

  const pos = new Map<string, LaidNode>()
  const nodes: LaidNode[] = []
  for (let r = 0; r <= maxRow; r++) {
    const bucket = rows.get(r) ?? []
    let x = PAD + (maxWidth - rowWidth(r)) / 2
    const y = PAD + (maxRow - r) * (ROW_GAP + 70)
    for (const n of bucket) {
      const s = sizeOf(n.type)
      const node: LaidNode = { id: n.id, type: n.type, name: n.name, x, y, w: s.w, h: s.h }
      nodes.push(node)
      pos.set(n.id, node)
      x += s.w + COL_GAP
    }
  }

  const edges: LaidEdge[] = []
  graph.edges.forEach((e, i) => {
    const s = pos.get(e.source)
    const t = pos.get(e.target)
    if (!s || !t) return
    edges.push({
      id: 'edge' + i,
      type: e.type,
      source: e.source,
      target: e.target,
      waypoints: [
        { x: s.x + s.w / 2, y: s.y }, // source top-center
        { x: t.x + t.w / 2, y: t.y + t.h }, // target bottom-center
      ],
    })
  })

  return { nodes, edges }
}
