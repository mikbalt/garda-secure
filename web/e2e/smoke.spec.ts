import { expect, openAs, seriousA11yViolations, test, type DemoRole } from './fixtures'

/** Key pages per role (lib/roles.ts). */
const PAGES: Record<DemoRole, string[]> = {
  admin: [
    '/',
    '/attendance',
    '/attendance?tab=review',
    '/patrol',
    '/patrol?tab=sessions',
    '/patrol?tab=routes',
    '/patrol?view=plan',
    '/incidents',
    '/incidents?tab=sos',
    '/schedules',
    '/guards',
    '/reports',
    '/reports?tab=compliance',
    '/reports?tab=incidents',
    '/settings',
    '/settings?tab=rules',
    '/settings?tab=platform',
    '/setup',
    '/audit',
    '/users',
    '/settings?tab=devices',
  ],
  supervisor: ['/', '/attendance', '/patrol', '/patrol?view=plan', '/incidents', '/incidents?tab=visitors', '/schedules', '/guards', '/reports', '/reports?tab=compliance'],
  hr: ['/', '/attendance', '/schedules', '/guards', '/reports'],
  'client-readonly': ['/', '/attendance', '/patrol', '/patrol?view=plan', '/incidents', '/reports', '/reports?tab=compliance'],
  'platform-admin': ['/settings?tab=channels', '/settings?tab=ops', '/settings?tab=opslog', '/settings?tab=backups'],
}

for (const [role, pages] of Object.entries(PAGES) as [DemoRole, string[]][]) {
  test.describe(`role ${role}`, () => {
    for (const path of pages) {
      test(`${path} renders without errors and passes axe`, async ({ page, problems }) => {
        await openAs(page, role, path)
        await expect(page.getByRole('heading', { level: 1 })).toBeVisible()
        // No horizontal page scroll at 390 px.
        const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)
        expect(overflow, 'horizontal overflow').toBeLessThanOrEqual(1)
        expect(await seriousA11yViolations(page)).toEqual([])
        expect(problems).toEqual([])
      })
    }
  })
}
