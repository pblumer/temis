import { test, expect } from '@playwright/test'

// Live-Graph illumination: after an evaluation the requirement edges that carried
// a value light up on the diagram itself and float the value that travelled each
// one at its midpoint — the dependency dataflow made visible on the graph, not
// only in the "Auswerten" panel. Driven against the bundled "Discount" example
// (two leaf inputs feeding one decision, so two data edges illuminate).

test('live-graph: evaluating illuminates the requirement edges with their flowing values', async ({ page }) => {
  await page.goto('/')
  await page.getByText('Discount', { exact: true }).first().click()

  // Enter Operate and evaluate a case.
  await page.locator('#modeOperate').click()
  await page.locator('select.eval-field').selectOption('Business')
  await page.locator('input.eval-field').fill('1200')
  await page.locator('#evalRun').click()

  // The on-node result pill stays (unchanged behaviour).
  await expect(page.locator('.node-result')).toHaveCount(1)

  // The requirement edges that carried a value are now coloured in (marker class).
  await expect(page.locator('.djs-connection.flow-active').first()).toBeVisible()

  // The value that travelled an edge floats on the diagram: the numeric input
  // (Order Total = 1200) and the string input (Customer = Business) each appear.
  await expect(page.locator('.flow-edge-val').filter({ hasText: '1200' })).toBeVisible()
  await expect(page.locator('.flow-edge-val').filter({ hasText: 'Business' })).toBeVisible()

  // Back in Design the illumination + labels persist with the active run's pills
  // (the same lifetime as the result overlays), then a fresh model clears them.
  await page.locator('#modeDesign').click()
  await expect(page.locator('.flow-edge-val').filter({ hasText: '1200' })).toBeVisible()
})
