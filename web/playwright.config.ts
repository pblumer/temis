import { defineConfig, devices } from '@playwright/test'

// End-to-end tests for the modeler frontend. They drive a real Chromium against
// the embedded app served by `temisd`, so the wasm FEEL engine and the built
// dist are exercised exactly as a user would.
//
// The browser is the one Playwright manages, unless PLAYWRIGHT_CHROMIUM_PATH
// points at a preinstalled Chromium (handy in sandboxes that ship one).
const PORT = Number(process.env.TEMIS_E2E_PORT ?? 8099)
const executablePath = process.env.PLAYWRIGHT_CHROMIUM_PATH || undefined

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['github'], ['list']] : 'list',
  use: {
    baseURL: `http://127.0.0.1:${PORT}`,
    trace: 'on-first-retry',
  },
  projects: [
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
        launchOptions: { executablePath, args: ['--no-sandbox'] },
      },
    },
  ],
  // Build temisd (which embeds web/dist) and serve it. cwd is the repo root,
  // resolved relative to this config in web/.
  webServer: {
    command: `go run ./cmd/temisd -addr 127.0.0.1:${PORT} -examples=true -mcp=false`,
    cwd: '..',
    url: `http://127.0.0.1:${PORT}/`,
    timeout: 180_000,
    reuseExistingServer: !process.env.CI,
    stdout: 'pipe',
    stderr: 'pipe',
  },
})
