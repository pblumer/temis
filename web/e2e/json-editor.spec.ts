import { test, expect } from '@playwright/test'

// The deluxe JSON editor (json-editor.ts): every field that coerces its value
// through JSON.parse — here the "Discount" example's numeric Order Total input —
// carries a { } opener that opens a roomy modal with live validation and
// format/compact tools, writing the value back into the field on „Übernehmen".

test('json editor: opener writes a value back, formats and validates', async ({ page }) => {
  await page.goto('/')
  await page.getByText('Discount', { exact: true }).first().click()
  await page.locator('#modeOperate').click()

  // The free-text input carries a { } opener beside it (the closed-enumeration
  // Customer <select> does not).
  const opener = page.locator('.je-open').first()
  await expect(opener).toBeVisible()

  // Seed the field, open the editor — the modal pretty-prints the seeded JSON.
  await page.locator('input.eval-field').fill('{"foo":42}')
  await opener.click()
  const modal = page.locator('.je-modal')
  await expect(modal).toBeVisible()
  const text = modal.locator('.je-text')
  await expect(text).toHaveValue(/"foo": 42/) // indented on open
  await expect(modal.locator('.je-status.is-ok')).toBeVisible()

  // Invalid JSON disables saving and shows the parser error.
  await text.fill('{ not json')
  await expect(modal.locator('.je-status.is-err')).toBeVisible()
  await expect(modal.locator('.dlg-btn-primary')).toBeDisabled()

  // Valid, multi-line JSON: „Übernehmen" writes it back compacted into the field.
  await text.fill('{\n  "amount": 1200,\n  "tier": "gold"\n}')
  await expect(modal.locator('.je-status.is-ok')).toBeVisible()
  await modal.locator('.dlg-btn-primary').click()
  await expect(modal).toBeHidden()
  await expect(page.locator('input.eval-field')).toHaveValue('{"amount":1200,"tier":"gold"}')

  // Reopen and „Formatieren" expands the compact value across lines.
  await opener.click()
  await modal.locator('.je-tool', { hasText: 'Formatieren' }).click()
  await expect(modal.locator('.je-text')).toHaveValue(/\n {2}"amount": 1200/)

  // Esc cancels without touching the field.
  await page.keyboard.press('Escape')
  await expect(modal).toBeHidden()
  await expect(page.locator('input.eval-field')).toHaveValue('{"amount":1200,"tier":"gold"}')
})
