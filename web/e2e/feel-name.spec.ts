import { test, expect } from '@playwright/test'

// Weg A: a node's display name is a free-form label, separate from its FEEL
// identifier (the variable name). The display name may carry characters FEEL
// rejects (parentheses here; FEEL already allows spaces and hyphens); the FEEL
// name stays a valid identifier and is edited via the context pad's FEEL-name
// action. Both survive a structural save.

async function boxOf(locator: import('@playwright/test').Locator): Promise<{ x: number; y: number; width: number; height: number }> {
  await expect(locator).toBeVisible()
  for (let i = 0; i < 20; i++) {
    const b = await locator.boundingBox()
    if (b) return b
    await locator.page().waitForTimeout(50)
  }
  throw new Error('no bounding box')
}

test('a node takes a free-form display label and a separate, validated FEEL name', async ({ page }) => {
  await page.goto('/')
  const model = 'E2E FeelName ' + Date.now()
  await page.locator('#newModel').click()
  const dialog = page.locator('.dlg-modal')
  await expect(dialog).toBeVisible()
  await dialog.locator('.dlg-input').fill(model)
  await dialog.getByRole('button', { name: 'Anlegen' }).click()
  await expect(page.locator('.model-item', { hasText: model })).toHaveClass(/is-current/)

  // Drop a decision; the auto-rename box opens.
  const canvas = page.locator('.djs-container').first()
  await expect(canvas).toBeVisible()
  await page.locator('.djs-palette [title="Decision erstellen"]').click()
  const box = await boxOf(canvas)
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2)
  await page.mouse.click(box.x + box.width / 2, box.y + box.height / 2)
  await expect(page.locator('.djs-shape')).toHaveCount(1)

  // The display name is free-form: a label FEEL rejects (parentheses) is accepted,
  // not rejected. Type it and commit with Enter.
  const editor = page.locator('.djs-direct-editing-content')
  await expect(editor).toBeVisible()
  await editor.selectText()
  await page.keyboard.type('Score (0-100)')
  await page.keyboard.press('Enter')
  await expect(page.locator('.djs-direct-editing-content')).toHaveCount(0)
  await expect(page.locator('.djs-element:has-text("Score (0-100)")')).toHaveCount(1)

  // Open the FEEL-name editor from the context pad. It must reject an invalid FEEL
  // name (a hyphen) live, and accept a clean identifier. The context pad only arms
  // on a FRESH selection, so deselect (empty-canvas click) before selecting.
  await page.mouse.click(box.x + 8, box.y + 8)
  await page.locator('.djs-shape').first().click()
  await expect(page.locator('.djs-context-pad')).toBeVisible()
  await page.locator('.djs-context-pad [title^="FEEL-Name"]').click()
  const feelBox = page.locator('.djs-direct-editing-content')
  await expect(feelBox).toBeVisible()
  // Live validation needs the FEEL engine (wasm) loaded; wait for it, then type so
  // the input event re-checks with the validator available.
  await page.waitForFunction(() => !!(window as unknown as { temisFeelValidateName?: unknown }).temisFeelValidateName)
  await feelBox.selectText()
  await page.keyboard.type('Score (0-100)')
  await expect(feelBox).toHaveClass(/name-invalid/)
  await feelBox.selectText()
  await page.keyboard.type('Score')
  await expect(feelBox).not.toHaveClass(/name-invalid/)
  await page.keyboard.press('Enter')
  await expect(page.locator('.djs-direct-editing-content')).toHaveCount(0)

  // The node now shows the FEEL identifier in its subtitle, distinct from the label.
  const node = page.locator('.djs-element:has-text("Score (0-100)")')
  await expect(node).toContainText('▸ Score')

  // Persist, then confirm the round-trip: after the save re-selects the new
  // revision, the node still carries the free-form label and the separate FEEL name
  // (loaded back from the server's DMN, so the <variable> was written and read).
  await page.locator('#save').click()
  const reloaded = page.locator('.djs-element:has-text("Score (0-100)")')
  await expect(reloaded).toHaveCount(1)
  await expect(reloaded).toContainText('▸ Score')
})
