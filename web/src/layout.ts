import type { Graph } from './api'
import { orthoRoute } from './ortho'

// Positioning a DRG for the canvas (ADR-0016, WP-65). When the model carries
// DMNDI, the engine hands us the authored bounds and we use them verbatim;
// otherwise we compute a simple layered layout (row = longest requirement
// chain). Requirement edges are routed border-to-border between the node boxes
// by default, so they work for any positions.
//
// The modeler opts into a nicer picture (opts.ortho): the auto-layout then
// aligns nodes into columns and routes edges as orthogonal (right-angle)
// connectors instead of diagonals, and opts.orientation flips whether inputs
// feed decisions from below (bottom-up, the default) or from above (top-down).
// Callers that pass no opts (e.g. the decision-flow canvas) get the original
// straight-line behaviour unchanged.

// LayoutOpts tunes the auto-layout for the DRD modeler; omitting it keeps the
// legacy straight-diagonal, bottom-up, DMNDI-verbatim behaviour.
export type Orientation = 'bottomUp' | 'topDown'
export type LayoutOpts = {
  // orientation: 'bottomUp' puts leaf inputs at the bottom with arrows pointing
  // up into decisions (default); 'topDown' flips it — inputs on top.
  orientation?: Orientation
  // ortho routes edges as right-angle connectors and aligns nodes into columns.
  ortho?: boolean
  // forceAuto ignores authored DMNDI bounds and always re-arranges (the "arrange
  // top-down / bottom-up" toolbar action).
  forceAuto?: boolean
}

export type LaidNode = { id: string; type: string; name: string; dataType?: string; varName?: string; hasTable?: boolean; hasLiteral?: boolean; hasContext?: boolean; hasConditional?: boolean; hasList?: boolean; hasRelation?: boolean; hasFilter?: boolean; hasIterator?: boolean; hasInvocation?: boolean; hasLogic?: boolean; x: number; y: number; w: number; h: number }
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

