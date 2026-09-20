import { test, expect, type Page } from '@playwright/test'

// Code completion surfaces the in-scope variables and the engine's built-in
// functions. The dropdown pops up under the caret as you type a word, or on
// Ctrl/Cmd+Space — never merely from entering or clicking a field.

async function waitForEngine(page: Page): Promise<void> {
  await page.waitForFunction(() => {
    const fn = (window as unknown as { temisFeelBuiltins?: () => unknown[] }).temisFeelBuiltins
    return typeof fn === 'function' && fn().length > 0
  })
}

async function openDecision(page: Page, model: string, elementId: string): Promise<void> {
  await page.goto('/')
  await page.getByText(model, { exact: true }).first().click()
  await page.locator(`[data-element-id="${elementId}"]`).first().dblclick()
}

test('the engine exposes its builtin catalog to the page', async ({ page }) => {
  await openDecision(page, 'Discount', 'id_discount')
  await waitForEngine(page)
  const n = await page.evaluate(
    () => (window as unknown as { temisFeelBuiltins: () => unknown[] }).temisFeelBuiltins().length,
  )
  expect(n).toBeGreaterThan(50)
})

test('the dropdown stays closed on focus and opens on typing', async ({ page }) => {
  await openDecision(page, 'Discount', 'id_discount')
  await expect(page.locator('.dt-overlay')).toBeVisible()
  await waitForEngine(page)

  // Focusing an (empty) output cell must NOT open the dropdown.
  const cell = page.locator('.dt-cell-out').first()
  await cell.click()
  await expect(page.locator('.cc-pop')).toHaveCount(0)

  // Typing a word opens it; it offers all three item kinds.
  await cell.pressSequentially('sub')
  const pop = page.locator('.cc-pop')
  await expect(pop).toBeVisible()
  await expect(pop.locator('.cc-function').first()).toBeVisible()
})

test('Ctrl+Space opens the dropdown on an empty field', async ({ page }) => {
  await openDecision(page, 'Discount', 'id_discount')
  await waitForEngine(page)
  const cell = page.locator('.dt-cell-out').first()
  await cell.click()
  await expect(page.locator('.cc-pop')).toHaveCount(0)
  await cell.press('Control+Space')
  const pop = page.locator('.cc-pop')
  await expect(pop).toBeVisible()
  await expect(pop.locator('.cc-variable').first()).toBeVisible()
  await expect(pop.locator('.cc-function').first()).toBeVisible()
  await expect(pop.locator('.cc-keyword').first()).toBeVisible()
})

test('decision-table cell filters and inserts on typing', async ({ page }) => {
  await openDecision(page, 'Discount', 'id_discount')
  await waitForEngine(page)
  const cell = page.locator('.dt-cell-out').first()
  await cell.fill('')
  await cell.pressSequentially('subst')
  const pop = page.locator('.cc-pop')
  await expect(pop).toBeVisible()
  const labels = await pop.locator('.cc-label').allInnerTexts()
  expect(labels.length).toBeGreaterThan(0)
  expect(labels.every((l) => l.toLowerCase().startsWith('subst'))).toBeTruthy()
  expect(labels).toContain('substring')
  await page.keyboard.press('Enter')
  await expect(cell).toHaveValue(/^subst.*\(/)
})

test('the in-scope input names are offered as variables', async ({ page }) => {
  await openDecision(page, 'Discount', 'id_discount')
  await waitForEngine(page)
  const cell = page.locator('.dt-cell-out').first()
  await cell.click()
  await cell.press('Control+Space')
  const pop = page.locator('.cc-pop')
  await expect(pop.locator('.cc-label', { hasText: /^Customer Type$/ })).toBeVisible()
  await expect(pop.locator('.cc-label', { hasText: /^Order Total$/ })).toBeVisible()
})

test('decision-table input header offers the decision scope variables', async ({ page }) => {
  await openDecision(page, 'Discount', 'id_discount')
  await waitForEngine(page)
  // Clear the first input-column expression so "Customer Type" is no longer a
  // sibling column name — it can now only come from the decision's scope (its
  // connected input-data nodes). The old editor, which knew only the column
  // names, would not offer it here.
  const head = page.locator('.dt-in .dt-head-field').first()
  await head.fill('')
  await head.press('Control+Space')
  const pop = page.locator('.cc-pop')
  await expect(pop.locator('.cc-label', { hasText: /^Customer Type$/ })).toBeVisible()
  await expect(pop.locator('.cc-label', { hasText: /^Order Total$/ })).toBeVisible()
})

test('decision-table input cell also offers the column value "?"', async ({ page }) => {
  await openDecision(page, 'Discount', 'id_discount')
  await waitForEngine(page)
  const inCell = page.locator('.dt-cell-in').first()
  await inCell.click()
  await inCell.press('Control+Space')
  await expect(page.locator('.cc-pop .cc-label', { hasText: /^\?$/ })).toBeVisible()
})

test('literal expression editor completes in its textarea', async ({ page }) => {
  await openDecision(page, 'Pricing', 'id_net')
  const ta = page.locator('.lit-text')
  await expect(ta).toBeVisible()
  await waitForEngine(page)
  await ta.fill('')
  await ta.pressSequentially('count')
  await expect(page.locator('.cc-pop .cc-label', { hasText: /^count$/ })).toBeVisible()
})

test('the conditional editor completes in its branch fields', async ({ page }) => {
  // The if/then/else fields of a boxed conditional get the same completion as
  // every other FEEL field (routed through attachFeelField).
  await openDecision(page, 'BoxedCollections', 'id_grade')
  const ifField = page.locator('.cond-text').first()
  await expect(ifField).toBeVisible()
  await waitForEngine(page)
  await ifField.fill('')
  await ifField.press('Control+Space')
  const pop = page.locator('.cc-pop')
  await expect(pop).toBeVisible()
  // The in-scope Threshold variable is offered alongside functions and keywords.
  await expect(pop.locator('.cc-label', { hasText: /^Threshold$/ })).toBeVisible()
  await expect(pop.locator('.cc-function').first()).toBeVisible()
})
