import { defineConfig, devices } from '@playwright/test'

/**
 * E2E smoke tests (WEB-5) against the demo build (`npm run build:demo`),
 * served by `vite preview` with the production Content-Security-Policy
 * (vite.config.ts `preview.headers`), so CSP violations fail the run too.
 *
 * Browser: CI installs Playwright's Chromium; locally set
 * PW_CHANNEL=chrome to use the installed Google Chrome instead.
 */
const PORT = Number(process.env.PW_PORT ?? 4173)
const channel = process.env.PW_CHANNEL || undefined

export default defineConfig({
  testDir: 'e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: process.env.CI ? 2 : undefined,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  timeout: 60_000,
  expect: { timeout: 10_000 },
  use: {
    baseURL: `http://127.0.0.1:${PORT}`,
    trace: 'retain-on-failure',
    locale: 'id-ID',
    timezoneId: 'Asia/Jakarta',
  },
  projects: [
    // Tests tagged @mobile / @desktop only make sense at one width.
    { name: 'desktop', grepInvert: /@mobile/, use: { ...devices['Desktop Chrome'], channel, viewport: { width: 1280, height: 900 } } },
    {
      name: 'mobile',
      grepInvert: /@desktop/,
      use: { ...devices['Desktop Chrome'], channel, viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: false },
    },
  ],
  webServer: {
    command: `npm run build:demo && npx vite preview --mode demo --port ${PORT} --strictPort --host 127.0.0.1`,
    url: `http://127.0.0.1:${PORT}`,
    reuseExistingServer: !process.env.CI,
    timeout: 180_000,
  },
})
