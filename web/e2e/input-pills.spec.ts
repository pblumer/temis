import { test, expect } from '@playwright/test'

// Stage 2 — on-canvas input pills (Operate). Each leaf input gets an editable pill
// mounted on its inputData node, so the whole graph's inputs are filled on the
// diagram itself. Editing a pill re-evaluates the whole graph live (debounced) and
// re-illuminates it. Driven against the bundled "Discount" example (Customer +
// Order Total feeding one decision).

test('input pills: editing a leaf input on its node re-evaluates the whole graph live', async ({ page }) => {
  await page.goto('/')
  await page.getByText('Discount', { exact: true }).first().click()

  // Enter Operate: an editable pill appears on each inputData node (one <select>
  // for the closed enum Customer, one text box for the numeric Order Total).
  await page.locator('#modeOperate').click()
  await expect(page.locator('.node-input')).toHaveCount(2)
  await expect(page.locator('select.pill-field')).toHaveCount(1)
  await expect(page.locator('input.pill-field')).toHaveCount(1)

  // Fill the inputs directly on the diagram — no side-panel form needed.
  await page.locator('select.pill-field').selectOption('Business')
  await page.locator('input.pill-field').fill('1200')
  await page.locator('input.pill-field').blur()

  // The whole graph evaluates live: the on-node result pill and the edge
  // illumination (Stage 1) appear from the pill edits alone.
  await expect(page.locator('.node-result')).toHaveCount(1)
  await expect(page.locator('.djs-connection.flow-active').first()).toBeVisible()
  await expect(page.locator('.flow-edge-val').filter({ hasText: '1200' })).toBeVisible()

  // A run landed in the session history from the on-canvas edit.
  await expect(page.locator('.op-run')).not.toHaveCount(0)

  // Editing again recomputes: a different order total flows through the diagram.
  await page.locator('input.pill-field').fill('50')
  await page.locator('input.pill-field').blur()
  await expect(page.locator('.flow-edge-val').filter({ hasText: '50' })).toBeVisible()

  // Leaving Operate takes the input pills off the diagram.
  await page.locator('#modeDesign').click()
  await expect(page.locator('.node-input')).toHaveCount(0)
})
