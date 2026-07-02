// The DRD orientation toggle (routing/layout improvements): a toolbar button
// flips the auto-layout between bottom-up (leaf inputs at the bottom feeding
// decisions upward — the default) and top-down (inputs on top feeding down), and
// re-routes every edge orthogonally. The Alterskette demo is a clean chain
// (Age → Category → Greeting), so the vertical order of the input relative to the
// final decision tells the two orientations apart.
import { test, expect } from '@playwright/test'

async function box(locator: import('@playwright/test').Locator): Promise<{ x: number; y: number; width: number; height: number }> {
  await expect(locator).toBeVisible()
  for (let i = 0; i < 20; i++) {
    const b = await locator.boundingBox()
    if (b) return b
    await locator.page().waitForTimeout(50)
  }
  throw new Error('no bounding box')
}

test('orientation toggle flips whether inputs feed decisions from below or above', async ({ page }) => {
  await page.goto('/')
  await page.getByText('Alterskette (Demo)', { exact: true }).first().click()
  await expect(page.locator('.djs-palette')).toBeVisible()
  await expect(page.locator('.djs-element[data-element-id]').first()).toBeVisible()

  const input = page.locator('.djs-element:has-text("Age")')
  const decision = page.locator('.djs-element:has-text("Greeting")')
  const orient = page.locator('#orient')

  // Default is bottom-up: the Age input sits below the Greeting decision.
  await expect(orient).toHaveText(/Bottom-up/)
  expect((await box(input)).y).toBeGreaterThan((await box(decision)).y)

  // Toggle to top-down: the input is now above the decision, and the label flips.
  await orient.click()
  await expect(orient).toHaveText(/Top-down/)
  await page.waitForTimeout(200)
  expect((await box(input)).y).toBeLessThan((await box(decision)).y)

  // Toggling back restores bottom-up.
  await orient.click()
  await expect(orient).toHaveText(/Bottom-up/)
  await page.waitForTimeout(200)
  expect((await box(input)).y).toBeGreaterThan((await box(decision)).y)
})
