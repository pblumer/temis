import { test, expect } from '@playwright/test'

// boxOf returns a locator's bounding box, retrying until it is available — under
// heavy parallel load the shared dev server can re-render the canvas mid-test, so
// a single boundingBox() call occasionally returns null (see palette.spec.ts).
async function boxOf(locator: import('@playwright/test').Locator): Promise<{ x: number; y: number; width: number; height: number }> {
  await expect(locator).toBeVisible()
  for (let i = 0; i < 20; i++) {
    const b = await locator.boundingBox()
    if (b) return b
    await locator.page().waitForTimeout(50)
  }
  throw new Error('no bounding box')
}

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
  await page.locator('.djs-context-pad [title^="Umbenennen"]').click()

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
  const box = await boxOf(canvas)
  await paletteEntry.click()
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2)
  await page.mouse.click(box.x + box.width / 2, box.y + box.height / 2)
  await expect(page.locator('.djs-shape')).toHaveCount(1)
  // A freshly dropped decision starts an inline rename so it can be named in the
  // same gesture; the box must appear. Dismiss it so we test the double-click
  // gesture on a settled element.
  await expect(page.locator('.djs-direct-editing-content')).toBeVisible()
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

// Creating an element and naming it is one gesture: the freshly dropped decision
// opens its inline-rename box, and typing + Enter names it directly — no second
// trip to the pencil icon.
test('a freshly dropped decision can be named directly in the same gesture', async ({ page }) => {
  await page.goto('/')
  const model = 'E2E NameOnCreate ' + Date.now()
  await page.locator('#newModel').click()
  const dialog = page.locator('.dlg-modal')
  await expect(dialog).toBeVisible()
  await dialog.locator('.dlg-input').fill(model)
  await dialog.getByRole('button', { name: 'Anlegen' }).click()
  await expect(page.locator('.model-item', { hasText: model })).toHaveClass(/is-current/)

  const canvas = page.locator('.djs-container').first()
  await expect(canvas).toBeVisible()
  const paletteEntry = page.locator('.djs-palette [title="Decision erstellen"]')
  await expect(paletteEntry).toBeVisible()
  const box = await boxOf(canvas)
  await paletteEntry.click()
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2)
  await page.mouse.click(box.x + box.width / 2, box.y + box.height / 2)
  await expect(page.locator('.djs-shape')).toHaveCount(1)

  // The rename box opens on its own; type the name and commit with Enter.
  const editor = page.locator('.djs-direct-editing-content')
  await expect(editor).toBeVisible()
  const name = 'Rabattstufe'
  await editor.selectText()
  await page.keyboard.type(name)
  await page.keyboard.press('Enter')
  await expect(page.locator('.djs-direct-editing-content')).toHaveCount(0)

  // The node now carries the typed name, not the "Neue Decision" default.
  await expect(page.locator(`.djs-element:has-text("${name}")`)).toHaveCount(1)
  await expect(page.locator('.djs-element:has-text("Neue Decision")')).toHaveCount(0)
})

// Enter is a second keyboard rename next to F2 (Finder-style): on the selected
// nameable shape it opens the inline-rename box.
test('Enter renames the selected element', async ({ page }) => {
  await page.goto('/')
  await page.getByText('BoxedCollections', { exact: true }).first().click()
  await expect(page.locator('.djs-palette')).toBeVisible()

  // Select a settled decision (deselect first so the selection is fresh), then
  // press Enter — the inline-rename box must open.
  await page.locator('[data-element-id="id_numbers"]').first().click()
  await expect(page.locator('.djs-context-pad')).toBeVisible()
  await page.keyboard.press('Enter')
  await expect(page.locator('.djs-direct-editing-content')).toBeVisible()
})
