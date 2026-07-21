import { test, expect } from '@playwright/test'

// Regression (WP-107): the one-time secret shown after creating (or rotating) a
// key must SURVIVE the key-list refresh. It was rendered inside the list, which
// the immediately-following refresh() wiped — so the admin never got to copy it.
// The fix shows the secret in a dedicated host outside the list. We mock an admin
// session and the key endpoints so the flow runs without a secured server.
test('a newly created key shows its secret and it survives the list refresh', async ({ page }) => {
  await page.route('**/v1/whoami', (route) =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ authEnabled: true, authenticated: true, subject: 'boss', scopes: ['admin'], isAdmin: true }) }),
  )
  await page.route('**/v1/access/public', (route) =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ evaluate: false, static: [], managed: [], persistent: true }) }),
  )
  await page.route('**/v1/keys', (route) => {
    if (route.request().method() === 'POST') {
      return route.fulfill({
        status: 201,
        contentType: 'application/json',
        body: JSON.stringify({ kid: 'k_new', secret: 's3cr3t', bearer: 'k_new.s3cr3t', scopes: ['evaluate'], owner: 'CI' }),
      })
    }
    return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ keys: [], count: 0 }) })
  })

  await page.goto('/')
  const group = page.locator('#groupAccess')
  await expect(group).toBeVisible()

  // Open the create form, pick a scope, give it an owner, create.
  await group.getByRole('button', { name: '+ Neuer Key' }).click()
  await group.locator('.access-create input[type="checkbox"]').first().check()
  await group.locator('.access-create input.access-input').first().fill('CI')
  await group.getByRole('button', { name: 'Erstellen' }).click()

  // The one-time secret is shown AND stays put after the list refresh.
  const secret = group.locator('.access-secret-value')
  await expect(secret).toBeVisible()
  await expect(secret).toHaveText('k_new.s3cr3t')
  // Still there a moment later (the bug wiped it on refresh).
  await page.waitForTimeout(300)
  await expect(secret).toBeVisible()

  // Copy works and the dismiss removes it.
  await group.getByRole('button', { name: 'Kopieren' }).click()
  await expect(group.getByRole('button', { name: 'Kopiert ✓' })).toBeVisible()
  await group.getByRole('button', { name: 'Schließen' }).click()
  await expect(secret).toHaveCount(0)
})
