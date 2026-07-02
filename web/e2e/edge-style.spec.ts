import { test, expect, type Page, type Locator } from '@playwright/test'

// Clicking a requirement edge opens the context pad with three shape actions:
// eckig (right-angle, the default), gerundet (rounded corners) and direkt
// (straight line). Choosing one re-routes/re-renders that single edge — a
// straight two-point polyline for 'direkt', the same right-angle waypoints for
// 'eckig', and an eased <path> for 'gerundet'. Dragging the chain's middle node
// sideways gives its edges a lasting horizontal offset, so they keep bending
// through the re-route and the three shapes stay distinct.

// pointCount returns how many coordinate pairs the edge's own polyline carries
// (class dmn-edge, so the invisible hit path doesn't count), or 0 if the edge is
// currently drawn as a <path> (gerundet) instead.
async function pointCount(conn: Locator): Promise<number> {
  const poly = conn.locator('polyline.dmn-edge')
  if ((await poly.count()) === 0) return 0
  const pts = (await poly.first().getAttribute('points')) ?? ''
  return pts.trim().split(/\s+/).filter(Boolean).length
}

// bentIndex returns the index of the first connection whose polyline bends
// (>2 points), or -1 if none do.
async function bentIndex(page: Page): Promise<number> {
  return await page.locator('.djs-connection').evaluateAll((gs) => {
    for (let i = 0; i < gs.length; i++) {
      const poly = gs[i].querySelector('polyline.dmn-edge')
      if (!poly) continue
      const n = (poly.getAttribute('points') ?? '').trim().split(/\s+/).filter(Boolean).length
      if (n > 2) return i
    }
    return -1
  })
}

async function drag(page: Page, loc: Locator, dx: number): Promise<void> {
  const b = await loc.boundingBox()
  if (!b) throw new Error('no box')
  const cx = b.x + b.width / 2
  const cy = b.y + b.height / 2
  await page.mouse.move(cx, cy)
  await page.mouse.down()
  await page.mouse.move(cx + dx / 2, cy, { steps: 6 })
  await page.mouse.move(cx + dx, cy, { steps: 6 })
  await page.mouse.up()
}

// edgePoints samples screen points along the connection's line, keeps those
// clear of any node box or open context pad, and returns them ranked by how far
// they sit from the nearest such box — the roomiest first, so a click there is
// least likely to hit a shady node hit-area instead of the thin edge.
async function edgePoints(conn: Locator): Promise<{ x: number; y: number }[]> {
  return await conn.evaluate((g) => {
    const line = g.querySelector('polyline.dmn-edge') as SVGPolylineElement
    const svg = g.ownerSVGElement as SVGSVGElement
    const m = line.getScreenCTM() as DOMMatrix
    const pts = (line.getAttribute('points') ?? '').trim().split(/\s+/).map((p) => {
      const [x, y] = p.split(',').map(Number)
      const sp = svg.createSVGPoint()
      sp.x = x
      sp.y = y
      const o = sp.matrixTransform(m)
      return { x: o.x, y: o.y }
    })
    const rects = [...document.querySelectorAll('.djs-shape, .djs-context-pad')].map((s) => s.getBoundingClientRect())
    // clearance is how far (x,y) sits outside the nearest box; negative if inside.
    const clearance = (x: number, y: number): number => {
      let min = Infinity
      for (const r of rects) {
        const dx = Math.max(r.left - x, x - r.right, 0)
        const dy = Math.max(r.top - y, y - r.bottom, 0)
        const inside = x >= r.left && x <= r.right && y >= r.top && y <= r.bottom
        min = Math.min(min, inside ? -1 : Math.hypot(dx, dy))
      }
      return min
    }
    const cand: { x: number; y: number; c: number }[] = []
    for (let i = 1; i < pts.length; i++) {
      const a = pts[i - 1]
      const b = pts[i]
      for (const t of [0.5, 0.4, 0.6, 0.3, 0.7, 0.2, 0.8]) {
        const x = a.x + (b.x - a.x) * t
        const y = a.y + (b.y - a.y) * t
        const c = clearance(x, y)
        if (c > 0) cand.push({ x, y, c })
      }
    }
    return cand.sort((p, q) => q.c - p.c).map((p) => ({ x: p.x, y: p.y }))
  })
}

// selectEdge clicks candidate points on the edge in order of clearance until its
// context pad (with the eckig action) actually appears.
async function selectEdge(page: Page, conn: Locator): Promise<void> {
  const eckig = page.locator('.djs-context-pad [title="Eckige Verbindung"]')
  for (const p of await edgePoints(conn)) {
    await page.mouse.click(p.x, p.y)
    // Give the pad a moment to render before moving on to the next candidate.
    const shown = await eckig.waitFor({ state: 'visible', timeout: 500 }).then(() => true).catch(() => false)
    if (shown) return
  }
  throw new Error('could not select edge')
}

test('the edge context pad switches a requirement edge between eckig, gerundet and direkt', async ({ page }) => {
  await page.goto('/')
  await page.getByText('Alterskette (Demo)', { exact: true }).first().click()
  await expect(page.locator('.djs-palette')).toBeVisible()
  await expect(page.locator('.djs-connection').first()).toBeVisible()

  // Offset the middle "Category" node so its requirement edges keep bending.
  await drag(page, page.locator('.djs-element:has-text("Category")'), 60)
  await expect(async () => {
    expect(await bentIndex(page)).toBeGreaterThanOrEqual(0)
  }).toPass()

  const conn = page.locator('.djs-connection').nth(await bentIndex(page))
  await selectEdge(page, conn)

  // The context pad offers all three edge shapes.
  const pad = page.locator('.djs-context-pad')
  await expect(pad.locator('[title="Eckige Verbindung"]')).toBeVisible()
  await expect(pad.locator('[title="Gerundete Verbindung"]')).toBeVisible()
  await expect(pad.locator('[title="Direkte Verbindung"]')).toBeVisible()

  // Eckig (default) keeps the bend: a polyline with more than two points.
  expect(await pointCount(conn)).toBeGreaterThan(2)

  // Direkt collapses the edge to a straight two-point line. Selection (and thus
  // the pad) stays, so the next shape is one more click away.
  await pad.locator('[title="Direkte Verbindung"]').click()
  await expect(async () => {
    expect(await pointCount(conn)).toBe(2)
  }).toPass()

  // Gerundet re-routes with a bend and renders the edge as a single <path> —
  // no plain polyline any more.
  await pad.locator('[title="Gerundete Verbindung"]').click()
  await expect(conn.locator('path.dmn-edge')).toHaveCount(1)
  await expect(conn.locator('polyline.dmn-edge')).toHaveCount(0)

  // Eckig restores the bending right-angle polyline.
  await pad.locator('[title="Eckige Verbindung"]').click()
  await expect(async () => {
    expect(await pointCount(conn)).toBeGreaterThan(2)
  }).toPass()
})
