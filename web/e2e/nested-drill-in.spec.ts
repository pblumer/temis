import { test, expect } from '@playwright/test'

// A boxed context that nests another boxed expression (here a list as one entry's
// value) used to open fully read-only. WP-66 Phase 2 keeps it read-only for its
// literal fields but offers a drill-in ("✎") on each nested entry that opens the
// matching editor at that entry's locator and edits it in place. This drives the
// bundled "Verschachtelt" example through the real temisd.

test('drill into a nested list inside a context and edit it in place', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('#modelList .model-item').first()).toBeVisible()

  await page.locator('.model-item', { hasText: 'Verschachtelt' }).first().click()
  const dec = page.locator('[data-element-id="id_gesamt"]').first()
  await expect(dec).toBeVisible()

  // Open the decision's boxed context; it nests a list, so it opens read-only.
  await dec.dblclick()
  const ctx = page.locator('.ctx-modal')
  await expect(ctx).toBeVisible()

  // The nested entry offers a drill-in; the literal fields are read-only.
  const drill = ctx.locator('.ctx-drill')
  await expect(drill).toHaveCount(1)
  await drill.click()

  // The list editor opens on the nested list (2 items) and edits it in place.
  const list = page.locator('.list-modal')
  await expect(list).toBeVisible()
  const items = list.locator('.list-item')
  await expect(items).toHaveCount(2)
  await items.nth(1).fill('basis * 2')
  await list.getByRole('button', { name: 'Speichern' }).click()

  // Saving the nested list recompiles to a new revision and closes the overlays.
  await expect(page.locator('.list-modal')).toHaveCount(0)
  await expect(page.locator('.ctx-modal')).toHaveCount(0)
})
