import { defineConfig, devices } from '@playwright/test'

/**
 * Screenshots for the sales / presales guide (docs/sales-guide/img/), taken
 * from the demo build (sample data, no backend or Keycloak). Not a test suite:
 * `npm run e2e` only looks at e2e/.
 *
 *   PW_CHANNEL=chrome npm run guide:shots
 */
const PORT = Number(process.env.PW_PORT ?? 4395)
const channel = process.env.PW_CHANNEL || undefined

export default defineConfig({
  testDir: 'guide',
  fullyParallel: true,
  retries: 0,
  reporter: 'list',
  timeout: 60_000,
  expect: { timeout: 10_000 },
  outputDir: 'test-results/guide',
  use: {
    baseURL: `http://127.0.0.1:${PORT}`,
    locale: 'id-ID',
    timezoneId: 'Asia/Jakarta',
    // The helper hides demo-only chrome and toasts with a style tag.
    bypassCSP: true,
  },
  projects: [
    {
      name: 'guide',
      use: { ...devices['Desktop Chrome'], channel, viewport: { width: 1366, height: 820 }, deviceScaleFactor: 1 },
    },
  ],
  webServer: {
    command: `npm run build:demo && npx vite preview --mode demo --port ${PORT} --strictPort --host 127.0.0.1`,
    url: `http://127.0.0.1:${PORT}`,
    reuseExistingServer: !process.env.CI,
    timeout: 180_000,
  },
})
