import { test, expect } from '@playwright/test'

// Resizing a node: selecting a shape reveals diagram-js resize handles; dragging a
// corner enlarges the shape, the change marks the model dirty and the structural
// save persists the new bounds. The DMNDI round-trip itself is covered by
// TestApplyGraphResizeExisting; here we drive the real handle interaction.

// modelWidth reads the shape's exact model width from the live diagram (exposed
// under __E2E__), so the assertion is independent of canvas zoom/scroll.
const modelWidth = (page: import('@playwright/test').Page, id: string): Promise<number | null> =>
  page.evaluate((elId) => {
    const el = (window as unknown as { __diagram?: { get: (n: string) => { get: (i: string) => { width?: number } | undefined } } }).__diagram?.get('elementRegistry').get(elId)
    return el && typeof el.width === 'number' ? el.width : null
  }, id)

test('a node can be resized on the canvas and the change is savable', async ({ page }) => {
  await page.addInitScript(() => {
    ;(window as unknown as { __E2E__: boolean }).__E2E__ = true
  })
  await page.goto('/')
  const model = 'E2E Resize ' + Date.now()
  await page.locator('#newModel').click()
  const dialog = page.locator('.dlg-modal')
  await expect(dialog).toBeVisible()
  await dialog.locator('.dlg-input').fill(model)
  await dialog.getByRole('button', { name: 'Anlegen' }).click()
  await expect(page.locator('.model-item', { hasText: model })).toHaveClass(/is-current/)

  // Drop a decision and dismiss its auto-rename box.
  const canvas = page.locator('.djs-container').first()
  await expect(canvas).toBeVisible()
  await page.locator('.djs-palette [title="Decision erstellen"]').click()
  const cbox = await canvas.boundingBox()
  if (!cbox) throw new Error('no canvas')
  await page.mouse.move(cbox.x + cbox.width / 2, cbox.y + cbox.height / 2)
  await page.mouse.click(cbox.x + cbox.width / 2, cbox.y + cbox.height / 2)
  await expect(page.locator('.djs-shape')).toHaveCount(1)
  await expect(page.locator('.djs-direct-editing-content')).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.locator('.djs-direct-editing-content')).toHaveCount(0)

  const id = await page.locator('.djs-shape').first().getAttribute('data-element-id')
  if (!id) throw new Error('no decision id')
  const w0 = (await modelWidth(page, id)) ?? 0

  // A fresh selection reveals the resize handles; drag the bottom-right one out.
  await page.mouse.click(cbox.x + 8, cbox.y + 8)
  await page.locator(`.djs-shape[data-element-id="${id}"]`).click()
  const handle = page.locator('.djs-resizer-se').first()
  await expect(handle).toBeVisible()
  const hb = await handle.boundingBox()
  if (!hb) throw new Error('no handle')
  await page.mouse.move(hb.x + hb.width / 2, hb.y + hb.height / 2)
  await page.mouse.down()
  await page.mouse.move(hb.x + 150, hb.y + 90, { steps: 10 })
  await page.mouse.up()

  // The shape actually grew in model space.
  await expect.poll(() => modelWidth(page, id)).toBeGreaterThan(w0 + 5)

  // The resize marked the model dirty; saving it round-trips without error, so the
  // save button returns to its disabled (nothing-to-save) state.
  const save = page.locator('#save')
  await expect(save).toBeEnabled()
  await save.click()
  await expect(save).toBeDisabled()
})
