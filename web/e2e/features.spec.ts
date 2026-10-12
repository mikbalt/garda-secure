import { expect, openAs, settle, test } from './fixtures'

// Feature checks run once, at desktop width (the mobile project skips @desktop).
test.describe('hardening features (demo) @desktop', () => {
  test('SOS banner shows alerts of every site, independent of the selected site', async ({ page, problems }) => {
    await openAs(page, 'supervisor', '/')
    const banner = page.getByRole('region', { name: 'Peringatan SOS' })
    await expect(banner).toBeVisible()
    // Demo world: one open SOS at each of the two sites.
    await expect(banner.getByText('Gudang Cikarang')).toBeVisible()
    await expect(banner.getByText('Kantor Pusat Sudirman')).toBeVisible()
    // The live region holds text only.
    const alertRegion = page.locator('[role="alert"]').filter({ hasText: 'SOS' })
    await expect(alertRegion.locator('button')).toHaveCount(0)
    // Sound toggle is a pressed button (muted in tests).
    await expect(banner.getByRole('button', { name: 'Suara mati' })).toHaveAttribute('aria-pressed', 'true')
    expect(problems).toEqual([])
  })

  test('platform operator lands on install-wide settings', async ({ page, problems }) => {
    await openAs(page, 'platform-admin', '/')
    await expect(page).toHaveURL(/\/settings/)
    const nav = page.getByRole('navigation', { name: 'Pengaturan' })
    await expect(nav.getByRole('button', { name: /Alert operasional/ }).first()).toBeVisible()
    await expect(nav.getByRole('button', { name: /Lokasi/ })).toHaveCount(0)
    await expect(page.getByRole('link', { name: 'Hari ini' })).toHaveCount(0)
    expect(problems).toEqual([])
  })

  test('realm admin sees an explanation instead of install-wide settings', async ({ page, problems }) => {
    await openAs(page, 'admin', '/settings?tab=channels')
    await expect(page.getByRole('heading', { name: 'Pengaturan seluruh instalasi' })).toBeVisible()
    await expect(page.getByRole('textbox', { name: /Token/ })).toHaveCount(0)
    expect(problems).toEqual([])
  })

  test('channel settings ask to re-enter the secret when the destination changes', async ({ page, problems }) => {
    await openAs(page, 'platform-admin', '/settings?tab=channels')
    const wa = page.locator('section', { has: page.getByRole('heading', { name: 'WhatsApp' }) })
    const url = wa.getByLabel('URL API')
    await url.fill('https://graph.facebook.com/v19.0')
    await wa.getByRole('button', { name: 'Simpan' }).click()
    const token = wa.getByLabel(/Token/)
    await expect(token).toBeFocused()
    await expect(token).toHaveAttribute('aria-invalid', 'true')
    await expect(wa.getByText(/Masukkan ulang kata sandi \/ token/)).toBeVisible()
    await token.fill('new-secret-token')
    await wa.getByRole('button', { name: 'Simpan' }).click()
    await expect(page.getByText('Tersimpan.').first()).toBeVisible()
    expect(problems).toEqual([])
  })

  test('audit log: filter, open an entry and see before/after', async ({ page, problems }) => {
    await openAs(page, 'admin', '/audit')
    await page.getByLabel('Bagian').selectOption('guards')
    await page.getByRole('button', { name: 'Terapkan' }).click()
    await settle(page)
    const row = page.getByRole('row').filter({ hasText: 'sebelum/sesudah' }).filter({ hasText: '/api/v1/guards/:id' })
    await row.getByRole('button', { name: /Buka detail/ }).click()
    const drawer = page.getByRole('dialog', { name: 'Catatan audit' })
    await expect(drawer).toBeVisible()
    await expect(drawer.getByRole('rowheader', { name: /status/ })).toBeVisible()
    await expect(drawer.getByText('inactive')).toBeVisible()
    await expect(drawer.getByText('active', { exact: true }).first()).toBeVisible()
    expect(problems).toEqual([])
  })

  test('flags are explained in the review queue', async ({ page, problems }) => {
    await openAs(page, 'supervisor', '/attendance?tab=review')
    const flag = page.getByRole('button', { name: /Tanda: Waktu mundur/ }).first()
    await flag.click()
    await expect(page.getByText(/sedang online setelah waktu yang diklaim/)).toBeVisible()
    expect(problems).toEqual([])
  })

  test('client-readonly does not get selfie buttons', async ({ page, problems }) => {
    await openAs(page, 'client-readonly', '/attendance')
    await expect(page.getByRole('columnheader', { name: 'Swafoto' })).toHaveCount(0)
    await expect(page.getByRole('button', { name: /Lihat foto/ })).toHaveCount(0)
    expect(problems).toEqual([])
  })

  test('API errors are translated (weak PIN)', async ({ page, problems }) => {
    await openAs(page, 'admin', '/guards')
    await page.getByRole('button', { name: /^Ganti PIN — / }).first().click()
    const dialog = page.getByRole('dialog')
    const inputs = dialog.locator('input[inputmode="numeric"]')
    await inputs.nth(0).fill('1234')
    await inputs.nth(1).fill('1234')
    await dialog.locator('button[type="submit"]').click()
    await expect(dialog.getByText('PIN terlalu mudah ditebak.')).toBeVisible()
    expect(problems).toEqual([])
  })
})
