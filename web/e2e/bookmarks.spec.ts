import { test, expect } from '@playwright/test'

// Bookmarks are the personal view layer over the namespace tree (ADR-0034,
// WP-142): create, rename and delete them from the sidebar. They live in
// localStorage (per browser), so these drive the real dialogs against a fresh
// context — no server state involved.

test('bookmarks: create, rename, and one-click delete when empty', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('#modelList .model-item').first()).toBeVisible()

  // Create.
  await page.locator('#newFolder').click()
  let dlg = page.locator('.dlg-modal')
  await expect(dlg).toBeVisible()
  await dlg.locator('.dlg-input').fill('Kunde A')
  await dlg.getByRole('button', { name: 'Anlegen' }).click()
  const head = page.locator('.folder-head', { hasText: 'Kunde A' })
  await expect(head).toBeVisible()
  await expect(head.locator('.folder-count')).toHaveText('0')

  // Rename: the dialog is prefilled with the old name. The action buttons reveal
  // on hover, so hover the bookmark head first.
  await head.hover()
  await head.locator('button[title="Lesezeichen umbenennen"]').click()
  dlg = page.locator('.dlg-modal')
  await expect(dlg.locator('.dlg-input')).toHaveValue('Kunde A')
  await dlg.locator('.dlg-input').fill('Kunde B')
  await dlg.getByRole('button', { name: 'Umbenennen' }).click()
  await expect(page.locator('.folder-name', { hasText: 'Kunde B' })).toBeVisible()
  await expect(page.locator('.folder-name', { hasText: 'Kunde A' })).toHaveCount(0)

  // Delete an empty bookmark: one click, no confirm dialog.
  const headB = page.locator('.folder-head', { hasText: 'Kunde B' })
  await headB.hover()
  await headB.locator('button[title="Leeres Lesezeichen löschen"]').click()
  await expect(page.locator('.folder-name', { hasText: 'Kunde B' })).toHaveCount(0)
})

test('bookmarks: deleting a non-empty bookmark asks first and keeps the models', async ({ page }) => {
  const model = 'Begrüßung (Demo)'
  // Seed a bookmark holding an example model before the app boots (it reads
  // localStorage on load), so the bookmark is non-empty without a drag gesture.
  await page.addInitScript((m) => {
    localStorage.setItem('temis.modeler.folders', JSON.stringify({ folders: ['Kunde X'], assign: { [m]: 'Kunde X' } }))
  }, model)
  await page.goto('/')

  const head = page.locator('.folder-head', { hasText: 'Kunde X' })
  await expect(head).toBeVisible()
  await expect(head.locator('.folder-count')).toHaveText('1')
  await expect(page.locator('.folder-body .model-item', { hasText: model })).toBeVisible()

  // Deleting a populated bookmark asks first.
  await head.hover()
  await head.locator('button[title="Lesezeichen löschen (Modelle bleiben erhalten)"]').click()
  const confirm = page.locator('.dlg-modal')
  await expect(confirm).toBeVisible()
  await confirm.getByRole('button', { name: 'Löschen' }).click()

  // The bookmark is gone; its model survives (now unfiled, flat in the list).
  await expect(page.locator('.folder-name', { hasText: 'Kunde X' })).toHaveCount(0)
  await expect(page.locator('#modelList .model-item', { hasText: model })).toBeVisible()
})
