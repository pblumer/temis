import { test, expect } from '@playwright/test'

// Managing models from the sidebar (ADR-0016): creating a blank model from
// scratch, renaming it and deleting it — the full stack, driving the real
// dialogs against temisd's rename/delete endpoints and the content-addressed
// cache. A unique name per run keeps parallel runs from colliding in the cache.

test('create a blank model, rename it, then delete it', async ({ page }) => {
  const base = 'E2E Modell ' + Date.now()
  const renamed = base + ' (umbenannt)'

  await page.goto('/')
  await expect(page.locator('#modelList .model-item').first()).toBeVisible()
  // A non-empty demo model loads by default, so the canvas has shapes to begin
  // with — the blank model must clear them.
  await expect(page.locator('.djs-shape').first()).toBeVisible()

  // Create: the new-model dialog, name it, confirm.
  await page.locator('#newModel').click()
  const dialog = page.locator('.dlg-modal')
  await expect(dialog).toBeVisible()
  await dialog.locator('.dlg-input').fill(base)
  await dialog.getByRole('button', { name: 'Anlegen' }).click()

  // It appears in the sidebar and becomes the current (selected) model.
  const row = page.locator('.model-item', { hasText: base })
  await expect(row).toHaveClass(/is-current/)
  // The blank model actually loads onto the canvas: its empty graph replaces the
  // previous model's shapes. Regression guard for the /graph endpoint returning
  // null (not []) for an empty model, which used to throw in layout() and leave
  // the old diagram on screen while the new model showed as current.
  await expect(page.locator('.djs-shape')).toHaveCount(0)

  // Rename: the current row's actions are visible; open the rename dialog.
  await row.locator('button[title="Modell umbenennen"]').click()
  const renameDialog = page.locator('.dlg-modal')
  await expect(renameDialog.locator('.dlg-input')).toHaveValue(base)
  await renameDialog.locator('.dlg-input').fill(renamed)
  await renameDialog.getByRole('button', { name: 'Umbenennen' }).click()

  // The row now shows the new name and the old name is gone.
  const renamedRow = page.locator('.model-item', { hasText: renamed })
  await expect(renamedRow).toBeVisible()
  // The old exact name is gone (renamed still contains base as a prefix, so match
  // the exact text node, not a substring).
  await expect(page.getByText(base, { exact: true })).toHaveCount(0)

  // Delete: confirm the destructive dialog, then the model is gone.
  await renamedRow.locator('button[title="Modell löschen"]').click()
  const confirm = page.locator('.dlg-modal')
  await expect(confirm).toBeVisible()
  await confirm.getByRole('button', { name: 'Löschen' }).click()

  await expect(page.locator('.model-item', { hasText: renamed })).toHaveCount(0)
})
