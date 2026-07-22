import type { Graph, GraphNode } from './api'
import { orthoRoute } from './ortho'
import dagre from '@dagrejs/dagre'

// Positioning a DRG for the canvas (ADR-0016, WP-65). Three paths:
//   - authored DMNDI → use the engine's bounds verbatim (hasLayout).
//   - plain auto-layout (the flow views, no opts) → dagre layered layout: dagre
//     assigns the ranks, minimizes crossings and routes the edges in one pass, so
//     flows with shared inputs read as clean, well-separated layers with bent
//     waypoints instead of a pile of overlapping straight lines (WP-97/98). See
//     dagreLayout for the rankdir/direction convention.
//   - ortho auto-layout (the DRD modeler, opts.ortho) → dagre positions the nodes
//     (same crossing-minimized layering as the flow views), then the orthogonal
//     (right-angle) edge router below re-routes every edge over those positions —
//     fanning a hub's inputs into a clean comb and threading skip edges through
//     clear lanes. opts.orientation flips whether inputs feed decisions from below
//     (bottom-up, dagre rankdir 'BT') or above (top-down, 'TB'). Adopting dagre
//     here retired the hand-rolled barycentre layout the modeler used before — it
//     piled long skip edges into overlapping columns; dagre keeps the layers apart.

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
const PAD = 24
// Vertical gap dagre keeps between rank boundaries (ranksep is the clear gap
// between rows, so the row centre-to-centre distance is RANK_GAP + node height).
const RANK_GAP = 90

// toLaidNode copies a graph node's render-driving fields into a LaidNode, taking
// its position/size from the layout. Every path (DMNDI, dagre, legacy) shares it
// so the icon-steering flags (hasTable, dataType, …) are never dropped.
function toLaidNode(n: GraphNode, x: number, y: number, w: number, h: number): LaidNode {
  return { id: n.id, type: n.type, name: n.name, dataType: n.dataType, varName: n.varName, hasTable: n.hasTable, hasLiteral: n.hasLiteral, hasContext: n.hasContext, hasConditional: n.hasConditional, hasList: n.hasList, hasRelation: n.hasRelation, hasFilter: n.hasFilter, hasIterator: n.hasIterator, hasInvocation: n.hasInvocation, hasLogic: n.hasLogic, x, y, w, h }
}

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

// dagreLayout positions a graph with dagre. Requirement edges run "target
// requires source" (leaf inputs are the sources); feeding dagre source→target
// puts the sources at rank 0. rankdir 'BT' (bottom-up) then places rank 0 at the
// bottom with the requiring decisions above them; 'TB' (top-down) flips it, inputs
// on top. y grows downward in both dagre's output and diagram-js, so positions map
// straight across (no inversion). Nodes/edges are fed in stable input order, and
// dagre is deterministic on fixed input, so the same graph always yields the same
// layout. When edgeWaypoints is given, it is filled per edge index with dagre's
// routed polyline (the flow views' path); when null, dagre positions the nodes
// only and the caller re-routes the edges itself (the modeler's ortho path).
function dagreLayout(
  graph: Graph,
  pos: Map<string, LaidNode>,
  edgeWaypoints: Map<number, { x: number; y: number }[]> | null,
  rankdir: 'BT' | 'TB' = 'BT',
): void {
  const byId = new Map(graph.nodes.map((n) => [n.id, n]))
  const g = new dagre.graphlib.Graph({ multigraph: true })
  g.setGraph({ rankdir, nodesep: COL_GAP, ranksep: RANK_GAP, marginx: PAD, marginy: PAD })
  g.setDefaultEdgeLabel(() => ({}))
  for (const n of graph.nodes) {
    const s = sizeOf(n.type)
    g.setNode(n.id, { width: s.w, height: s.h })
  }
  // Edge name = index so each graph edge's routed points are retrievable by index,
  // even if two edges ever shared endpoints (multigraph).
  graph.edges.forEach((e, i) => {
    if (byId.has(e.source) && byId.has(e.target)) g.setEdge(e.source, e.target, {}, String(i))
  })
  dagre.layout(g)
  for (const n of graph.nodes) {
    const nd = g.node(n.id)
    const s = sizeOf(n.type)
    // dagre reports node centres; Laid wants the top-left corner.
    pos.set(n.id, toLaidNode(n, nd.x - s.w / 2, nd.y - s.h / 2, s.w, s.h))
  }
  if (!edgeWaypoints) return
  graph.edges.forEach((e, i) => {
    if (!byId.has(e.source) || !byId.has(e.target)) return
    const ge = g.edge(e.source, e.target, String(i)) as { points?: { x: number; y: number }[] } | undefined
    if (ge?.points?.length) edgeWaypoints.set(i, ge.points.map((p) => ({ x: p.x, y: p.y })))
  })
}

