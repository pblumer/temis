// The DMN palette (ADR-0016): creating elements from the left toolbar. Guards
// the create gesture against the ghost click that trails a native drag (which
// used to leave a phantom element stuck to the cursor) and gives new elements
// unique default names so two "Neue Decision" nodes never silently collide.
import { test, expect } from '@playwright/test'

const active = (page: import('@playwright/test').Page) =>
  page.evaluate(() => !!document.querySelector('.djs-drag-active, .djs-dragging, .djs-drag-group'))

// box returns a locator's bounding box, retrying until it is available — under
// heavy parallel load the shared dev server can re-render the canvas mid-test, so
// a single boundingBox() call occasionally returns null.
async function box(locator: import('@playwright/test').Locator): Promise<{ x: number; y: number; width: number; height: number }> {
  await expect(locator).toBeVisible()
  for (let i = 0; i < 20; i++) {
    const b = await locator.boundingBox()
    if (b) return b
    await locator.page().waitForTimeout(50)
  }
  throw new Error('no bounding box')
}

// openModeler opens the Discount example and waits until the diagram has actually
// rendered — the modeler destroys and rebuilds the container on load, so the
// palette can be "visible" while `.djs-container` is momentarily detached and
// reports a null bounding box.
async function openModeler(page: import('@playwright/test').Page): Promise<void> {
  await page.goto('/')
  await page.getByText('Discount', { exact: true }).first().click()
  await expect(page.locator('.djs-palette')).toBeVisible()
  await expect(page.locator('.djs-element[data-element-id]').first()).toBeVisible()
}

// The trailing "ghost" click a browser fires after a canceled native drag from
// the palette used to start a second, phantom create session that stuck to the
// cursor (only Esc/reload cleared it). The click action must ignore it.
test('palette: ghost click after a drag does not leave a stuck element', async ({ page }) => {
  await openModeler(page)

  const entry = page.locator('.djs-palette [data-action="create-decision"]')
  const cbox = await box(page.locator('.djs-container'))
  const target = { x: cbox.x + cbox.width / 2, y: cbox.y + cbox.height / 2 }
  const from = await box(entry)

  await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2)
  await page.mouse.down()
  await entry.dispatchEvent('dragstart', {})
  await page.mouse.move(target.x, target.y, { steps: 8 })
  await page.mouse.up()
  // Ghost click on the source, as a browser emits after the prevented DnD.
  await entry.dispatchEvent('click', {})
  await page.mouse.move(target.x + 90, target.y + 40, { steps: 4 })

  expect(await active(page)).toBe(false)
})

// Two decisions created back-to-back must get distinct names ("Neue Decision",
// "Neue Decision 2") rather than silently colliding.
test('palette: created decisions get unique names', async ({ page }) => {
  await openModeler(page)

  const entry = page.locator('.djs-palette [data-action="create-decision"]')
  const canvas = page.locator('.djs-container')

  const drop = async (dx: number, dy: number): Promise<void> => {
    const cbox = await box(canvas)
    const from = await box(entry)
    const t = { x: cbox.x + cbox.width / 2 + dx, y: cbox.y + cbox.height / 2 + dy }
    await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2)
    await page.mouse.down()
    await entry.dispatchEvent('dragstart', {})
    await page.mouse.move(t.x, t.y, { steps: 8 })
    await page.mouse.up()
    await page.mouse.move(t.x + 60, t.y + 40, { steps: 3 })
  }

  await drop(-160, -70)
  await drop(160, 70)

  await expect(page.locator('.djs-element:has-text("Neue Decision")')).toHaveCount(2)
  await expect(page.locator('.djs-element:has-text("Neue Decision 2")')).toHaveCount(1)
})

// A palette click creates immediately at the visible canvas center; it must not
// leave a click-to-place preview stuck to the cursor.
test('palette: clicking a create tool immediately creates a real element', async ({ page }) => {
  await openModeler(page)
  const before = await page.locator('.djs-element[data-element-id]').count()

  const entry = page.locator('.djs-palette [data-action="create-decision"]')
  await entry.click()

  expect(await page.locator('.djs-element[data-element-id]').count()).toBe(before + 1)
  expect(await active(page)).toBe(false)
})

// If a listener reacting to the freshly created shape throws, that exception used
// to escape diagram-js' create.end — its cleanup never ran, so the new element
// stayed glued to the cursor and could only be dismissed with Esc/reload. The
// palette's error boundary must catch it (logging it, so it stays debuggable),
// let the create finish (element placed) and free the cursor.
test('palette: a throwing create-time listener must not strand the cursor', async ({ page }) => {
  await page.addInitScript(() => { (window as unknown as { __E2E__: boolean }).__E2E__ = true })
  const uncaught: string[] = []
  const consoleErrors: string[] = []
  page.on('pageerror', (e) => uncaught.push(String(e.message)))
  page.on('console', (m) => { if (m.type() === 'error') consoleErrors.push(m.text()) })

  await openModeler(page)

  // Simulate the model-specific exception: a handler that throws on create.end.
  await page.evaluate(() => {
    const d = (window as unknown as { __diagram: { get: (n: string) => { on: (e: string, p: number, cb: () => void) => void } } }).__diagram
    d.get('eventBus').on('create.end', 2000, () => { throw new Error('boom in create.end') })
  })

  const before = await page.locator('.djs-element[data-element-id]').count()
  const entry = page.locator('.djs-palette [data-action="create-decision"]')
  const cbox = await box(page.locator('.djs-container'))
  const from = await box(entry)
  const t = { x: cbox.x + cbox.width * 0.4, y: cbox.y + cbox.height * 0.7 }

  await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2)
  await page.mouse.down()
  await page.mouse.move(from.x + 18, from.y + 18, { steps: 4 })
  await page.mouse.move(t.x, t.y, { steps: 12 })
  await page.mouse.up()
  await page.mouse.move(t.x + 60, t.y + 30, { steps: 4 })

  expect(consoleErrors.some((e) => e.includes('suppressed exception during element creation')), 'exception should be caught and logged').toBe(true)
  expect(uncaught, 'no uncaught page error').toHaveLength(0)
  expect(await active(page), 'cursor must be freed, not stuck').toBe(false)
  expect(await page.locator('.djs-element[data-element-id]').count(), 'element still placed').toBe(before + 1)
})
