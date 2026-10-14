import type { Page } from '@playwright/test'
import { expect, openAs, quiet, seriousA11yViolations, test } from './fixtures'

/**
 * The preview has no reachable push service: make subscribe() fail like an
 * offline browser, so demo mode takes its simulated-subscription path
 * deterministically.
 */
async function offlinePushService(page: Page) {
  await page.addInitScript(() => {
    if ('PushManager' in window) {
      PushManager.prototype.subscribe = () => Promise.reject(new DOMException('push service not available', 'AbortError'))
    }
  })
}

test.describe('push notifications (demo)', () => {
  test('the user menu opens the notifications page @desktop', async ({ page, problems }) => {
    await openAs(page, 'hr', '/')
    await page.getByRole('button', { name: 'Menu pengguna' }).click()
    await page.getByRole('group', { name: 'Menu pengguna' }).getByRole('link', { name: 'Notifikasi' }).click({ timeout: 5_000 })
    await expect(page).toHaveURL(/\/notifications$/)
    await expect(page.getByRole('heading', { level: 1, name: 'Notifikasi' })).toBeVisible()
    expect(problems).toEqual([])
  })

  test('turn on, send a test, turn off again', async ({ page, context, problems }) => {
    await context.grantPermissions(['notifications'])
    await offlinePushService(page)
    await openAs(page, 'admin', '/notifications')

    const device = page.locator('section').filter({ has: page.getByRole('heading', { name: 'Perangkat ini', exact: true }) })
    await expect(device.getByText('Mati', { exact: true })).toBeVisible()
    await device.getByRole('button', { name: 'Aktifkan notifikasi' }).click()
    await expect(device.getByText('Aktif', { exact: true })).toBeVisible()
    await expect(page.getByText('Notifikasi aktif di perangkat ini.').first()).toBeVisible()

    // The new device is listed and marked as this one (2 sample devices + this one).
    const list = page.locator('section').filter({ has: page.getByRole('heading', { name: 'Perangkat Anda' }) })
    await expect(list.getByRole('listitem')).toHaveCount(3)
    await expect(list.getByRole('listitem').first().getByText('Perangkat ini')).toBeVisible()

    await page.getByRole('button', { name: 'Kirim tes' }).click()
    await expect(page.getByText('Tes dikirim ke 3 perangkat.').first()).toBeVisible()
    // The button is dimmed while the request runs; axe must see its final colours.
    await expect(page.getByRole('button', { name: 'Kirim tes' })).toBeEnabled()

    await quiet(page)
    expect(await seriousA11yViolations(page)).toEqual([])

    await device.getByRole('button', { name: 'Matikan di perangkat ini' }).click()
    await expect(device.getByText('Mati', { exact: true })).toBeVisible()
    await expect(list.getByRole('listitem')).toHaveCount(2)
    expect(problems).toEqual([])
  })

  test('preferences: only the role’s alerts, saved per switch', async ({ page, problems }) => {
    await openAs(page, 'hr', '/notifications')
    await expect(page.getByRole('region', { name: 'Absensi' })).toBeVisible()
    await expect(page.getByRole('region', { name: 'SOS & insiden' })).toHaveCount(0)
    await expect(page.getByRole('switch')).toHaveCount(3)

    const late = page.getByRole('switch', { name: 'Petugas terlambat' })
    await expect(late).toHaveAttribute('aria-checked', 'false')
    await late.click()
    await expect(late).toHaveAttribute('aria-checked', 'true')
    await quiet(page)
    expect(await seriousA11yViolations(page)).toEqual([])
    expect(problems).toEqual([])
  })

  test('remove another device after confirming', async ({ page, problems }) => {
    await openAs(page, 'supervisor', '/notifications')
    await page.getByRole('button', { name: 'Hapus Firefox on Windows' }).click()
    const dialog = page.getByRole('dialog', { name: 'Hapus perangkat ini?' })
    await expect(dialog).toBeVisible()
    await dialog.getByRole('button', { name: 'Hapus' }).click()
    await expect(page.getByText('Perangkat dihapus.').first()).toBeVisible()
    await expect(page.getByRole('button', { name: 'Hapus Firefox on Windows' })).toHaveCount(0)
    expect(problems).toEqual([])
  })

  test('dashboard hint for supervisors, dismissed for good', async ({ page, context, problems }) => {
    await context.grantPermissions(['notifications'])
    await openAs(page, 'supervisor', '/')
    const hint = page.getByRole('complementary', { name: 'Terima peringatan SOS meski GARDA ditutup' })
    await expect(hint).toBeVisible()
    await expect(hint.getByRole('link', { name: /Atur notifikasi/ })).toHaveAttribute('href', '/notifications')
    await hint.getByRole('button', { name: 'Nanti saja' }).click()
    await expect(hint).toHaveCount(0)
    await page.reload()
    await expect(page.locator('main h1').first()).toBeVisible()
    await expect(page.getByRole('complementary', { name: 'Terima peringatan SOS meski GARDA ditutup' })).toHaveCount(0)
    expect(problems).toEqual([])
  })

  test('no dashboard hint for HR', async ({ page, problems }) => {
    await openAs(page, 'hr', '/')
    await expect(page.getByRole('complementary', { name: 'Terima peringatan SOS meski GARDA ditutup' })).toHaveCount(0)
    expect(problems).toEqual([])
  })

  test('the service worker registers under the production CSP @desktop', async ({ page, problems }) => {
    await openAs(page, 'admin', '/notifications')
    const scope = await page.evaluate(async () => (await navigator.serviceWorker.ready).scope)
    expect(new URL(scope).pathname).toBe('/')
    expect(problems).toEqual([])
  })
})
