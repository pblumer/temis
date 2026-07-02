import { test, expect, type APIRequestContext } from '@playwright/test'
import { readFileSync } from 'fs'

// The Flows view (WP-97, ADR-0026): a catalog of registered decision flows and a
// studio that draws a flow's steps as a graph and runs it. This drives the full
// stack: it uploads two example models, registers a flow composing them, then
// browses and evaluates it in the browser.

async function uploadModel(request: APIRequestContext, path: string): Promise<string> {
  const xml = readFileSync(path, 'utf8')
  const r = await request.post('/v1/models', { headers: { 'Content-Type': 'application/xml' }, data: xml })
  expect(r.ok(), `upload ${path}`).toBeTruthy()
  return ((await r.json()) as { modelId: string }).modelId
}

test('flows: browse a flow graph and evaluate it with per-step results', async ({ page, request }) => {
  const riskId = await uploadModel(request, '../flow/testdata/risk.dmn')
  const loanId = await uploadModel(request, '../flow/testdata/loan.dmn')
  const descriptor = {
    flow: 'loan-decisioning',
    inputs: [
      { name: 'Credit Score', type: 'number' },
      { name: 'Applicant Age', type: 'number' },
    ],
    steps: [
      { id: 'risk', model: riskId, decision: 'Risk Level', in: { 'Credit Score': 'Credit Score' } },
      { id: 'decide', model: loanId, decision: 'Loan Decision', in: { Risk: 'risk.Risk Level', 'Applicant Age': 'Applicant Age' } },
    ],
    output: { Decision: 'decide.Loan Decision' },
  }
  const reg = await request.post('/v1/flows', { data: descriptor })
  expect(reg.ok(), 'register flow').toBeTruthy()

  await page.goto('/')

  // Flows (L2a) have their own always-visible sidebar section above Modelle (L1) —
  // no mode tab. The catalog lists the registered flow; opening it switches the
  // editor to the studio and hides the DMN modeling palette (read-only view).
  await expect(page.locator('#groupFlows .section-title')).toContainText('Flows')
  await page.locator('.flow-item', { hasText: 'loan-decisioning' }).click()
  await expect(page.locator('.djs-palette')).toBeHidden()

  // The canvas draws the two step decisions (and their input nodes), with no
  // validation warning since both models are loaded.
  const canvas = page.locator('#flowCanvas')
  await expect(canvas).toContainText('Risk Level')
  await expect(canvas).toContainText('Loan Decision')
  await expect(page.locator('.flow-warn')).toHaveCount(0)

  // Fill the flow inputs and evaluate: 750/30 → low risk → approve.
  await page.locator('.flow-input[data-name="Credit Score"]').fill('750')
  await page.locator('.flow-input[data-name="Applicant Age"]').fill('30')
  await page.locator('#flowRun').click()

  // The assembled output and a result badge on each of the two step nodes.
  await expect(page.locator('.flow-out')).toContainText('approve')
  await expect(page.locator('#flowCanvas .node-result')).toHaveCount(2)

  // Trace illumination (WP-98): the wires light up with the values that travelled
  // them (the entered Credit Score of 750 flows into the risk step), the active
  // edges are marked, and the Entscheidungspfad lists the rules that fired.
  await expect(page.locator('#flowCanvas .flow-edge-val').filter({ hasText: '750' })).toHaveCount(1)
  await expect(page.locator('#flowCanvas .djs-connection.flow-active').first()).toBeVisible()
  await expect(page.locator('.flow-trace')).toContainText('Entscheidungspfad')

  // A different input changes the outcome: 550 → high risk → decline. The wire
  // value updates in place to the newly entered score.
  await page.locator('.flow-input[data-name="Credit Score"]').fill('550')
  await page.locator('#flowRun').click()
  await expect(page.locator('.flow-out')).toContainText('decline')
  await expect(page.locator('#flowCanvas .flow-edge-val').filter({ hasText: '550' })).toHaveCount(1)

  // Leaving Flows hides the studio; the DMN modeler chrome returns.
  await page.locator('#modeDesign').click()
  await expect(page.locator('.flow-studio')).toBeHidden()
  await expect(page.locator('.canvas-wrap')).toBeVisible()
})
