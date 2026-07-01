import { test, expect, type Page } from '@playwright/test'

// Code completion must appear everywhere FEEL can be entered, surfacing the
// in-scope variables and the engine's built-in functions (src/complete.ts). The
// decision-table cell (an <input>) and the literal editor (a <textarea>) are the
// two distinct field types; the BKM editor shares the literal editor's wiring.

// The engine's builtin catalog is exposed by the wasm module once it loads; the
// function completions depend on it, so wait for it before asserting on them.
async function waitForEngine(page: Page): Promise<void> {
  await page.waitForFunction(() => {
    const fn = (window as unknown as { temisFeelBuiltins?: () => unknown[] }).temisFeelBuiltins
    return typeof fn === 'function' && fn().length > 0
  })
}

// Open an example model from the sidebar, then double-click a decision shape to
// open its editor overlay.
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

test('decision-table cell completes variables, functions and keywords', async ({ page }) => {
  await openDecision(page, 'Discount', 'id_discount')
  await expect(page.locator('.dt-overlay')).toBeVisible()
  await waitForEngine(page)

  // Focusing an output cell reveals the dropdown with all three item kinds.
  const cell = page.locator('.dt-cell-out').first()
  await cell.click()
  const pop = page.locator('.cc-pop')
  await expect(pop).toBeVisible()
  await expect(pop.locator('.cc-variable').first()).toBeVisible()
  await expect(pop.locator('.cc-function').first()).toBeVisible()
  await expect(pop.locator('.cc-keyword').first()).toBeVisible()

  // The decision's in-scope input names are offered as variables.
  await expect(pop.locator('.cc-label', { hasText: /^Customer Type$/ })).toBeVisible()
  await expect(pop.locator('.cc-label', { hasText: /^Order Total$/ })).toBeVisible()

  // Typing filters to matching items, and Enter inserts the chosen one.
  await cell.fill('')
  await cell.pressSequentially('subst')
  const labels = await pop.locator('.cc-label').allInnerTexts()
  expect(labels.length).toBeGreaterThan(0)
  expect(labels.every((l) => l.toLowerCase().startsWith('subst'))).toBeTruthy()
  expect(labels).toContain('substring')
  await page.keyboard.press('Enter')
  await expect(cell).toHaveValue(/^subst.*\(/)
})

test('decision-table input cell also offers the column value "?"', async ({ page }) => {
  await openDecision(page, 'Discount', 'id_discount')
  await waitForEngine(page)
  const inCell = page.locator('.dt-cell-in').first()
  await inCell.click()
  const pop = page.locator('.cc-pop')
  await expect(pop).toBeVisible()
  await expect(pop.locator('.cc-label', { hasText: /^\?$/ })).toBeVisible()
})

test('literal expression editor completes in its textarea', async ({ page }) => {
  await openDecision(page, 'Pricing', 'id_net')
  const ta = page.locator('.lit-text')
  await expect(ta).toBeVisible()
  await waitForEngine(page)

  // Empty field + focus → the full list of what is available is shown.
  await ta.fill('')
  await ta.click()
  const pop = page.locator('.cc-pop')
  await expect(pop).toBeVisible()
  await expect(pop.locator('.cc-function').first()).toBeVisible()
  await expect(pop.locator('.cc-label', { hasText: /^Unit Price$/ })).toBeVisible()

  // The built-in `count` is reachable by typing its name.
  await ta.fill('')
  await ta.pressSequentially('count')
  await expect(pop.locator('.cc-label', { hasText: /^count$/ })).toBeVisible()
})
