import { test, expect } from '@playwright/test'

// The Operate cockpit (ADR-0016): a runtime view kept distinct from the Design
// editor. This drives the full stack against the bundled "Discount" example
// (a decision table with a numeric input) to cover the three building blocks:
//   1. a keyboard-navigable run history above the diagram,
//   2. frosted summary overlays over the diagram, and
//   3. a hover graphic that draws the decision table with the hit rule.

test('operate: run history is keyboard-navigable and drives overlays', async ({ page }) => {
  await page.goto('/')
  await page.getByText('Discount', { exact: true }).first().click()

  // Enter Operate; the design-only palette must be gone (read-only view).
  await page.locator('#modeOperate').click()
  await expect(page.locator('.op-history')).toBeVisible()
  await expect(page.locator('.djs-palette')).toBeHidden()

  // Run two evaluations to build a session history (newest ends up on top).
  const runOnce = async (customer: string, total: number): Promise<void> => {
    await page.locator('select.eval-field').selectOption(customer)
    await page.locator('input.eval-field').fill(String(total))
    await page.locator('#evalRun').click()
  }
  await runOnce('Business', 1200)
  await runOnce('Private', 300)
  await expect(page.locator('.op-run')).toHaveCount(2)

  // The listbox has proper ARIA and the newest run is active.
  const history = page.locator('#opHistory')
  await expect(history).toHaveAttribute('role', 'listbox')
  await expect(page.locator('.op-run.is-active .op-run-in')).toHaveText(/Order Total=300/)

  // Keyboard: ArrowDown moves to the older run; the overlays follow.
  await history.focus()
  await page.keyboard.press('ArrowDown')
  await expect(page.locator('.op-run.is-active')).toHaveAttribute('aria-selected', 'true')
  await expect(page.locator('.op-run.is-active .op-run-in')).toHaveText(/Order Total=1200/)
  await expect(page.locator('.op-ov-inputs')).toContainText('1200')

  // Home jumps back to the newest, j/k step older/newer.
  await page.keyboard.press('Home')
  await expect(page.locator('.op-run.is-active .op-run-in')).toHaveText(/Order Total=300/)
  await page.keyboard.press('j')
  await expect(page.locator('.op-run.is-active .op-run-in')).toHaveText(/Order Total=1200/)
  await page.keyboard.press('k')
  await expect(page.locator('.op-run.is-active .op-run-in')).toHaveText(/Order Total=300/)

  // Overlays summarise the active run; on-node result pills remain.
  await expect(page.locator('.op-ov-results')).toContainText('Discount')
  await expect(page.locator('.node-result')).toHaveCount(1)

  // Baustein 3: hovering a result row reveals the decision-table matrix.
  await page.locator('.op-ov-results .op-ov-row.op-ov-hoverable').first().hover()
  await expect(page.locator('.op-pop .op-mgrid')).toBeVisible()
  await expect(page.locator('.op-pop .op-mrule.is-hit')).toBeVisible()

  // Back in Design the cockpit chrome is hidden again.
  await page.locator('#modeDesign').click()
  await expect(page.locator('.op-history')).toBeHidden()
  await expect(page.locator('.op-overlays')).toBeHidden()
})
