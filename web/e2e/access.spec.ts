import { test, expect } from '@playwright/test'

// Zugriff section (WP-107, ADR-0028/0035): the sidebar access panel must mount
// against the live server. The e2e temisd runs OPEN (no keys), so whoami reports
// an authenticated admin — the section is visible and its panels render: the
// open-API note, the Public Decisions panel (none configured here) and the
// key-management hint (dormant without -keys-dir). This proves the new session/
// access modules wire up with no runtime errors and read the real endpoints.

test('access section renders against an open server', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('#modelList .model-item').first()).toBeVisible()

  const group = page.locator('#groupAccess')
  await expect(group).toBeVisible()
  await expect(group.locator('.section-title')).toHaveText(/Zugriff/)

  // Open API → the identity block states no auth is configured.
  await expect(group.locator('.access-note').first()).toContainText(/Offene API|keine Authentifizierung/)

  // Public Decisions panel is present and, with nothing configured, says so.
  await expect(group.locator('.access-heading', { hasText: 'Public Decisions' })).toBeVisible()
  await expect(group.getByText(/Keine öffentlich auswertbaren Modelle/)).toBeVisible()

  // API-Keys panel offers the trust-on-first-use bootstrap on an open server.
  await expect(group.locator('.access-heading', { hasText: 'API-Keys' })).toBeVisible()
  await expect(group.getByText(/Dieser Server ist offen/)).toBeVisible()
  const secureBtn = group.getByRole('button', { name: /Admin-Key anlegen & absichern/ })
  await expect(secureBtn).toBeVisible()
  await expect(secureBtn).toBeEnabled()

  // The section collapses via its header toggle.
  await group.locator('#accessToggle').click()
  await expect(group).toHaveAttribute('data-collapsed', 'true')
})

// Per-model public toggle (WP-107, ADR-0035): the toolbar switch opens/closes the
// open model for anonymous evaluation. On the open e2e server the admin view is
// active, so the toggle appears for the loaded model and flips on click. The
// change is in-memory (no -keys-dir) but the round-trip through the API is real.
test('per-model public toggle flips the model state', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('#modelList .model-item').first()).toBeVisible()

  const toggle = page.locator('#publicToggle')
  await expect(toggle).toBeVisible()
  await expect(toggle).toHaveText(/Privat/)

  await toggle.click()
  await expect(toggle).toHaveText(/Öffentlich/)
  await expect(toggle).toHaveClass(/is-public/)

  // The Zugriff → Public Decisions panel stays in sync (live event, no reload).
  await expect(page.locator('#groupAccess .access-public-row').first()).toBeVisible()

  // Toggling back closes it again (round-trips through the real API each time).
  await toggle.click()
  await expect(toggle).toHaveText(/Privat/)
})
