import { test, expect } from '@playwright/test'

// Guards audit finding H1: values rendered into quoted attributes (value="…")
// must be fully HTML-escaped, so a payload containing a double quote can neither
// break out of the attribute (attribute injection / stored XSS) nor corrupt a
// legitimate value that contains quotes (e.g. a FEEL string literal "Winter").
// The flow designer's name field is rendered back as value="${esc(draft.flow)}",
// which is exactly the path that was unsafe before escapeHtml gained quote
// escaping.
test('flow designer: quote payload in a field does not inject markup', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('#modelList .model-item').first()).toBeVisible()

  await page.locator('#newFlow').click()
  await expect(page.locator('.flow-editor')).toBeVisible()

  const payload = '"><img src=x onerror="window.__xss=1">'
  await page.locator('#feFlowName').fill(payload)

  // Force a full re-render of the editor markup (adding an input rebuilds it),
  // so the field value passes back through the innerHTML template + esc().
  await page.locator('#feAddInput').click()

  // No element was injected, and the inline handler never ran.
  await expect(page.locator('.flow-editor img')).toHaveCount(0)
  expect(await page.evaluate(() => (window as unknown as { __xss?: number }).__xss)).toBeUndefined()

  // A benign value carrying double quotes survives intact through the attribute.
  await page.locator('#feFlowName').fill('season "Winter"')
  await page.locator('#feAddInput').click()
  await expect(page.locator('#feFlowName')).toHaveValue('season "Winter"')
})
