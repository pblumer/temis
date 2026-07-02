import { test, expect, type Page } from '@playwright/test'

// Syntax highlighting renders the FEEL text as coloured token spans in a backdrop
// behind the (transparent) field: functions, variables, keywords, strings and
// numbers each get their own class/colour.

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

test('literal editor colours functions, variables, keywords, strings and numbers', async ({ page }) => {
  await openDecision(page, 'Pricing', 'id_net')
  const ta = page.locator('.lit-text')
  await expect(ta).toBeVisible()
  await waitForEngine(page)

  // Unit Price / Quantity are in-scope variables (Quantity single-word, Unit
  // Price multi-word), abs a built-in function, if/then/else keywords.
  await ta.fill('if Quantity > 10 then abs(Unit Price) else "none"')

  const bd = page.locator('.hl-wrap .hl-content')
  await expect(bd.locator('.hl-kw', { hasText: /^if$/ })).toBeVisible()
  await expect(bd.locator('.hl-kw', { hasText: /^then$/ })).toBeVisible()
  await expect(bd.locator('.hl-var', { hasText: /^Quantity$/ })).toBeVisible()
  await expect(bd.locator('.hl-var', { hasText: /^Unit Price$/ })).toBeVisible()
  await expect(bd.locator('.hl-fn', { hasText: /^abs$/ })).toBeVisible()
  await expect(bd.locator('.hl-num', { hasText: /^10$/ })).toBeVisible()
  await expect(bd.locator('.hl-str', { hasText: 'none' })).toBeVisible()
})

test('the field text is made transparent so only the coloured backdrop shows', async ({ page }) => {
  await openDecision(page, 'Pricing', 'id_net')
  const ta = page.locator('.lit-text.hl-field')
  await expect(ta).toBeVisible()
  await waitForEngine(page)
  const fill = await ta.evaluate((el) => getComputedStyle(el).webkitTextFillColor)
  // Transparent text fill (rgba alpha 0) — the caret and backdrop remain visible.
  expect(fill.replace(/\s/g, '')).toMatch(/rgba?\(0,0,0,0\)|transparent/)
})

test('decision-table cells are highlighted too', async ({ page }) => {
  await openDecision(page, 'Discount', 'id_discount')
  await expect(page.locator('.dt-overlay')).toBeVisible()
  await waitForEngine(page)
  // The input-column headers hold the input expressions, which are in-scope names.
  await expect(page.locator('.dt-in .hl-wrap .hl-content .hl-var', { hasText: /^Customer Type$/ })).toBeVisible()
  // A numeric output cell (e.g. 0.10) is coloured as a number.
  await expect(page.locator('.dt-cell-out').first()).toHaveClass(/hl-field/)
  await expect(page.locator('.dt-out .hl-content .hl-num').first()).toBeVisible()
})
