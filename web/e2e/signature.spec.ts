import { test, expect, type Page } from '@playwright/test'

// The signature/construct hint (src/signature.ts) is a translucent beginner aid
// that floats above a FEEL field while the caret is inside a function call or a
// control-flow construct, spelling out the expected shape with the active part
// picked out. It never edits the field; it appears and disappears with the
// caret's context.

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

test('an if construct shows the if/then/else template above the field', async ({ page }) => {
  await openDecision(page, 'Pricing', 'id_net')
  const ta = page.locator('.lit-text')
  await expect(ta).toBeVisible()
  await waitForEngine(page)

  // No hint on a plain expression.
  await ta.fill('Unit Price * Quantity')
  await ta.click()
  await expect(page.locator('.sig-hint')).toHaveCount(0)

  // Typing an `if` surfaces the whole template with the keywords spelled out.
  await ta.fill('if ')
  await ta.click()
  await page.keyboard.press('End')
  const hint = page.locator('.sig-hint')
  await expect(hint).toBeVisible()
  const kws = await hint.locator('.sig-kw').allInnerTexts()
  expect(kws).toEqual(['if', 'then', 'else'])
  // The caret is right after `if`, so the condition placeholder is the active one.
  await expect(hint.locator('.sig-ph.sig-active')).toHaveText('Bedingung')

  // Once `then` is typed, the active placeholder advances to the then-value.
  await ta.fill('if Quantity > 10 then ')
  await ta.click()
  await page.keyboard.press('End')
  await expect(hint.locator('.sig-ph.sig-active')).toHaveText('Wert')
})

test('a builtin call shows its parameter signature with the active argument lit', async ({ page }) => {
  await openDecision(page, 'Pricing', 'id_net')
  const ta = page.locator('.lit-text')
  await expect(ta).toBeVisible()
  await waitForEngine(page)

  // substring(string, start position, length?) — the caret sits on the first arg.
  await ta.fill('substring(')
  await ta.click()
  await page.keyboard.press('End')
  const hint = page.locator('.sig-hint')
  await expect(hint).toBeVisible()
  await expect(hint.locator('.sig-fn')).toHaveText('substring')
  const first = await hint.locator('.sig-ph.sig-active').first().innerText()
  expect(first.length).toBeGreaterThan(0)

  // Closing the call removes the hint again (the caret is no longer inside it).
  await ta.fill('substring("hi", 1)')
  await ta.click()
  await page.keyboard.press('End')
  await expect(page.locator('.sig-hint')).toHaveCount(0)
})

test('the hint hides when the field loses focus', async ({ page }) => {
  await openDecision(page, 'Pricing', 'id_net')
  const ta = page.locator('.lit-text')
  await expect(ta).toBeVisible()
  await waitForEngine(page)

  await ta.fill('if ')
  await ta.click()
  await page.keyboard.press('End')
  await expect(page.locator('.sig-hint')).toBeVisible()

  // Blurring the field (Escape keeps the modal but the textarea keeps focus, so
  // click the type dropdown instead) tears the hint down.
  await page.locator('.lit-type').focus()
  await expect(page.locator('.sig-hint')).toHaveCount(0)
})
