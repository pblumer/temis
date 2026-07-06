import { test, expect } from '@playwright/test'

// Stage 3 — the juice. A fresh evaluation plays the illumination as a depth-
// staggered wave: a particle layer over the diagram, the wires stream, and each
// decision pulses as its inputs arrive. It is opt-out via the ⚡ toolbar toggle.
// Driven against the bundled "Discount" example (two inputs feeding one decision).

test('juice: evaluating streams the wires; the ⚡ toggle turns the effects off', async ({ page }) => {
  await page.goto('/')
  await page.getByText('Discount', { exact: true }).first().click()
  await page.locator('#modeOperate').click()

  // The particle layer is mounted over the diagram.
  await expect(page.locator('canvas.fx-layer')).toHaveCount(1)

  // Evaluate on the canvas (Stage 2 pills). With effects on (default), the lit
  // wires stream — the flowing-dash animation marker is added.
  await page.locator('select.pill-field').selectOption('Business')
  await page.locator('input.pill-field').fill('1200')
  await page.locator('input.pill-field').blur()
  await expect(page.locator('.djs-connection.flow-stream').first()).toBeVisible()

  // Turn effects off and re-evaluate: the wires no longer stream, but the static
  // illumination (Stage 1) still lights them and floats the flowing value.
  await page.locator('#juice').click()
  await expect(page.locator('#juice')).toHaveClass(/juice-off/)
  await page.locator('input.pill-field').fill('50')
  await page.locator('input.pill-field').blur()
  await expect(page.locator('.flow-edge-val').filter({ hasText: '50' })).toBeVisible()
  await expect(page.locator('.djs-connection.flow-stream')).toHaveCount(0)
  await expect(page.locator('.djs-connection.flow-active').first()).toBeVisible()
})
