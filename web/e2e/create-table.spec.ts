import { test, expect } from '@playwright/test'

// Giving a freshly dropped decision a decision table on a diagram built from
// scratch (ADR-0016). Regression guard, mirroring bkm.spec.ts: a new decision
// lives only in the live canvas graph until the next structural save, so the
// context-pad "Decision Table anlegen" action must persist the graph first —
// otherwise POST .../decisions/{id}/create-table 400s for the not-yet-saved
// decision ("cannot create a table for decision … (unknown …)") and the table
// editor never opens. createTable force-persists exactly for this reason.

test('drop a decision and create its table without a manual save first', async ({ page }) => {
  const name = 'E2E Table ' + Date.now()

  await page.goto('/')
  await expect(page.locator('#modelList .model-item').first()).toBeVisible()

  // Start from an empty canvas so the only decision is the one we drop.
  await page.locator('#newModel').click()
  const dialog = page.locator('.dlg-modal')
  await expect(dialog).toBeVisible()
  await dialog.locator('.dlg-input').fill(name)
  await dialog.getByRole('button', { name: 'Anlegen' }).click()
  await expect(page.locator('.model-item', { hasText: name })).toHaveClass(/is-current/)
  await expect(page.locator('.djs-shape')).toHaveCount(0)

  // Drop a decision via the palette: click the create tool, then click the canvas
  // to place it. Placing auto-selects the new shape and opens its context pad.
  const canvas = page.locator('.djs-container').first()
  await expect(canvas).toBeVisible()
  const paletteEntry = page.locator('.djs-palette [title="Decision erstellen"]')
  await expect(paletteEntry).toBeVisible()
  const box = await canvas.boundingBox()
  if (!box) throw new Error('no canvas')
  await paletteEntry.click()
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2)
  await page.mouse.click(box.x + box.width / 2, box.y + box.height / 2)
  await expect(page.locator('.djs-shape')).toHaveCount(1)

  // Create its decision table via the context pad — WITHOUT first clicking
  // Speichern. This is the path that 400s if the new decision was not persisted.
  await page.locator('.djs-context-pad [title="Decision Table anlegen"]').click()

  // The table overlay opens (it would not if the create-table request had failed),
  // titled after the decision.
  const overlay = page.locator('.dt-overlay')
  await expect(overlay).toBeVisible()
  await expect(overlay.locator('.dt-title')).toContainText('Neue Decision')

  // The decision is still on the canvas and now carries a table.
  await expect(page.locator('.djs-shape')).toHaveCount(1)
})
