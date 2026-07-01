import { test, expect } from '@playwright/test'

// clio connection indicator (ADR-0030): the toolbar badge must reflect the real
// backend state from GET /v1/status. The e2e temisd runs with no clio token, so
// the audit sink is off — the badge must render and report exactly that (grey
// "clio aus"), proving the indicator is wired to the live status, not faked.

test('clio badge shows the sink is off when no clio is configured', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('#modelList .model-item').first()).toBeVisible()

  const badge = page.locator('#clioStatus')
  // The badge appears once the first /v1/status poll resolves. The exact class
  // list is the grey "off" state — neither conn-ok (green) nor conn-bad (red).
  await expect(badge).toBeVisible()
  await expect(badge).toHaveClass('conn-badge conn-off')
  await expect(badge.locator('.conn-label')).toHaveText('clio aus')
  // The tooltip points the operator at the one opt-in step, and never leaks a secret.
  await expect(badge).toHaveAttribute('title', /TEMIS_CLIO_TOKEN/)
})
