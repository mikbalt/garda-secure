/**
 * Guards page tests (real stack).
 * Seeds a site + guard before each test, uses them for assertions.
 */
import { test, expect, openAs, settle } from './fixtures'
import { E2E } from './api'

test.describe('Guards', () => {
  test('admin melihat daftar petugas', async ({ context, testGuard, problems }) => {
    const page = await openAs(context, 'admin', '/guards')
    await expect(page.getByText(testGuard.name)).toBeVisible()
    await expect(page.getByText(testGuard.badge_number)).toBeVisible()
    expect(problems).toEqual([])
  })

  test('admin dapat menambah petugas baru', async ({ context, testSite, problems }) => {
    const page = await openAs(context, 'admin', '/guards')

    await page.getByRole('button', { name: /tambah petugas/i }).click()
    const modal = page.getByRole('dialog')
    await expect(modal).toBeVisible()

    const guardName = `${E2E} Petugas Baru ${Date.now()}`
    await modal.getByLabel(/nama/i).fill(guardName)
    await modal.getByLabel(/badge|nomor/i).fill(`E2E-NEW-${Date.now()}`)

    // Pick the seeded test site.
    const siteSelect = modal.getByLabel(/lokasi/i)
    await siteSelect.selectOption({ label: testSite.name })

    await modal.getByRole('button', { name: /simpan|tambah/i }).click()
    await expect(modal).toBeHidden({ timeout: 10_000 })
    await settle(page)

    await expect(page.getByText(guardName)).toBeVisible()
    expect(problems).toEqual([])
  })

  test('supervisor dapat melihat petugas tapi tidak bisa edit', async ({ context, testGuard, problems }) => {
    const page = await openAs(context, 'supervisor', '/guards')
    await expect(page.getByText(testGuard.name)).toBeVisible()
    // Supervisor should not have add/edit buttons.
    await expect(page.getByRole('button', { name: /tambah petugas/i })).toHaveCount(0)
    expect(problems).toEqual([])
  })
})
