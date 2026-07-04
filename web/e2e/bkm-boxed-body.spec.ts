import { test, expect } from '@playwright/test'

// A business knowledge model whose encapsulated body is a boxed expression (here a
// decision table) used to open read-only in the simple BKM editor
// ("Boxed-Expression-Body — hier (noch) schreibgeschützt"). WP-66 makes it open in
// the matching boxed editor instead, anchored so edits write back to the body.
// This drives the bundled "Bewertung" example (a BKM with a decision-table body)
// through the real temisd and edits the table.

test('a BKM with a boxed table body opens the table editor and saves', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('#modelList .model-item').first()).toBeVisible()

  // Open the Bewertung example; its DRG renders the BKM + a decision + input.
  await page.locator('.model-item', { hasText: 'Bewertung' }).first().click()
  const bkm = page.locator('[data-element-id="id_bewertung"]').first()
  await expect(bkm).toBeVisible()

  // Select the BKM and open its function via the context pad.
  await bkm.click()
  await page.locator('.djs-context-pad [title="Funktion bearbeiten"]').click()

  // The boxed table editor opens — NOT the read-only "schreibgeschützt" note.
  const overlay = page.locator('.dt-overlay')
  await expect(overlay).toBeVisible()
  await expect(overlay.locator('.eval-empty')).toHaveCount(0)
  const outCells = overlay.locator('.dt-cell-out')
  await expect(outCells.first()).toBeVisible()
  // Three rules → three output cells.
  await expect(outCells).toHaveCount(3)

  // Edit the top rule's output and save; the model recompiles and the overlay closes.
  await outCells.first().fill('"top"')
  await overlay.getByRole('button', { name: 'Speichern' }).click()
  await expect(overlay).toHaveCount(0)

  // Reopening shows the edit persisted on the new revision.
  await page.locator('[data-element-id="id_bewertung"]').first().click()
  await page.locator('.djs-context-pad [title="Funktion bearbeiten"]').click()
  await expect(page.locator('.dt-overlay .dt-cell-out').first()).toHaveValue('"top"')
})
