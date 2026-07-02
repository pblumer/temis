import { test, expect } from '@playwright/test'

// The sidebar model search (ADR-0016): a live text filter over the model list.
// The more models on the server, the more it earns its keep — so this drives the
// real demo models served by temisd (-examples=true) and checks the filtering,
// diacritic-insensitive matching, match highlighting and the clear affordance.

test('filter the model list from the search box', async ({ page }) => {
  await page.goto('/')
  const items = page.locator('#modelList .model-item')
  await expect(items.first()).toBeVisible()
  const total = await items.count()
  expect(total).toBeGreaterThan(1)

  const search = page.locator('#modelSearch')
  const clear = page.locator('#modelSearchClear')
  await expect(clear).toBeHidden()

  // Typing narrows the list to matches and hides the rest.
  await search.fill('alterskette')
  await expect(items).toHaveCount(1)
  await expect(page.locator('.model-item', { hasText: 'Alterskette (Demo)' })).toBeVisible()
  await expect(page.locator('.model-item', { hasText: 'Begrüßung (Demo)' })).toHaveCount(0)
  // The matched part of the name is highlighted so the reason it shows is obvious.
  await expect(items.first().locator('.model-name-hit')).toHaveText('alterskette', { ignoreCase: true })

  // Matching is diacritic-insensitive: "begru" finds "Begrüßung".
  await search.fill('begru')
  await expect(items).toHaveCount(1)
  await expect(page.locator('.model-item', { hasText: 'Begrüßung (Demo)' })).toBeVisible()

  // Terms match in any order, each as a substring.
  await search.fill('demo alter')
  await expect(items).toHaveCount(1)
  await expect(page.locator('.model-item', { hasText: 'Alterskette (Demo)' })).toBeVisible()

  // A query that matches nothing shows the empty hint, no rows.
  await search.fill('zzz-no-such-model')
  await expect(items).toHaveCount(0)
  await expect(page.locator('#modelList .model-empty')).toBeVisible()

  // The clear button appears while filtering and restores the full list.
  await expect(clear).toBeVisible()
  await clear.click()
  await expect(search).toHaveValue('')
  await expect(clear).toBeHidden()
  await expect(items).toHaveCount(total)
})
