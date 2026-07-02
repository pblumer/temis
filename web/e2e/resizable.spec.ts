import { test, expect, type APIRequestContext } from '@playwright/test'
import { readFileSync } from 'fs'

// Resizable panels (src/resizable.ts): the left sidebar (model/flow catalog) and
// the flow-designer's right inspector both sit at a fixed width by default and can
// be widened/narrowed by dragging their divider, with the width remembered per
// browser. This drives both dividers and asserts the panel width actually changes.

async function uploadModel(request: APIRequestContext, path: string): Promise<string> {
  const xml = readFileSync(path, 'utf8')
  const r = await request.post('/v1/models', { headers: { 'Content-Type': 'application/xml' }, data: xml })
  expect(r.ok(), `upload ${path}`).toBeTruthy()
  return ((await r.json()) as { modelId: string }).modelId
}

// dragBy grabs an element at its centre and moves the pointer by (dx, dy).
async function dragBy(page: import('@playwright/test').Page, selector: string, dx: number, dy = 0): Promise<void> {
  const box = await page.locator(selector).boundingBox()
  if (!box) throw new Error(`no bounding box for ${selector}`)
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2)
  await page.mouse.down()
  await page.mouse.move(box.x + box.width / 2 + dx, box.y + box.height / 2 + dy, { steps: 8 })
  await page.mouse.up()
}

const widthOf = async (page: import('@playwright/test').Page, sel: string): Promise<number> => (await page.locator(sel).boundingBox())!.width

test('sidebar divider resizes the sidebar and the width persists', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('#modelList .model-item').first()).toBeVisible()

  const before = await widthOf(page, '.sidebar')
  await dragBy(page, '#sidebarResizer', 120)
  const after = await widthOf(page, '.sidebar')
  expect(after).toBeGreaterThan(before + 80)

  // The width is remembered across a reload (localStorage).
  await page.reload()
  await expect(page.locator('#modelList .model-item').first()).toBeVisible()
  expect(Math.abs((await widthOf(page, '.sidebar')) - after)).toBeLessThan(4)

  // Double-clicking the divider restores the default width.
  await page.locator('#sidebarResizer').dblclick()
  expect(Math.abs((await widthOf(page, '.sidebar')) - before)).toBeLessThan(4)
})

test('flow-designer inspector divider resizes the inspector', async ({ page, request }) => {
  await uploadModel(request, '../flow/testdata/risk.dmn')
  await page.goto('/')
  await expect(page.locator('#modelList .model-item').first()).toBeVisible()

  await page.locator('#newFlow').click()
  await expect(page.locator('.flow-editor')).toBeVisible()

  const before = await widthOf(page, '.flow-editor-inspector')
  // The inspector sits to the RIGHT of its divider, so dragging left grows it.
  await dragBy(page, '#feInspectorResizer', -120)
  const after = await widthOf(page, '.flow-editor-inspector')
  expect(after).toBeGreaterThan(before + 80)

  await page.locator('#feInspectorResizer').dblclick()
  expect(Math.abs((await widthOf(page, '.flow-editor-inspector')) - before)).toBeLessThan(4)
})
