/**
 * Auth setup project — runs once before any real-stack test.
 * Logs in as each role, saves the authenticated browser state to
 * .playwright-auth/<role>.json so test specs don't need to log in.
 *
 * Re-run this whenever tokens expire or passwords change:
 *   npx playwright test -c playwright.real.config.ts --project=setup
 */
import { test as setup, expect } from '@playwright/test'
import path from 'path'
import fs from 'fs'
import { BASE_URL, REALM } from '../../playwright.real.config'

const AUTH_DIR = path.join(process.cwd(), '.playwright-auth')

const USERS: { role: string; email: string; password: string }[] = [
  { role: 'platform-admin', email: 'platform@garda.test', password: 'Garda@Platform1' },
  { role: 'admin',          email: 'admin@garda.test',    password: 'Garda@Admin1' },
  { role: 'supervisor',     email: 'supervisor@garda.test', password: 'Garda@Supervisor1' },
  { role: 'hr',             email: 'hr@garda.test',       password: 'Garda@Hr01' },
  { role: 'client',         email: 'client@garda.test',   password: 'Garda@Client1' },
]

fs.mkdirSync(AUTH_DIR, { recursive: true })

for (const user of USERS) {
  setup(`login as ${user.role}`, async ({ page, context }) => {
    // Set realm in localStorage BEFORE navigating so the app skips the realm picker.
    await context.addInitScript(
      ([realm]) => {
        try { localStorage.setItem('sp.realm', realm) } catch { /* */ }
      },
      [REALM] as [string]
    )

    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' })

    // Keycloak login page loads after the app detects realm + initiates PKCE.
    // URL pattern: /auth/realms/<realm>/protocol/openid-connect/auth
    await page.waitForURL(/\/auth\/realms\//, { timeout: 20_000 })

    // Fill Keycloak credentials.
    // Keycloak uses id="username" and id="password" consistently across versions.
    await page.locator('#username').fill(user.email)
    await page.locator('#password').fill(user.password)
    await page.locator('#kc-login').click()

    // Wait until Keycloak redirects back to the app and the app finishes booting.
    await page.waitForURL(new RegExp(`^${BASE_URL.replace('/', '\\/')}`), { timeout: 20_000 })
    // The app shows a heading once fully loaded.
    await expect(page.locator('main h1').first()).toBeVisible({ timeout: 20_000 })

    // Save the full auth state (cookies + localStorage incl. Keycloak tokens).
    await context.storageState({ path: path.join(AUTH_DIR, `${user.role}.json`) })
    console.log(`  ✓ ${user.role} → .playwright-auth/${user.role}.json`)
  })
}
