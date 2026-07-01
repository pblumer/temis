import { test, expect } from '@playwright/test'

// The Import cockpit (Testfall-Import): a third mode next to Design/Operate that
// runs imported test cases against the live engine as an animated conveyor belt —
// records flow from the Eingang lane, through Evaluation, into the clio Store,
// carrying their computed results. This drives the full stack against the bundled
// "Discount" example (a decision table: a Customer enum + a numeric Order Total).

test('import: seeded samples run down the belt into the clio store', async ({ page }) => {
  await page.goto('/')
  await page.getByText('Discount', { exact: true }).first().click()

  // Enter Import; the design palette and evaluate form step aside for the belt.
  await page.locator('#modeImport').click()
  await expect(page.locator('.import-cockpit')).toBeVisible()
  await expect(page.locator('.djs-palette')).toBeHidden()
  await expect(page.locator('.eval-panel')).toBeHidden()

  // The three lanes are labelled Eingang → Evaluation → clio Store.
  await expect(page.locator('.imp-empty-msg')).toBeVisible()

  // Seed example cases from the model's inferred input values, then run them.
  await page.getByRole('button', { name: 'Beispiele einfügen' }).click()
  await expect(page.locator('.imp-lane-in .imp-card')).toHaveCount(3)

  await page.getByRole('button', { name: /Durchlaufen lassen/ }).click()

  // Every record lands in the clio Store lane, carrying the Discount result.
  await expect(page.locator('.imp-lane-store .imp-card')).toHaveCount(3, { timeout: 15_000 })
  await expect(page.locator('.imp-lane-store .imp-out-k').first()).toHaveText('Discount')
  await expect(page.locator('.imp-note')).toContainText('gelaufen')

  // Leeren empties the belt back to the hint state.
  await page.getByRole('button', { name: 'Leeren' }).click()
  await expect(page.locator('.imp-card')).toHaveCount(0)
  await expect(page.locator('.imp-empty-msg')).toBeVisible()
})

test('import: a CSV of test cases imports, runs and asserts expectations', async ({ page }) => {
  await page.goto('/')
  await page.getByText('Discount', { exact: true }).first().click()
  await page.locator('#modeImport').click()

  // A hand-authored CSV in the template shape: the `case` label, the two inputs,
  // and a `→Discount` expected column (turning each row into a pass/fail check).
  const csv = ['case,Customer Type,Order Total,→Discount', 'Großkunde,Business,1200,0.1', 'Klein,Private,300,0'].join('\n')
  await page.locator('input.imp-file').setInputFiles({ name: 'faelle.csv', mimeType: 'text/csv', buffer: Buffer.from(csv) })

  await expect(page.locator('.imp-lane-in .imp-card')).toHaveCount(2)
  await expect(page.locator('.imp-note')).toContainText('importiert')

  await page.getByRole('button', { name: /Durchlaufen lassen/ }).click()
  await expect(page.locator('.imp-lane-store .imp-card')).toHaveCount(2, { timeout: 15_000 })
  // Each asserted case lands with a pass/fail badge; the summary reports the tally.
  await expect(page.locator('.imp-lane-store .imp-badge')).toHaveCount(2)
  await expect(page.locator('.imp-note')).toContainText('bestanden')
})

test('import: the CSV template downloads shaped to the model', async ({ page }) => {
  await page.goto('/')
  await page.getByText('Discount', { exact: true }).first().click()
  await page.locator('#modeImport').click()

  const [download] = await Promise.all([page.waitForEvent('download'), page.getByRole('button', { name: 'Vorlage · CSV' }).click()])
  expect(download.suggestedFilename()).toContain('testfaelle.csv')
})
