import { test, expect } from '@playwright/test'

// Guards audit finding H3: on a server with no models, boot() must NOT return
// early. Before the fix it bailed out before wiring showModel, the search, the
// folder actions and the flows catalog — so on a fresh server the sidebar was
// dead and "Neues Modell" threw a ReferenceError (showModel used before its
// declaration). We simulate the empty server by returning an empty model list,
// then assert the shell is fully wired.
test('empty server: boot wires the shell, no early return', async ({ page }) => {
  // Force the "no models" boot path regardless of preloaded examples.
  await page.route('**/v1/models', async (route) => {
    if (route.request().method() === 'GET') {
      await route.fulfill({ status: 200, contentType: 'application/json', body: '[]' })
    } else {
      await route.continue()
    }
  })

  const errors: string[] = []
  page.on('pageerror', (e) => errors.push(String(e)))

  await page.goto('/')

  // The empty-state message renders — proof boot ran past the old early return.
  await expect(page.locator('#modelList .model-empty')).toContainText('Keine Modelle')

  // The folder action is wired only AFTER the point boot used to bail at, so a
  // working "Neuer Ordner" dialog proves the tail of boot() executed (H3).
  await page.locator('#newFolder').click()
  await expect(page.locator('.dlg-modal')).toBeVisible()
  await page.keyboard.press('Escape')

  // The new-model action is wired too.
  await page.locator('#newModel').click()
  await expect(page.locator('.dlg-modal')).toBeVisible()

  expect(errors, 'boot must not throw (e.g. showModel TDZ ReferenceError)').toEqual([])
})