// dockDagre re-anchors a dagre-routed edge's endpoints onto the node borders
// (dagre already clips to the box, but borderPoint keeps docking consistent with
// the other paths and with how flow-canvas anchors the value labels). The first
// point aims at the second and the last at the second-to-last, so the bends dagre
// found in between are preserved.
function dockDagre(s: LaidNode, t: LaidNode, pts: { x: number; y: number }[]): { x: number; y: number }[] {
  const first = borderPoint(s, pts[1].x, pts[1].y)
  const last = borderPoint(t, pts[pts.length - 2].x, pts[pts.length - 2].y)
  return dedupePoints([first, ...pts.slice(1, -1), last])
}

export function layout(graph: Graph, opts: LayoutOpts = {}): Laid {
  const pos = new Map<string, LaidNode>()
  // Filled by dagreLayout only (edgeIndex → routed polyline); empty otherwise.
  const dagreEdges = new Map<number, { x: number; y: number }[]>()

  // Use authored DMNDI bounds when every node has them (unless the caller forces
  // a re-arrange), else auto-layout with dagre: the flow views take dagre's routed
  // edges, the modeler's ortho path takes dagre's node positions only (edgeWaypoints
  // = null) and re-routes the edges orthogonally below. opts.orientation picks the
  // rank direction — bottom-up ('BT', inputs at the bottom) or top-down ('TB').
  const hasLayout =
    graph.nodes.length > 0 &&
    graph.nodes.every((n) => (n.width ?? 0) > 0 && (n.height ?? 0) > 0)
  const auto = !hasLayout || opts.forceAuto === true
  if (!auto) {
    for (const n of graph.nodes) {
      pos.set(n.id, toLaidNode(n, n.x ?? 0, n.y ?? 0, n.width ?? 0, n.height ?? 0))
    }
  } else if (opts.ortho) {
    dagreLayout(graph, pos, null, (opts.orientation ?? 'bottomUp') === 'topDown' ? 'TB' : 'BT')
  } else {
    dagreLayout(graph, pos, dagreEdges)
  }

  // Orthogonal routing (modeler only, and only for an auto-arranged graph):
  // fan each node's edges across its docking face so a hub's inputs merge into a
  // clean comb instead of piling onto one point, then route each as a right-angle
  // connector. The flow views use dagre's routed waypoints; authored (DMNDI)
  // layouts fall back to the diagonal border-to-border line.
  const ortho = opts.ortho === true && auto
  const dockX = ortho ? fanDockX(graph, pos) : null
  const allNodes = [...pos.values()]

  const edges: LaidEdge[] = []
  graph.edges.forEach((e, i) => {
    const s = pos.get(e.source)
    const t = pos.get(e.target)
    if (!s || !t) return
    let waypoints: { x: number; y: number }[]
    const dpts = dagreEdges.get(i)
    if (dpts && dpts.length >= 2) {
      waypoints = dockDagre(s, t, dpts)
    } else if (dockX) {
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
