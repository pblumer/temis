import { test, expect, type Page } from '@playwright/test'

// A decision table derives its input columns from the decision's information
// requirements only when the table is created. If the table then loses a column
// (or an input is wired in afterwards), that requirement used to be missing from
// the table editor entirely — no Input column, even though the input pill is
// right there in the graph. The editor now reconciles against the decision's
// live wired inputs and surfaces any that has no column yet.

async function createModel(page: Page): Promise<void> {
  await page.goto('/')
  await page.locator('#newModel').click()
  const dialog = page.locator('.dlg-modal')
  await expect(dialog).toBeVisible()
  await dialog.locator('.dlg-input').fill('WiredInput ' + Date.now())
  await dialog.getByRole('button', { name: 'Anlegen' }).click()
  await expect(page.locator('.model-item.is-current')).toBeVisible()
}

// dropDecision palette-drops a bare decision and returns its element id.
async function dropDecision(page: Page): Promise<string> {
  const canvas = page.locator('#canvas')
  await expect(canvas).toBeVisible()
  const paletteEntry = canvas.locator('.djs-palette [title="Decision erstellen"]')
  await expect(paletteEntry).toBeVisible()
  const box = await canvas.boundingBox()
  if (!box) throw new Error('no canvas')
  await paletteEntry.click()
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 3)
  await page.mouse.click(box.x + box.width / 2, box.y + box.height / 3)
  await expect(page.locator('.djs-shape')).toHaveCount(1)
  // A freshly dropped decision starts an inline rename so it can be named in one
  // gesture; wait for that box, then dismiss it to keep the default name here.
  await expect(page.locator('.djs-direct-editing-content')).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.locator('.djs-direct-editing-content')).toHaveCount(0)
  const id = await page.locator('.djs-shape').first().getAttribute('data-element-id')
  if (!id) throw new Error('no decision id')
  return id
}

// selectDecision raises the decision's context pad. The pad only opens on a fresh
// selection, so we deselect (click an empty canvas corner) before clicking the
// shape by id.
async function selectDecision(page: Page, id: string): Promise<void> {
  const box = await page.locator('#canvas').boundingBox()
  if (!box) throw new Error('no canvas')
  await page.mouse.click(box.x + 40, box.y + box.height - 40)
  await page.locator(`.djs-shape[data-element-id="${id}"]`).click()
  await expect(page.locator('.djs-context-pad.open')).toBeVisible()
}

test('a wired input with no column is surfaced when the table is opened', async ({ page }) => {
  await createModel(page)
  const id = await dropDecision(page)

  // Wire an input to the decision.
  await selectDecision(page, id)
  await page.locator('.djs-context-pad [title="Eingabedaten anhängen"]').click()
  await expect(page.locator('.djs-shape')).toHaveCount(2)

  const overlay = page.locator('.dt-overlay')

  // Create the table: its column is derived from the wired input. Then delete that
  // column and save — leaving a table with no input columns while the requirement
  // edge stays. This is the state a table lands in when an input is wired after the
  // table was made (the requirement never became a column).
  await selectDecision(page, id)
  await page.locator('.djs-context-pad [title="Decision Table anlegen"]').click()
  await expect(overlay).toBeVisible()
  await expect(overlay.locator('.dt-in .dt-head-field').first()).toHaveValue('Neue Eingabe')
  await overlay.locator('.dt-in .dt-colhead .dt-rm').click()
  await expect(overlay.locator('.dt-band .dt-in')).toHaveCount(0)
  await overlay.locator('.dt-save').click()
  await expect(overlay).toHaveCount(0)

  // Reopen the table: the wired input must be surfaced again as an Input column,
  // pre-filled with the input's name — without the fix it would be missing.
  await selectDecision(page, id)
  await page.locator('.djs-context-pad [title="Decision Table anzeigen"]').click()
  await expect(overlay).toBeVisible()
  await expect(overlay.locator('.dt-band .dt-in', { hasText: 'Input' })).toBeVisible()
  await expect(overlay.locator('.dt-in .dt-head-field').first()).toHaveValue('Neue Eingabe')
})
