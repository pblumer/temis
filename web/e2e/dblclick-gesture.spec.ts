import { test, expect } from '@playwright/test'

// Double-click switches to an element's CONTENT throughout the editor — it never
// renames. Renaming is a deliberate gesture only: the context pad's pencil icon
// or the F2 key. These guards pin both halves of that contract.

test('double-clicking a boxed-list decision opens its editor without inline-renaming', async ({ page }) => {
  await page.goto('/')
  // BoxedCollections is a served example; "Numbers" (id_numbers) is a boxed list.
  await page.getByText('BoxedCollections', { exact: true }).first().click()
  await page.locator('[data-element-id="id_numbers"]').first().dblclick()

  // The list editor opens …
  const overlay = page.locator('.dt-overlay')
  await expect(overlay).toBeVisible()
  await expect(overlay.locator('.dt-title')).toHaveText('Liste · Numbers')

  // … and the inline-rename box does NOT appear.
  await expect(page.locator('.djs-direct-editing-content')).toHaveCount(0)
})

test('double-clicking a BKM opens its function editor without inline-renaming', async ({ page }) => {
  await page.goto('/')
  // BkmInvocation is a served example; "Discount Rate" (id_rate) is a BKM.
  await page.getByText('BkmInvocation', { exact: true }).first().click()
  await page.locator('[data-element-id="id_rate"]').first().dblclick()

  // The BKM function editor opens …
  const overlay = page.locator('.dt-overlay')
  await expect(overlay).toBeVisible()
  await expect(overlay.locator('.dt-title')).toHaveText('Discount Rate (BKM)')

  // … and no inline-rename box appears.
  await expect(page.locator('.djs-direct-editing-content')).toHaveCount(0)
})

test('a decision with logic is renamable via the context pad', async ({ page }) => {
  // Renaming is a deliberate gesture, so it opens the inline editor WITHOUT
  // opening the logic editor.
  await page.goto('/')
  await page.getByText('BoxedCollections', { exact: true }).first().click()
  await page.locator('[data-element-id="id_numbers"]').first().click()
  await page.locator('.djs-context-pad [title="Umbenennen"]').click()

  await expect(page.locator('.djs-direct-editing-content')).toBeVisible()
  await expect(page.locator('.dt-overlay')).toHaveCount(0)
})

test('double-clicking a decision never inline-renames; F2 does', async ({ page }) => {
  // A bare, undecided decision has no content to open, so double-click does
  // nothing — it must NOT rename. F2 on the selection is the keyboard rename.
  await page.goto('/')
  const name = 'E2E Rename ' + Date.now()
  await page.locator('#newModel').click()
  const dialog = page.locator('.dlg-modal')
  await expect(dialog).toBeVisible()
  await dialog.locator('.dlg-input').fill(name)
  await dialog.getByRole('button', { name: 'Anlegen' }).click()
  await expect(page.locator('.model-item', { hasText: name })).toHaveClass(/is-current/)

  // Drop a bare decision via the palette (click the tool, then click the canvas).
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
  // A freshly dropped decision starts an inline rename; dismiss it so we test the
  // double-click gesture on a settled element.
  await page.keyboard.press('Escape')
  await expect(page.locator('.djs-direct-editing-content')).toHaveCount(0)

  // Double-click must NOT rename (no editor overlay either — there is no logic).
  await page.locator('.djs-shape').first().dblclick()
  await expect(page.locator('.djs-direct-editing-content')).toHaveCount(0)
  await expect(page.locator('.dt-overlay')).toHaveCount(0)

  // F2 on the selected decision opens the inline-rename box. The context pad (and
  // selection) only arm on a FRESH selection, so deselect first (click an empty
  // canvas corner) before selecting the shape.
  await page.mouse.click(box.x + 8, box.y + 8)
  await page.locator('.djs-shape').first().click()
  await expect(page.locator('.djs-context-pad')).toBeVisible()
  await page.keyboard.press('F2')
  await expect(page.locator('.djs-direct-editing-content')).toBeVisible()
})
