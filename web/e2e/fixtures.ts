import AxeBuilder from '@axe-core/playwright'
import { test as base, expect, type Page } from '@playwright/test'

export type DemoRole = 'admin' | 'supervisor' | 'hr' | 'client-readonly' | 'platform-admin'

/** Collects console errors, page errors and CSP violations of a page. */
export const test = base.extend<{ problems: string[] }>({
  problems: async ({ page }, provide) => {
    const problems: string[] = []
    page.on('console', (msg) => {
      if (msg.type() === 'error') problems.push(`console: ${msg.text()}`)
    })
    page.on('pageerror', (err) => problems.push(`pageerror: ${err.message}`))
    await page.addInitScript(() => {
      document.addEventListener('securitypolicyviolation', (e) => {
        console.error(`CSP violation: ${e.violatedDirective} ${e.blockedURI}`)
      })
    })
    await provide(problems)
  },
})

export { expect }

/** Opens the demo app as `role` (Indonesian UI unless `lang` is given). */
export async function openAs(page: Page, role: DemoRole, path = '/', lang: 'id' | 'en' = 'id') {
  await page.addInitScript(
    ([r, l]) => {
      try {
        localStorage.setItem('sp.demo.role', r)
        localStorage.setItem('sp.lang', l)
        // Keep the SOS alarm quiet in tests.
        localStorage.setItem('sp.sos.muted', '1')
      } catch {
        /* ignore */
      }
    },
    [role, lang] as const
  )
  await page.goto(path)
  await settle(page)
}

/** Waits until the page heading is shown and loading skeletons are gone. */
export async function settle(page: Page) {
  await expect(page.locator('main h1').first()).toBeVisible()
  await expect(page.locator('main .animate-pulse')).toHaveCount(0, { timeout: 15_000 })
  await quiet(page)
}

/**
 * Waits for finite CSS animations/transitions (dialog slide-in, button colour
 * transitions) to end, so axe measures final colours. Endless ones (live dots) are ignored.
 */
export async function quiet(page: Page) {
  await page.waitForFunction(
    () => document.getAnimations().every((a) => a.playState !== 'running' || a.effect?.getComputedTiming().iterations === Infinity),
    undefined,
    { timeout: 5_000 }
  )
}

/** axe-core scan (WCAG 2.1 A/AA); returns serious + critical violations. */
export async function seriousA11yViolations(page: Page) {
  // Measure final colours: a late data update can start a colour transition
  // (e.g. a setup card switching state) while axe is sampling.
  await page.addStyleTag({ content: '*, *::before, *::after { transition: none !important; animation: none !important; }' })
  await page.evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))))
  const result = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']).analyze()
  return result.violations
    .filter((v) => v.impact === 'serious' || v.impact === 'critical')
    .map((v) => `${v.id} (${v.impact}): ${v.help} — ${v.nodes.slice(0, 3).map((n) => n.target.join(' ')).join(' | ')}`)
}
