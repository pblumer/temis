import { test, expect } from '@playwright/test'

// Double-click on a decision must do exactly ONE thing. A decision that carries
// logic (a decision table or any boxed expression) opens its editor; a bare,
// undecided decision inline-renames. Regression guard: isRenamable used to
// exclude only table/literal/context decisions, so a decision backed by a boxed
// list (or conditional/relation/filter/for/some/every) both opened its editor
// AND started an inline rename at once — two gestures firing on one double-click.

test('double-clicking a boxed-list decision opens its editor without inline-renaming', async ({ page }) => {
  await page.goto('/')
  // BoxedCollections is a served example; "Numbers" (id_numbers) is a boxed list.
  await page.getByText('BoxedCollections', { exact: true }).first().click()
  await page.locator('[data-element-id="id_numbers"]').first().dblclick()

  // The list editor opens …
  const overlay = page.locator('.dt-overlay')
  await expect(overlay).toBeVisible()
  await expect(overlay.locator('.dt-title')).toHaveText('Liste · Numbers')

  // … and the inline-rename box does NOT appear (the colliding second gesture).
  await expect(page.locator('.djs-direct-editing-content')).toHaveCount(0)
})

test('a decision with logic is renamable via the context pad', async ({ page }) => {
  // Double-click is reserved for opening a logic-decision's editor, so renaming
  // one is a deliberate context-pad action. It must inline-rename WITHOUT opening
  // the editor.
  await page.goto('/')
  await page.getByText('BoxedCollections', { exact: true }).first().click()
  await page.locator('[data-element-id="id_numbers"]').first().click()
  await page.locator('.djs-context-pad [title="Umbenennen"]').click()

  await expect(page.locator('.djs-direct-editing-content')).toBeVisible()
  await expect(page.locator('.dt-overlay')).toHaveCount(0)
})

test('double-clicking an undecided decision still inline-renames', async ({ page }) => {
  // Guard the other side: a logic-less decision must remain double-click renamable.
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

  // Double-click it: no logic, so it inline-renames (the box appears) and no
  // editor overlay opens.
  await page.locator('.djs-shape').first().dblclick()
  await expect(page.locator('.djs-direct-editing-content')).toBeVisible()
  await expect(page.locator('.dt-overlay')).toHaveCount(0)
})
