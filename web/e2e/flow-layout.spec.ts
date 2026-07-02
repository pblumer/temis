import { test, expect, type APIRequestContext } from '@playwright/test'
import { readFileSync } from 'fs'

// Flow autolayout regression (dagre): a flow whose inputs are shared across
// several steps must render as clean, well-separated layers — inputs at the
// bottom, the final decision on top — with no overlapping node boxes. Before the
// switch to dagre, shared sources piled every input into one row and the boxes
// could collide; these asserts guard the layered picture.

async function uploadModel(request: APIRequestContext, path: string): Promise<string> {
  const xml = readFileSync(path, 'utf8')
  const r = await request.post('/v1/models', { headers: { 'Content-Type': 'application/xml' }, data: xml })
  expect(r.ok(), `upload ${path}`).toBeTruthy()
  return ((await r.json()) as { modelId: string }).modelId
}

test('flows: a multi-source flow lays out in clean, collision-free layers', async ({ page, request }) => {
  const premiumId = await uploadModel(request, '../flow/testdata/premium.dmn')
  // Four steps over premium.dmn, with VehicleValue/RiskScore/RegionLoad shared as
  // flow inputs. RiskScore feeds two steps; base feeds two; final is the single
  // sink — a small kfz-antrag-shaped DAG with shared sources.
  const descriptor = {
    flow: 'kfz-antrag-layout',
    inputs: [
      { name: 'VehicleValue', type: 'number' },
      { name: 'RiskScore', type: 'number' },
      { name: 'RegionLoad', type: 'number' },
    ],
    steps: [
      { id: 'base', model: premiumId, decision: 'BasePremium', in: { VehicleValue: 'VehicleValue' } },
      { id: 'cat', model: premiumId, decision: 'RiskCategory', in: { RiskScore: 'RiskScore' } },
      { id: 'total', model: premiumId, decision: 'TotalRiskScore', in: { BasePremium: 'base.BasePremium', RiskCategory: 'cat.RiskCategory' } },
      { id: 'final', model: premiumId, decision: 'FinalPremium', in: { BasePremium: 'total.TotalRiskScore', RiskScore: 'RiskScore', RegionLoad: 'RegionLoad' } },
    ],
    output: { Premium: 'final.FinalPremium' },
  }
  const reg = await request.post('/v1/flows', { data: descriptor })
  expect(reg.ok(), 'register flow').toBeTruthy()

  await page.goto('/')
  await page.locator('.flow-item', { hasText: 'kfz-antrag-layout' }).click()
  const canvas = page.locator('#flowCanvas')
  await expect(canvas).toContainText('FinalPremium')
  await expect(canvas).toContainText('BasePremium')

  // Graph: 3 input nodes + 4 step nodes = 7 shapes; 7 requirement edges
  // (VehicleValue→base, RiskScore→cat, base→total, cat→total, total→final,
  // RiskScore→final, RegionLoad→final).
  await expect(page.locator('#flowCanvas .djs-shape')).toHaveCount(7)
  await expect(page.locator('#flowCanvas .djs-connection')).toHaveCount(7)

  // Read each shape's screen bounds, keyed by its graph-node id.
  const rects = await page.locator('#flowCanvas .djs-shape').evaluateAll((gs) =>
    gs.map((g) => {
      const r = g.getBoundingClientRect()
      return { id: g.getAttribute('data-element-id') ?? '', x: r.x, y: r.y, w: r.width, h: r.height }
    }),
  )
  const by = Object.fromEntries(rects.map((r) => [r.id, r]))
  const inputs = ['in:VehicleValue', 'in:RiskScore', 'in:RegionLoad']
  const final = by['final']
  expect(final, 'final decision rendered').toBeTruthy()

  // Rank separation: the final decision sits fully above every flow input (smaller
  // y in diagram-js' y-down screen coordinates), so the layers read top → bottom.
  for (const id of inputs) {
    expect(by[id], `input ${id} rendered`).toBeTruthy()
    expect(final.y + final.h, `final above input ${id}`).toBeLessThanOrEqual(by[id].y)
  }

  // Collision-free: no two node boxes overlap (a couple of px of stroke slack).
  for (let i = 0; i < rects.length; i++) {
    for (let j = i + 1; j < rects.length; j++) {
      const a = rects[i]
      const b = rects[j]
      const overlapX = Math.min(a.x + a.w, b.x + b.w) - Math.max(a.x, b.x)
      const overlapY = Math.min(a.y + a.h, b.y + b.h) - Math.max(a.y, b.y)
      expect(overlapX > 2 && overlapY > 2, `${a.id} overlaps ${b.id}`).toBeFalsy()
    }
  }
})
