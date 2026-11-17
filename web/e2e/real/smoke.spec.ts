/**
 * Smoke tests: each role can reach their key pages without errors.
 * Uses saved auth state — no Keycloak login prompt.
 */
import { test, expect, openAs } from './fixtures'
import type { RealRole } from './fixtures'

type RolePages = { role: RealRole; paths: string[] }

const CASES: RolePages[] = [
  {
    role: 'admin',
    paths: ['/', '/attendance', '/guards', '/schedules', '/patrol', '/incidents', '/reports', '/settings', '/users', '/audit'],
  },
  {
    role: 'supervisor',
    paths: ['/', '/attendance', '/guards', '/patrol', '/incidents', '/reports'],
  },
  {
    role: 'hr',
    paths: ['/', '/attendance', '/schedules', '/guards', '/reports'],
  },
  {
    role: 'client',
    paths: ['/', '/attendance', '/patrol', '/incidents', '/reports'],
  },
  {
    role: 'platform-admin',
    paths: ['/settings?tab=channels', '/settings?tab=ops', '/settings?tab=backups'],
  },
]

for (const { role, paths } of CASES) {
  test.describe(`role: ${role}`, () => {
    for (const urlPath of paths) {
      test(`${urlPath} loads`, async ({ context, problems }) => {
        const page = await openAs(context, role, urlPath)
        await expect(page.locator('main h1').first()).toBeVisible()
        // No horizontal overflow at 1280px
        const overflow = await page.evaluate(
          () => document.documentElement.scrollWidth - document.documentElement.clientWidth
        )
        expect(overflow, 'horizontal overflow').toBeLessThanOrEqual(2)
        expect(problems).toEqual([])
      })
    }
  })
}
