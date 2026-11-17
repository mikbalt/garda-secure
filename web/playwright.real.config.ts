/**
 * Real-stack E2E tests — target the live Docker stack at https://localhost.
 *
 * Prerequisites: `cd infra && docker compose up -d && make keycloak-config`
 * Run:  npx playwright test -c playwright.real.config.ts
 * UI:   npx playwright test -c playwright.real.config.ts --ui
 */
import { defineConfig, devices } from '@playwright/test'
import path from 'path'

export const BASE_URL = process.env.GARDA_URL ?? 'https://localhost'
export const KC_URL = process.env.KC_URL ?? 'http://localhost:8081/auth'
export const REALM = process.env.GARDA_REALM ?? 'securepatrol-client1'
export const API_URL = `${BASE_URL}/api/v1`

export function authFile(role: string) {
  return path.join(__dirname, '.playwright-auth', `${role}.json`)
}

export default defineConfig({
  testDir: 'e2e/real',
  fullyParallel: false, // real DB — run serially to avoid conflicts
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : [['list'], ['html', { open: 'on-failure' }]],
  timeout: 60_000,
  expect: { timeout: 12_000 },

  use: {
    baseURL: BASE_URL,
    ignoreHTTPSErrors: true, // self-signed private CA
    trace: 'retain-on-failure',
    video: 'retain-on-failure',
    screenshot: 'only-on-failure',
    locale: 'id-ID',
    timezoneId: 'Asia/Jakarta',
  },

  projects: [
    // 1. Auth setup: login once per role, save storageState.
    //    Re-run with: npx playwright test -c playwright.real.config.ts --project=setup
    {
      name: 'setup',
      testMatch: 'e2e/real/auth.setup.ts',
      use: { ...devices['Desktop Chrome'], ignoreHTTPSErrors: true },
    },

    // 2. Desktop tests that require an authenticated session.
    {
      name: 'desktop',
      testDir: 'e2e/real',
      testIgnore: ['**/auth.setup.ts'],
      use: { ...devices['Desktop Chrome'], viewport: { width: 1280, height: 900 } },
      dependencies: ['setup'],
    },
  ],
})
