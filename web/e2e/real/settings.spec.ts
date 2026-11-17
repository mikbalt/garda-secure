/**
 * Settings → Lokasi tests (real stack).
 * Covers creating and editing a site via the UI.
 * Uses [E2E] prefix on all created data so it's easy to identify.
 */
import { test, expect, openAs, settle } from './fixtures'
import { E2E } from './api'

test.describe('Settings → Lokasi', () => {
  test('admin dapat membuat lokasi baru', async ({ context, problems }) => {
    const page = await openAs(context, 'admin', '/settings')

    // Open Sites tab if not already active.
    const sitesTab = page.getByRole('button', { name: /lokasi/i })
    if (await sitesTab.isVisible()) await sitesTab.click()

    // Click "Tambah lokasi".
    await page.getByRole('button', { name: /tambah lokasi/i }).click()

    // Modal / form should appear.
    const modal = page.getByRole('dialog')
    await expect(modal).toBeVisible()

    const siteName = `${E2E} Lokasi Playwright ${Date.now()}`
    await modal.getByLabel(/nama/i).fill(siteName)
    await modal.getByLabel(/alamat/i).fill('Jl. Playwright Test No. 1, Jakarta')

    // Pick timezone if there is a selector.
    const tzSelect = modal.getByLabel(/zona waktu/i)
    if (await tzSelect.isVisible()) await tzSelect.selectOption('Asia/Jakarta')

    await modal.getByRole('button', { name: /simpan|tambah/i }).click()

    // Modal should close, new site appears in the list.
    await expect(modal).toBeHidden({ timeout: 10_000 })
    await settle(page)
    await expect(page.getByText(siteName)).toBeVisible()
    expect(problems).toEqual([])
  })

  test('admin dapat mengedit lokasi yang sudah ada', async ({ context, testSite, problems }) => {
    const page = await openAs(context, 'admin', '/settings')

    const sitesTab = page.getByRole('button', { name: /lokasi/i })
    if (await sitesTab.isVisible()) await sitesTab.click()
    await settle(page)

    // Find the seeded [E2E] site and open its edit.
    const siteCard = page.locator('li, [data-testid="site-card"]').filter({ hasText: testSite.name }).first()
    await expect(siteCard).toBeVisible()
    await siteCard.getByRole('button', { name: /edit|ubah|pencil/i }).click()

    const modal = page.getByRole('dialog')
    await expect(modal).toBeVisible()

    const newAddress = 'Jl. Update Playwright, Bandung'
    await modal.getByLabel(/alamat/i).clear()
    await modal.getByLabel(/alamat/i).fill(newAddress)
    await modal.getByRole('button', { name: /simpan/i }).click()

    await expect(modal).toBeHidden({ timeout: 10_000 })
    await settle(page)
    await expect(page.getByText(newAddress)).toBeVisible()
    expect(problems).toEqual([])
  })

  test('supervisor tidak bisa edit lokasi (read-only)', async ({ context, problems }) => {
    const page = await openAs(context, 'supervisor', '/settings')
    // Supervisor should not see an "Tambah lokasi" button.
    await expect(page.getByRole('button', { name: /tambah lokasi/i })).toHaveCount(0)
    expect(problems).toEqual([])
  })
})
