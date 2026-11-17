/**
 * Shared Playwright fixtures for real-stack E2E tests.
 *
 * Usage:
 *   import { test, expect, openAs } from './fixtures'
 *
 * Each spec picks the role it needs. Auth state is loaded from
 * .playwright-auth/<role>.json (created by auth.setup.ts).
 */
import { test as base, expect, type BrowserContext, type Page } from '@playwright/test'
import path from 'path'
import { BASE_URL, REALM } from '../../playwright.real.config'
import { getAdminToken, type Site, type Guard, seedSite, seedGuard, cleanupE2ESites } from './api'

export type RealRole = 'admin' | 'supervisor' | 'hr' | 'client' | 'platform-admin'

function authFile(role: RealRole): string {
  return path.join(process.cwd(), '.playwright-auth', `${role}.json`)
}

/** Waits until the page heading is visible and loading skeletons are gone. */
export async function settle(page: Page): Promise<void> {
  await expect(page.locator('main h1').first()).toBeVisible({ timeout: 20_000 })
  await expect(page.locator('main .animate-pulse')).toHaveCount(0, { timeout: 15_000 })
}

/** Opens the app as `role` at `path`, using saved storageState (no login prompt). */
export async function openAs(context: BrowserContext, role: RealRole, urlPath = '/'): Promise<Page> {
  // Load saved auth state.
  await context.storageState({ path: authFile(role) }).catch(() => {
    throw new Error(`Auth state for '${role}' not found. Run the setup project first:\n  npx playwright test -c playwright.real.config.ts --project=setup`)
  })
  // Ensure realm is set so the app doesn't show the picker.
  await context.addInitScript(
    ([realm]) => { try { localStorage.setItem('sp.realm', realm) } catch { /* */ } },
    [REALM] as [string]
  )
  const page = await context.newPage()
  await page.goto(`${BASE_URL}${urlPath}`)
  await settle(page)
  return page
}

// ── Fixtures ─────────────────────────────────────────────────────────────────

type E2EFixtures = {
  /** Admin bearer token for seeding via API (not via UI). */
  adminToken: string
  /** A test site created for this test, deleted after. */
  testSite: Site
  /** A test guard on testSite, deleted after. */
  testGuard: Guard
  /** Console errors, page errors, CSP violations. */
  problems: string[]
}

export const test = base.extend<E2EFixtures>({
  adminToken: async ({}, use) => {
    const token = await getAdminToken()
    await use(token)
  },

  testSite: async ({ adminToken }, use) => {
    const site = await seedSite(adminToken)
    await use(site)
    // Cleanup: guards cascade-delete with site once #34 is implemented.
    // For now we just leave the site (it has an [E2E] prefix so it's identifiable).
  },

  testGuard: async ({ adminToken, testSite }, use) => {
    const guard = await seedGuard(adminToken, testSite.id)
    await use(guard)
  },

  problems: async ({ page }, use) => {
    const problems: string[] = []
    page.on('console', (msg) => {
      if (msg.type() === 'error') problems.push(`console: ${msg.text()}`)
    })
    page.on('pageerror', (err) => problems.push(`pageerror: ${err.message}`))
    await page.addInitScript(() => {
      document.addEventListener('securitypolicyviolation', (e) => {
        console.error(`CSP: ${e.violatedDirective} ${e.blockedURI}`)
      })
    })
    await use(problems)
  },
})

export { expect }
