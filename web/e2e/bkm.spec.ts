import { test, expect } from '@playwright/test'

// Editing a Business Knowledge Model's function on a diagram built from scratch
// (ADR-0016). Regression guard: a freshly dropped BKM lives only in the live
// canvas graph until the next structural save, so the context-pad "Funktion
// bearbeiten" action must persist the graph first — otherwise GET .../bkm/{id}
// 404s for the not-yet-saved BKM and the editor overlay silently fails to open.

test('drop a BKM and edit its function without a manual save first', async ({ page }) => {
  const name = 'E2E BKM ' + Date.now()

  await page.goto('/')
  await expect(page.locator('#modelList .model-item').first()).toBeVisible()

  // Start from an empty canvas so the only BKM is the one we drop.
  await page.locator('#newModel').click()
  const dialog = page.locator('.dlg-modal')
  await expect(dialog).toBeVisible()
  await dialog.locator('.dlg-input').fill(name)
  await dialog.getByRole('button', { name: 'Anlegen' }).click()
  await expect(page.locator('.model-item', { hasText: name })).toHaveClass(/is-current/)
  await expect(page.locator('.djs-shape')).toHaveCount(0)

  // Drop a BKM via the palette: click the create tool, then click the canvas to
  // place it (diagram-js create mode follows the cursor until the placing click).
  // Placing auto-selects the new shape and opens its context pad.
  const canvas = page.locator('.djs-container').first()
  await expect(canvas).toBeVisible()
  const paletteEntry = page.locator('.djs-palette [title="Business Knowledge Model erstellen"]')
  await expect(paletteEntry).toBeVisible()
  const box = await canvas.boundingBox()
  if (!box) throw new Error('no canvas')
  await paletteEntry.click()
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2)
  await page.mouse.click(box.x + box.width / 2, box.y + box.height / 2)
  await expect(page.locator('.djs-shape')).toHaveCount(1)

  // Open its function editor via the context pad — WITHOUT first clicking
  // Speichern. This is the path that used to fail silently (404 on the unsaved BKM).
  await page.locator('.djs-context-pad [title="Funktion bearbeiten"]').click()

  // The BKM overlay opens (it did not before the fix), showing an editable,
  // empty function. Define a constant body and save.
  const overlay = page.locator('.dt-overlay')
  await expect(overlay).toBeVisible()
  await expect(overlay.locator('.dt-title')).toContainText('(BKM)')
  const body = overlay.locator('textarea.lit-text')
  await expect(body).toBeVisible()
  await body.fill('42')
  await overlay.getByRole('button', { name: 'Speichern' }).click()

  // Saving recompiles and closes the overlay; the BKM is still on the canvas.
  await expect(overlay).toHaveCount(0)
  await expect(page.locator('.djs-shape')).toHaveCount(1)
})
