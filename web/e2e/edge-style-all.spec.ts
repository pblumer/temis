import { test, expect, type Page } from '@playwright/test'

// The toolbar's "Verbindungen" selector sets the shape of EVERY requirement edge
// at once — eckig (right-angle polyline), gerundet (rounded <path>) or direkt
// (straight two-point polyline) — as a single undoable step. The per-edge context
// pad (edge-style.spec.ts) can still override an individual edge afterwards. The
// Discount demo is a hub (the Discount decision requires two inputs) and carries
// no authored DMNDI, so it auto-lays-out and its fanned edges bend under the ortho
// routing — the three shapes stay distinguishable without dragging anything.

// straightCount / bentCount / pathCount tally, across all edges, how the edge's
// own line (class dmn-edge, so the invisible hit path is ignored) is drawn.
async function tally(page: Page): Promise<{ straight: number; bent: number; path: number }> {
  return await page.locator('.djs-connection').evaluateAll((gs) => {
    let straight = 0
    let bent = 0
    let path = 0
    for (const g of gs) {
      const poly = g.querySelector('polyline.dmn-edge')
      if (poly) {
        const n = (poly.getAttribute('points') ?? '').trim().split(/\s+/).filter(Boolean).length
        if (n > 2) bent++
        else straight++
      } else if (g.querySelector('path.dmn-edge')) {
        path++
      }
    }
    return { straight, bent, path }
  })
}

test('the Verbindungen selector sets every requirement edge to eckig, gerundet or direkt at once', async ({ page }) => {
  await page.goto('/')
  await page.getByText('Discount', { exact: true }).first().click()
  await expect(page.locator('.djs-palette')).toBeVisible()
  await expect(page.locator('.djs-connection').first()).toBeVisible()

  const select = page.locator('#edgeStyle')
  await expect(select).toBeVisible()

  const total = await page.locator('.djs-connection').count()
  expect(total).toBeGreaterThan(0)

  // The hub's fanned edges bend under the default eckig routing.
  await expect(async () => {
    expect((await tally(page)).bent).toBeGreaterThan(0)
  }).toPass()

  // Direkt collapses every edge to a straight two-point polyline — no bends, no
  // rounded paths anywhere.
  await select.selectOption('direct')
  await expect(async () => {
    const t = await tally(page)
    expect(t.bent).toBe(0)
    expect(t.path).toBe(0)
    expect(t.straight).toBe(total)
  }).toPass()

  // Gerundet redraws the (re-routed, bending) edges as rounded <path>s — at least
  // the fanned hub edges, which are no longer plain polylines.
  await select.selectOption('curved')
  await expect(async () => {
    expect((await tally(page)).path).toBeGreaterThan(0)
  }).toPass()

  // Eckig restores the right-angle polylines: bends are back, no paths remain.
  await select.selectOption('ortho')
  await expect(async () => {
    const t = await tally(page)
    expect(t.path).toBe(0)
    expect(t.bent).toBeGreaterThan(0)
  }).toPass()

  // The batch is one undoable step: a single undo reverts the whole set back to
  // the gerundet <path>s.
  await page.keyboard.press('Control+z')
  await expect(async () => {
    expect((await tally(page)).path).toBeGreaterThan(0)
  }).toPass()
})
