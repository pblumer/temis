// Orthogonal (Manhattan) routing for requirement edges in the auto-laid DRD
// (ADR-0016). The default layouter docks edges as straight diagonals border-to-
// border; on a top-down/bottom-up layered layout that reads as spaghetti. This
// routes every edge as a right-angle connector: it leaves the required element
// on the face pointing at the requiring one, runs the cross move through the
// empty band next to the target, and enters the target head-on — so a hub's
// inputs merge into a clean comb rather than a fan of crossing diagonals.

export type Pt = { x: number; y: number }
export type Box = { x: number; y: number; w: number; h: number }

// How far into the inter-row gap the cross (horizontal) segment sits, measured
// from the target's docking face. Kept in the empty band next to the target so
// the perpendicular run never cuts across a node; capped to a fraction of the
// available gap so it can't invert when nodes sit close together.
const BAND = 26

// orthoRoute routes a requirement edge from s (the required element) to t (the
// requiring element, where the arrow lands). exitX/entryX override the docking
// x on s's and t's faces so several edges sharing a node can fan out instead of
// stacking on one point; both default to the node's centre. The layered layout
// is vertical, so edges dock on the top/bottom faces and the arrow always
// enters t head-on.
export function orthoRoute(s: Box, t: Box, exitX?: number, entryX?: number): Pt[] {
  const ex = exitX ?? s.x + s.w / 2
  const nx = entryX ?? t.x + t.w / 2
  // t sits above s (bottom-up) or below it (top-down); dock on the facing edges.
  const tAbove = t.y + t.h / 2 < s.y + s.h / 2
  const E: Pt = { x: ex, y: tAbove ? s.y : s.y + s.h }
  const N: Pt = { x: nx, y: tAbove ? t.y + t.h : t.y }
  if (Math.abs(ex - nx) < 1) return [E, N]
  const gap = Math.abs(E.y - N.y)
  const band = Math.min(BAND, gap * 0.4)
  const my = tAbove ? N.y + band : N.y - band
  return [E, { x: ex, y: my }, { x: nx, y: my }, N]
}