function autoLayout(graph: Graph, pos: Map<string, LaidNode>, opts: LayoutOpts = {}): void {
  const orientation = opts.orientation ?? 'bottomUp'
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

  // yOf places row r vertically. Bottom-up (default) keeps leaf inputs at the
  // bottom (row 0) with decisions stacking upward; top-down flips it so inputs
  // sit on top and decisions grow downward.
  const stepY = ROW_GAP + 70
  const yOf = (r: number): number => PAD + (orientation === 'topDown' ? r : maxRow - r) * stepY

  // pack lays out each row left-to-right in its current order, centred.
  const pack = (): void => {
    for (const [r, ids] of rowIds) {
      let x = PAD + (maxWidth - rowWidth(ids)) / 2
      const y = yOf(r)
      for (const id of ids) {
        const n = byId.get(id)!
        const s = sizeOf(n.type)
        pos.set(id, { id, type: n.type, name: n.name, dataType: n.dataType, varName: n.varName, hasTable: n.hasTable, hasLiteral: n.hasLiteral, hasContext: n.hasContext, hasConditional: n.hasConditional, hasList: n.hasList, hasRelation: n.hasRelation, hasFilter: n.hasFilter, hasIterator: n.hasIterator, hasInvocation: n.hasInvocation, hasLogic: n.hasLogic, x, y, w: s.w, h: s.h })
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

  // With edges routed orthogonally, straight vertical connectors read best, so
  // pull each node toward the horizontal centre of the nodes it connects to —
  // without changing its in-row order or letting it overlap its row neighbours.
  // A node with a single parent/child ends up directly under/over it (a straight
  // edge); a hub settles above the centre of its inputs. Gauss-Seidel style: a
  // few sweeps, alternating direction, using neighbours' current positions.
  if (opts.ortho) {
    const centre = (id: string): number => {
      const p = pos.get(id)!
      return p.x + p.w / 2
    }
    const rows = [...rowIds.values()]
    for (let round = 0; round < 16; round++) {
      const order = round % 2 ? [...rows].reverse() : rows
      for (const ids of order) {
        for (let i = 0; i < ids.length; i++) {
          const p = pos.get(ids[i])!
          const nb = [...(req.get(ids[i]) ?? []), ...(reqBy.get(ids[i]) ?? [])]
          let want = nb.length ? nb.reduce((a, n) => a + centre(n), 0) / nb.length : centre(ids[i])
          // Clamp between the in-row neighbours so order and spacing hold.
          if (i > 0) {
            const left = pos.get(ids[i - 1])!
            want = Math.max(want, left.x + left.w + COL_GAP + p.w / 2)
          }
          if (i < ids.length - 1) {
            const right = pos.get(ids[i + 1])!
            want = Math.min(want, right.x - COL_GAP - p.w / 2)
          }
          p.x = want - p.w / 2
        }
      }
    }
  }
}

export function layout(graph: Graph, opts: LayoutOpts = {}): Laid {
  const pos = new Map<string, LaidNode>()

  // Use authored DMNDI bounds when every node has them (unless the caller forces
  // a re-arrange), else auto-layout.
  const hasLayout =
    graph.nodes.length > 0 &&
    graph.nodes.every((n) => (n.width ?? 0) > 0 && (n.height ?? 0) > 0)
  const auto = !hasLayout || opts.forceAuto === true
  if (!auto) {
    for (const n of graph.nodes) {
      pos.set(n.id, { id: n.id, type: n.type, name: n.name, dataType: n.dataType, varName: n.varName, hasTable: n.hasTable, hasLiteral: n.hasLiteral, hasContext: n.hasContext, hasConditional: n.hasConditional, hasList: n.hasList, hasRelation: n.hasRelation, hasFilter: n.hasFilter, hasIterator: n.hasIterator, hasInvocation: n.hasInvocation, hasLogic: n.hasLogic, x: n.x ?? 0, y: n.y ?? 0, w: n.width ?? 0, h: n.height ?? 0 })
    }
  } else {
    autoLayout(graph, pos, opts)
  }

  // Orthogonal routing (modeler only, and only for an auto-arranged graph):
  // fan each node's edges across its docking face so a hub's inputs merge into a
  // clean comb instead of piling onto one point, then route each as a right-angle
  // connector. Authored (DMNDI) layouts and no-opts callers keep the diagonal
  // border-to-border line, which works for any node placement.
  const ortho = opts.ortho === true && auto
  const dockX = ortho ? fanDockX(graph, pos) : null
  const allNodes = [...pos.values()]

  const edges: LaidEdge[] = []
  graph.edges.forEach((e, i) => {
    const s = pos.get(e.source)
    const t = pos.get(e.target)
    if (!s || !t) return
    let waypoints: { x: number; y: number }[]
    if (dockX) {
      waypoints = routeAuto(s, t, dockX.exit.get(i) ?? s.x + s.w / 2, dockX.entry.get(i) ?? t.x + t.w / 2, allNodes)
    } else {
      const sc = { x: s.x + s.w / 2, y: s.y + s.h / 2 }
      const tc = { x: t.x + t.w / 2, y: t.y + t.h / 2 }
      waypoints = [borderPoint(s, tc.x, tc.y), borderPoint(t, sc.x, sc.y)]
    }
    edges.push({ id: 'edge' + i, type: e.type, source: e.source, target: e.target, waypoints })
  })

  return { nodes: [...pos.values()], edges }
}

// fanDockX spreads the docking x-coordinate of each edge across its endpoints'
// faces. Edges leaving one source (or entering one target) are ordered by the
// horizontal position of their other end and placed at evenly spaced points
// along the node's width, so parallel connectors don't overlap and don't cross
// as they fan out. Returns, per edge index, the exit x (on the source) and the
// entry x (on the target).
function fanDockX(graph: Graph, pos: Map<string, LaidNode>): { exit: Map<number, number>; entry: Map<number, number> } {
  const outgoing = new Map<string, number[]>()
  const incoming = new Map<string, number[]>()
  graph.edges.forEach((e, i) => {
    if (!pos.has(e.source) || !pos.has(e.target)) return
    ;(outgoing.get(e.source) ?? outgoing.set(e.source, []).get(e.source)!).push(i)
    ;(incoming.get(e.target) ?? incoming.set(e.target, []).get(e.target)!).push(i)
  })
  const centre = (id: string): number => {
    const p = pos.get(id)!
    return p.x + p.w / 2
  }
  const exit = new Map<number, number>()
  const entry = new Map<number, number>()
  // Spread k edges across a node of width w: fractions 1/(k+1) … k/(k+1), so a
  // single edge stays centred and the outermost never sits on a corner.
  const spread = (nodeId: string, idxs: number[], otherOf: (i: number) => string, out: Map<number, number>): void => {
    const n = pos.get(nodeId)!
    const ordered = [...idxs].sort((a, b) => centre(otherOf(a)) - centre(otherOf(b)))
    ordered.forEach((i, k) => out.set(i, n.x + (n.w * (k + 1)) / (ordered.length + 1)))
  }
  for (const [id, idxs] of outgoing) spread(id, idxs, (i) => graph.edges[i].target, exit)
  for (const [id, idxs] of incoming) spread(id, idxs, (i) => graph.edges[i].source, entry)
  return { exit, entry }
}

// How far into the inter-row gap a threaded skip edge's turn sits, and the
// clearance kept on either side of an obstacle when picking its vertical lane.
const TURN_BAND = 26
const LANE_MARGIN = 14

// routeAuto routes one requirement edge for the auto-layout. Edges between
// adjacent rows are a plain orthogonal connector (a clean comb into the target).
// A skip edge — one whose straight vertical would cross a node sitting between
// its endpoints — is threaded instead: it leaves the source, jogs into the gap
// band, runs its long vertical down a clear lane *between* the intervening nodes'
// columns, then jogs into the target. So a long edge never cuts through a box.
function routeAuto(s: LaidNode, t: LaidNode, exitX: number, entryX: number, nodes: LaidNode[]): { x: number; y: number }[] {
  const tAbove = t.y + t.h / 2 < s.y + s.h / 2
  const sFaceY = tAbove ? s.y : s.y + s.h
  const tFaceY = tAbove ? t.y + t.h : t.y
  const lo = Math.min(sFaceY, tFaceY)
  const hi = Math.max(sFaceY, tFaceY)

  // Nodes whose vertical band sits strictly between the two faces are obstacles
  // the long vertical must dodge (endpoints and same-row siblings are excluded,
  // their bands touching but not crossing the corridor).
  const between = nodes.filter((n) => n.id !== s.id && n.id !== t.id && n.y + n.h > lo + 1 && n.y < hi - 1)
  if (!between.length) return orthoRoute(s, t, exitX, entryX)

  // Turn just inside the gap next to each face (guaranteed clear: rows are spaced
  // wider than TURN_BAND), then choose a lane x clear of every obstacle column.
  const yS = tAbove ? sFaceY - TURN_BAND : sFaceY + TURN_BAND
  const yT = tAbove ? tFaceY + TURN_BAND : tFaceY - TURN_BAND
  const ints = between.map((n) => [n.x - LANE_MARGIN, n.x + n.w + LANE_MARGIN] as const)
  const clear = (x: number): boolean => ints.every(([a, b]) => x < a || x > b)
  const cands = [exitX, entryX, ...ints.flatMap(([a, b]) => [a, b])]
  cands.push(Math.min(...ints.map((i) => i[0])) - LANE_MARGIN, Math.max(...ints.map((i) => i[1])) + LANE_MARGIN)
  const cost = (x: number): number => Math.abs(x - exitX) + Math.abs(x - entryX)
  const feasible = cands.filter(clear).sort((a, b) => cost(a) - cost(b))
  const laneX = feasible.length ? feasible[0] : Math.max(...ints.map((i) => i[1])) + LANE_MARGIN

  return dedupePoints([
    { x: exitX, y: sFaceY },
    { x: exitX, y: yS },
    { x: laneX, y: yS },
    { x: laneX, y: yT },
    { x: entryX, y: yT },
    { x: entryX, y: tFaceY },
  ])
}

// dedupePoints drops coincident points and collapses three collinear points into
// two, so a threaded route whose lane happens to line up with a dock face comes
// out as a clean straight run rather than a zero-length dogleg.
function dedupePoints(pts: { x: number; y: number }[]): { x: number; y: number }[] {
  const out: { x: number; y: number }[] = []
  for (const p of pts) {
    const last = out[out.length - 1]
    if (last && Math.abs(last.x - p.x) < 0.5 && Math.abs(last.y - p.y) < 0.5) continue
    out.push(p)
  }
  for (let i = 1; i < out.length - 1; ) {
    const a = out[i - 1]
    const b = out[i]
    const c = out[i + 1]
    const collinear = (Math.abs(a.x - b.x) < 0.5 && Math.abs(b.x - c.x) < 0.5) || (Math.abs(a.y - b.y) < 0.5 && Math.abs(b.y - c.y) < 0.5)
    if (collinear) out.splice(i, 1)
    else i++
  }
  return out
}
