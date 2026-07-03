import { test, expect } from '@playwright/test'
import { spawn, type ChildProcess } from 'node:child_process'
import { mkdtempSync, writeFileSync, mkdirSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { createHash } from 'node:crypto'

// The decision catalog (ADR-0034, WP-142) turns the flat model sidebar into a
// namespace tree with layer badges and a tag filter. The catalog is loaded
// read-only from disk at startup, so this spec spins up its own temisd — with a
// models dir and a catalog dir it authors into temp dirs — instead of the shared
// examples server. It drives the real tree end to end.

const PORT = Number(process.env.TEMIS_E2E_CATALOG_PORT ?? 8151)
const BASE = `http://127.0.0.1:${PORT}`

// dmn builds a minimal, valid DMN document with a single decision, so the model
// compiles and lists. namespace here is the DMN XML namespace, unrelated to the
// catalog namespace under test.
const dmn = (id: string, name: string): string =>
  `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" id="${id}" name="${name}" namespace="ex">
  <inputData id="i_${id}" name="Amount"/>
  <decision id="d_${id}" name="${name}">
    <informationRequirement><requiredInput href="#i_${id}"/></informationRequirement>
    <literalExpression><text>Amount</text></literalExpression>
  </decision>
</definitions>`

const modelID = (xml: string): string => 'sha256:' + createHash('sha256').update(xml).digest('hex')

let server: ChildProcess | undefined

test.beforeAll(async () => {
  const modelsDir = mkdtempSync(join(tmpdir(), 'temis-e2e-models-'))
  const catalogDir = mkdtempSync(join(tmpdir(), 'temis-e2e-catalog-'))

  const priceXml = dmn('price', 'Base Price')
  const riskXml = dmn('risk', 'Risk Level')
  writeFileSync(join(modelsDir, 'price.dmn'), priceXml)
  writeFileSync(join(modelsDir, 'risk.dmn'), riskXml)

  const entry = (dir: string, file: string, body: object) => {
    mkdirSync(join(catalogDir, dir), { recursive: true })
    writeFileSync(join(catalogDir, dir, file), JSON.stringify(body))
  }
  entry('domains/pricing', 'base-price.catalog.json', { model: modelID(priceXml), layer: 'L1', tags: ['pii'], status: 'active' })
  entry('domains/risk', 'risk-level.catalog.json', { model: modelID(riskXml), layer: 'L1', status: 'active' })

  server = spawn(
    'go',
    ['run', './cmd/temisd', '-addr', `127.0.0.1:${PORT}`, '-examples=false', '-mcp=false', '-models-dir', modelsDir, '-catalog-dir', catalogDir],
    { cwd: '..', stdio: 'ignore', detached: true },
  )

  const deadline = Date.now() + 150_000
  for (;;) {
    try {
      if ((await fetch(`${BASE}/readyz`)).ok) break
    } catch {
      /* not up yet */
    }
    if (Date.now() > deadline) throw new Error('temisd did not become ready')
    await new Promise((r) => setTimeout(r, 500))
  }
})

test.afterAll(() => {
  // detached: true makes the child a group leader, so a negative pid kills the
  // whole group — including the binary `go run` compiled and exec'd.
  if (server?.pid) {
    try {
      process.kill(-server.pid, 'SIGKILL')
    } catch {
      /* already gone */
    }
  }
})

test('namespace tree, layer badges and the tag filter render from the catalog', async ({ page }) => {
  await page.goto(BASE + '/')

  // The tree renders the catalog namespaces (nested domains/pricing, domains/risk),
  // open by default, with the models as leaves.
  await expect(page.locator('.folder-name', { hasText: 'domains' })).toBeVisible()
  await expect(page.locator('.folder-name', { hasText: 'pricing' })).toBeVisible()
  await expect(page.locator('.folder-name', { hasText: 'risk' })).toBeVisible()
  await expect(page.locator('.model-item', { hasText: 'Base Price' })).toBeVisible()
  await expect(page.locator('.model-item', { hasText: 'Risk Level' })).toBeVisible()

  // Layer badge on the namespace node (both models are L1).
  await expect(page.locator('.ns-layer').first()).toHaveText('L1')

  // The tag filter chip (only "pii" exists) AND-narrows the list to the pricing model.
  const chip = page.locator('.tag-chip', { hasText: 'pii' })
  await expect(chip).toBeVisible()
  await chip.click()
  await expect(page.locator('.model-item', { hasText: 'Base Price' })).toBeVisible()
  await expect(page.locator('.model-item', { hasText: 'Risk Level' })).toHaveCount(0)
  // The now-empty risk namespace disappears with its only model filtered out.
  await expect(page.locator('.folder-name', { hasText: 'risk' })).toHaveCount(0)

  // Toggling the chip off restores the full tree.
  await chip.click()
  await expect(page.locator('.model-item', { hasText: 'Risk Level' })).toBeVisible()
})
