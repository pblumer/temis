import { test, expect } from '@playwright/test'

// Publishing a model as a release (ADR-0037): every save is a draft; the toolbar
// "Veröffentlichen" button tags the current revision as a named version, and the
// sidebar then leads with that release (a version badge + a clickable release
// chip with its channel) instead of the raw revision count. A unique name per run
// keeps parallel runs from colliding in the content-addressed cache.

test('publish a model as a release and see it in the sidebar', async ({ page }) => {
  const name = 'E2E Release ' + Date.now()

  await page.goto('/')
  await expect(page.locator('#modelList .model-item').first()).toBeVisible()

  // Create a fresh model so this run owns it.
  await page.locator('#newModel').click()
  const dialog = page.locator('.dlg-modal')
  await expect(dialog).toBeVisible()
  await dialog.locator('.dlg-input').fill(name)
  await dialog.getByRole('button', { name: 'Anlegen' }).click()

  const row = page.locator('.model-item', { hasText: name })
  await expect(row).toHaveClass(/is-current/)

  // Publish: the toolbar button opens the version dialog pre-filled with 1.0.0.
  await page.locator('#publish').click()
  const pub = page.locator('.dlg-modal')
  await expect(pub.locator('.dlg-input')).toHaveValue('1.0.0')
  // Scope the OK button to the dialog so it doesn't collide with the toolbar's own
  // "Veröffentlichen" button.
  await pub.getByRole('button', { name: 'Veröffentlichen' }).click()

  // The row now leads with the release version badge, and — since the current head
  // IS the published revision — carries no unpublished-draft flag.
  await expect(row.locator('.model-release-badge')).toHaveText('1.0.0')
  await expect(row.locator('.model-draft-badge')).toHaveCount(0)

  // A clickable release chip with its channel appears under the model.
  await expect(page.locator('.release-chip .release-ver', { hasText: '1.0.0' }).first()).toBeVisible()
  await expect(page.locator('.release-channel', { hasText: 'latest' }).first()).toBeVisible()

  // The status line confirms the publish.
  await expect(page.locator('#status')).toContainText('veröffentlicht')
})
