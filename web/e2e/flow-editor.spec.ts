import { test, expect, type APIRequestContext } from '@playwright/test'
import { readFileSync } from 'fs'

// The Flow Designer (WP-116, ADR-0026): create/design a decision flow visually,
// then test and register it — without hand-writing the JSON descriptor. This drives
// the full stack: it uploads the two example models over the API, then builds the
// composing flow entirely through the designer UI (inputs, steps with model +
// decision pickers, input wiring, output mapping), tests the draft inline, checks
// it against the loaded models, and registers it into the studio.

async function uploadModel(request: APIRequestContext, path: string): Promise<string> {
  const xml = readFileSync(path, 'utf8')
  const r = await request.post('/v1/models', { headers: { 'Content-Type': 'application/xml' }, data: xml })
  expect(r.ok(), `upload ${path}`).toBeTruthy()
  return ((await r.json()) as { modelId: string }).modelId
}

test('flow designer: build, test and register a flow visually', async ({ page, request }) => {
  const riskId = await uploadModel(request, '../flow/testdata/risk.dmn')
  const loanId = await uploadModel(request, '../flow/testdata/loan.dmn')

  await page.goto('/')

  // Enter the designer from the FLOWS sidebar section (+ button). The DMN modeling
  // palette hides — this is a flow activity, not a model one.
  await page.locator('#newFlow').click()
  await expect(page.locator('.flow-editor')).toBeVisible()
  await expect(page.locator('.djs-palette')).toBeHidden()

  await page.locator('#feFlowName').fill('ui-loan')

  // Declared inputs: Credit Score, Applicant Age (both number).
  await page.locator('#feAddInput').click()
  await page.locator('.fe-name[data-i="0"]').fill('Credit Score')
  await page.locator('.fe-type[data-i="0"]').selectOption('number')
  await page.locator('#feAddInput').click()
  await page.locator('.fe-name[data-i="1"]').fill('Applicant Age')
  await page.locator('.fe-type[data-i="1"]').selectOption('number')

  // Step 0 → the risk model's "Risk Level" decision. Naming the step "risk" makes
  // its output referenceable as "risk.Risk Level" downstream.
  await page.locator('.fe-step-id[data-si="0"]').fill('risk')
  await page.locator('.fe-step-id[data-si="0"]').blur()
  await page.locator('.fe-step-model[data-si="0"]').selectOption(riskId)
  await page.locator('.fe-step-decision[data-si="0"]').selectOption('Risk Level')
  // Selecting a decision auto-wires its inputs, referencing same-named flow inputs:
  // Credit Score ← Credit Score.
  await expect(page.locator('.fe-step[data-si="0"] .fe-wire-key').first()).toHaveValue('Credit Score')
  await expect(page.locator('.fe-step[data-si="0"] .fe-wire-expr').first()).toHaveValue('Credit Score')

  // Step 1 → the loan model's "Loan Decision".
  await page.locator('#feAddStep').click()
  await page.locator('.fe-step-id[data-si="1"]').fill('decide')
  await page.locator('.fe-step-id[data-si="1"]').blur()
  await page.locator('.fe-step-model[data-si="1"]').selectOption(loanId)
  await page.locator('.fe-step-decision[data-si="1"]').selectOption('Loan Decision')
  await expect(page.locator('.fe-step[data-si="1"] .fe-wire')).toHaveCount(2)
  // Wire the composed input "Risk" to the earlier step's output; "Applicant Age" was
  // auto-wired to the same-named flow input.
  const decideWires = page.locator('.fe-step[data-si="1"] .fe-wire')
  const wireCount = await decideWires.count()
  for (let i = 0; i < wireCount; i++) {
    const key = await decideWires.nth(i).locator('.fe-wire-key').inputValue()
    if (key === 'Risk') await decideWires.nth(i).locator('.fe-wire-expr').fill('risk.Risk Level')
  }

  // Output mapping: Decision ← decide.Loan Decision.
  await page.locator('#feAddOutput').click()
  await page.locator('.fe-out-key[data-i="0"]').fill('Decision')
  await page.locator('.fe-out-expr[data-i="0"]').fill('decide.Loan Decision')

  // The live preview draws both step decisions as a cross-model graph.
  await expect(page.locator('.flow-editor-canvas')).toContainText('Risk Level')
  await expect(page.locator('.flow-editor-canvas')).toContainText('Loan Decision')

  // Test the draft inline (POST /v1/flow/evaluate, not registered): 750/30 → approve,
  // and the preview illuminates with a result badge on each step.
  await page.locator('#feTest').click()
  await page.locator('.fe-test-in[data-name="Credit Score"]').fill('750')
  await page.locator('.fe-test-in[data-name="Applicant Age"]').fill('30')
  await page.locator('#feRunTest').click()
  await expect(page.locator('.fe-test-out')).toContainText('approve')
  await expect(page.locator('.flow-editor-canvas .node-result')).toHaveCount(2)

  // Check against the loaded models: no diagnostics.
  await page.locator('#feCheck').click()
  await expect(page.locator('.fe-note-ok')).toBeVisible()

  // Register & open: the flow appears in the catalog and opens in the studio.
  await page.locator('#feRegister').click()
  await expect(page.locator('.flow-studio')).toBeVisible()
  await expect(page.locator('.flow-item', { hasText: 'ui-loan' })).toBeVisible()
  await expect(page.locator('#flowCanvas')).toContainText('Loan Decision')

  // Round-trip: the studio's "Bearbeiten" reopens the flow in the designer prefilled.
  await page.locator('#flowEdit').click()
  await expect(page.locator('.flow-editor')).toBeVisible()
  await expect(page.locator('#feFlowName')).toHaveValue('ui-loan')
})
