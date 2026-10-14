import { fileURLToPath } from 'node:url'
import { expect, test, type Locator, type Page } from '@playwright/test'
import { openAs, quiet, settle, type DemoRole } from '../e2e/fixtures'

/**
 * Product screenshots for the sales guide (docs/sales-guide/img/), taken from
 * the demo build in Indonesian. Run: `PW_CHANNEL=chrome npm run guide:shots`.
 * Every test writes one PNG (two for the route editor); the file names are
 * referenced by the guide and by docs/sales-guide/img/manifest.json.
 */
const OUT = fileURLToPath(new URL('../../docs/sales-guide/img/', import.meta.url))

/** Demo-only chrome and transient UI that make a screenshot look unfinished. */
const HIDE = `
  /* demo role switcher bar */
  .bg-gradient-to-r.from-violet-600.to-primary-600 { display: none !important; }
  /* toasts */
  .fixed.bottom-4.right-4 { display: none !important; }
  *, *::before, *::after { caret-color: transparent !important; }
`
const HIDE_SOS = 'section[aria-label="Peringatan SOS"] { display: none !important; }'

/** Yesterday in Asia/Jakarta (the demo sites' time zone). */
function yesterdayWib(): string {
  return new Date(Date.now() + 7 * 3600_000 - 86_400_000).toISOString().slice(0, 10)
}

/**
 * Opens `path` as `role` (Indonesian, SOS alarm muted). The red SOS banner sits
 * on every page while an SOS is open; it is hidden unless `sos` is set, so
 * shots about other features show the feature.
 */
async function open(page: Page, role: DemoRole, path = '/', { sos = false } = {}) {
  await page.addInitScript((css) => {
    const add = () => {
      const s = document.createElement('style')
      s.textContent = css
      document.head.appendChild(s)
    }
    if (document.head) add()
    else document.addEventListener('DOMContentLoaded', add)
  }, sos ? HIDE : HIDE + HIDE_SOS)
  await openAs(page, role, path, 'id')
}

/** Waits for loading and animations to finish, then saves the viewport (or `target`). */
async function shot(page: Page, name: string, target?: Locator) {
  await expect(page.getByText('Memuat lokasi')).toHaveCount(0, { timeout: 15_000 })
  await expect(page.locator('main .animate-pulse')).toHaveCount(0, { timeout: 15_000 })
  // Secondary queries (SOS banner, setup progress) arrive a moment later.
  await page.waitForTimeout(800)
  await quiet(page)
  // Let images (floor plans, demo photos, QR codes) decode.
  await page.waitForFunction(() => Array.from(document.images).every((i) => i.complete), undefined, { timeout: 10_000 })
  await page.mouse.move(0, 0)
  const opts = { path: `${OUT}${name}.png`, animations: 'disabled' as const }
  if (target) await target.screenshot(opts)
  else await page.screenshot({ ...opts, fullPage: false })
}

/** Scrolls `el` to the top of its scroll container (main area or modal body), minus `offset` px. */
async function scrollToTop(el: Locator, offset = 0) {
  await el.evaluate((node, off) => {
    node.scrollIntoView({ block: 'start' })
    let p = node.parentElement
    while (p && p.scrollHeight <= p.clientHeight) p = p.parentElement
    if (p) p.scrollTop -= off
  }, offset)
}

/** Switches the global site selector. */
async function pickSite(page: Page, name: string) {
  await page.getByRole('button', { name: /^Ganti lokasi/ }).click()
  // DOM click: without the SOS banner the open list renders under <main> (stacking bug), so a pointer click is intercepted.
  await page.getByRole('button', { name: new RegExp(`^${name}`) }).evaluate((el) => (el as HTMLButtonElement).click())
  await settle(page)
}

function dialog(page: Page, name?: string | RegExp) {
  return name ? page.getByRole('dialog', { name }) : page.getByRole('dialog').last()
}

/** Gives the guard without a PIN one, which completes the demo's setup guide. */
async function setMissingPin(page: Page, submit: boolean) {
  await page.getByRole('button', { name: /^Atur PIN — / }).first().click()
  const pins = dialog(page).locator('input[inputmode="numeric"]')
  await pins.nth(0).fill('482915')
  await pins.nth(1).fill('482915')
  await pins.nth(1).blur()
  if (submit) {
    await dialog(page).locator('button[type="submit"]').click()
    await expect(page.getByRole('dialog')).toHaveCount(0)
  }
}

test.describe('admin', () => {
  test('setup guide', async ({ page }) => {
    await open(page, 'admin', '/setup')
    await shot(page, 'web-setup')
  })

  test('dashboard', async ({ page }) => {
    await open(page, 'admin', '/', { sos: true })
    await expect(page.getByRole('region', { name: 'Peringatan SOS' })).toBeVisible()
    await shot(page, 'web-dashboard')
  })

  test('SOS banner', async ({ page }) => {
    await open(page, 'admin', '/', { sos: true })
    const banner = page.getByRole('region', { name: 'Peringatan SOS' })
    await expect(banner).toBeVisible()
    await shot(page, 'web-sos-banner', banner)
  })

  test('attendance', async ({ page }) => {
    await open(page, 'admin', '/attendance')
    await shot(page, 'web-attendance')
  })

  test('attendance review', async ({ page }) => {
    await open(page, 'admin', '/attendance?tab=review')
    await shot(page, 'web-attendance-review')
  })

  test('attendance override', async ({ page }) => {
    await open(page, 'admin', '/attendance')
    await page.getByRole('button', { name: /^Koreksi — / }).first().click()
    const reason = dialog(page, 'Koreksi kehadiran').getByRole('textbox', { name: /Alasan/ })
    await reason.fill('Tablet pos mati pagi ini; jam masuk dikonfirmasi supervisor lewat buku mutasi.')
    await reason.blur()
    await shot(page, 'web-attendance-override')
  })

  test('attendance correction history', async ({ page }) => {
    await open(page, 'admin', `/attendance?date=${yesterdayWib()}`)
    await page.getByRole('button', { name: /^Riwayat — / }).first().click()
    await expect(dialog(page, /^Riwayat koreksi — /)).toBeVisible()
    await shot(page, 'web-attendance-history')
  })

  test('schedule patterns', async ({ page }) => {
    await open(page, 'admin', '/schedules?tab=templates')
    await shot(page, 'web-schedules-templates')
  })

  test('weekly roster', async ({ page }) => {
    await open(page, 'admin', '/schedules')
    await shot(page, 'web-schedules-roster')
  })

  test('guards', async ({ page }) => {
    await open(page, 'admin', '/guards')
    await shot(page, 'web-guards')
  })

  test('guard PIN', async ({ page }) => {
    await open(page, 'admin', '/guards')
    await setMissingPin(page, false)
    await shot(page, 'web-guard-pin')
  })

  test('live patrol', async ({ page }) => {
    await open(page, 'admin', '/patrol')
    // Sudirman's running round has an overdue checkpoint.
    await pickSite(page, 'Kantor Pusat Sudirman')
    await shot(page, 'web-patrol-live')
  })

  test('live patrol on the floor plan', async ({ page }) => {
    await open(page, 'admin', '/patrol?view=plan')
    await expect(page.getByRole('img', { name: /Denah / }).first()).toBeVisible()
    await scrollToTop(page.getByText('1 patroli berjalan'), 16)
    await shot(page, 'web-patrol-plan')
  })

  test('route editor', async ({ page }) => {
    await open(page, 'admin', '/patrol?tab=routes')
    await page.getByRole('button', { name: 'Ubah — Patroli Keliling Gudang' }).click()
    const d = dialog(page, 'Ubah rute')
    await expect(d).toBeVisible()
    await d.getByLabel(/Nama rute/).blur()
    await shot(page, 'web-route-editor')
    await d.getByRole('tab', { name: /Denah/ }).click()
    await expect(d.getByRole('button', { name: /^Titik 1, / })).toBeVisible()
    await scrollToTop(d.getByText('Titik yang ditempatkan'), 8)
    await shot(page, 'web-route-plan')
    await d.getByRole('tab', { name: /Jadwal/ }).click()
    await expect(d.getByText('Patroli terencana berikutnya')).toBeVisible()
    await shot(page, 'web-route-schedule')
  })

  test('QR sheet', async ({ page }) => {
    await open(page, 'admin', '/patrol?tab=routes')
    await page.getByRole('button', { name: 'Cetak kode QR — Patroli Keliling Gudang' }).click()
    await expect(dialog(page, 'Kode QR — Patroli Keliling Gudang')).toBeVisible()
    await shot(page, 'web-qr-sheet')
  })

  test('patrol history and session timeline', async ({ page }) => {
    await page.setViewportSize({ width: 1366, height: 1000 })
    await open(page, 'admin', '/patrol?tab=sessions')
    // The finished round with a "too fast" flag.
    await page.getByRole('button', { name: 'Patroli Keliling Gudang' }).nth(1).click()
    await expect(dialog(page, 'Detail patroli')).toBeVisible()
    await shot(page, 'web-session-timeline')
  })

  test('incidents', async ({ page }) => {
    await open(page, 'admin', '/incidents')
    await shot(page, 'web-incidents')
  })

  test('incident detail', async ({ page }) => {
    // Tall enough to show the amended values and the photos.
    await page.setViewportSize({ width: 1366, height: 1180 })
    await open(page, 'admin', '/incidents')
    await page.getByRole('button', { name: /Pencurian/ }).first().click()
    const d = dialog(page, 'Detail insiden')
    await expect(d.getByText('Dikoreksi setelah dilaporkan').first()).toBeVisible()
    await shot(page, 'web-incident-detail')
  })

  test('SOS tab', async ({ page }) => {
    await open(page, 'admin', '/incidents?tab=sos', { sos: true })
    await shot(page, 'web-sos-tab')
  })

  test('defects', async ({ page }) => {
    await open(page, 'admin', '/incidents?tab=defects')
    await shot(page, 'web-defects')
  })

  test('visitors', async ({ page }) => {
    await open(page, 'admin', '/incidents?tab=visitors')
    await shot(page, 'web-visitors')
  })

  test('reports', async ({ page }) => {
    await open(page, 'admin', '/reports')
    await shot(page, 'web-reports')
  })

  for (const [tab, name] of [
    ['daily', 'web-report-daily'],
    ['compliance', 'web-report-compliance'],
    ['kpi', 'web-report-kpi'],
    ['monthly', 'web-report-monthly'],
  ] as const) {
    test(`report ${tab}`, async ({ page }) => {
      await open(page, 'admin', `/reports?tab=${tab}`)
      await settle(page)
      await shot(page, name)
    })
  }

  test('report export menu', async ({ page }) => {
    // The incident report offers all three formats.
    await open(page, 'admin', '/reports?tab=incidents')
    await settle(page)
    await page.getByRole('button', { name: /^Unduh/ }).click()
    await expect(page.getByRole('button', { name: /PDF/ })).toBeVisible()
    await shot(page, 'web-report-export')
  })

  test('notification rules', async ({ page }) => {
    await open(page, 'admin', '/settings?tab=rules')
    await shot(page, 'web-rules')
  })

  test('tablets', async ({ page }) => {
    await open(page, 'admin', '/settings?tab=devices')
    await shot(page, 'web-devices')
  })

  test('tablet health', async ({ page }) => {
    await page.setViewportSize({ width: 1366, height: 1080 })
    await open(page, 'admin', '/settings?tab=devices')
    await page.getByRole('button', { name: /Kesehatan tablet: 3 peringatan — Tablet Lobby Sudirman/ }).click()
    const d = dialog(page, 'Kesehatan tablet — Tablet Lobby Sudirman')
    await expect(d).toBeVisible()
    await d.locator('.modal-body').evaluate((el) => (el.scrollTop = 0))
    await shot(page, 'web-device-health')
  })

  test('tablet account', async ({ page }) => {
    await open(page, 'admin', '/settings?tab=devices')
    await page.getByRole('button', { name: 'Akun tablet baru' }).first().click()
    await dialog(page, 'Akun tablet baru').getByLabel(/Nama tablet/).fill('Pos Gerbang Timur')
    await dialog(page, 'Akun tablet baru').getByRole('button', { name: 'Buat akun' }).click()
    const d = dialog(page, 'Akun tablet untuk Pos Gerbang Timur')
    await expect(d.getByText('Cara masuk di tablet')).toBeVisible()
    // The server address is the browser's host; show a customer-like one instead of 127.0.0.1:port.
    await d.getByText(/^127\.0\.0\.1:\d+$/).evaluate((el) => (el.textContent = 'patroli.contoh.co.id'))
    const secret = d.getByRole('textbox', { name: 'Kata sandi tablet' })
    await secret.evaluate((el) => (el as HTMLInputElement).setSelectionRange(0, 0))
    await secret.blur()
    await shot(page, 'web-device-account')
  })

  test('users', async ({ page }) => {
    await open(page, 'admin', '/users')
    await shot(page, 'web-users')
  })

  test('create user', async ({ page }) => {
    await open(page, 'admin', '/users?new=1')
    const d = dialog(page, 'Tambah pengguna')
    await d.getByLabel(/Nama pengguna/).fill('spv.sari')
    await d.getByLabel(/Nama depan/).fill('Sari')
    await d.getByRole('checkbox', { name: /^Supervisor/ }).check()
    await d.getByRole('radio', { name: /Hanya lokasi berikut/ }).check()
    await d.getByRole('checkbox', { name: /Gudang Cikarang/ }).check()
    await d.getByRole('checkbox', { name: /Gudang Cikarang/ }).blur()
    await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur())
    await scrollToTop(d.getByRole('group', { name: 'Peran' }), 4)
    await shot(page, 'web-user-create')
  })

  test('audit log', async ({ page }) => {
    await open(page, 'admin', '/audit')
    await page.getByLabel('Bagian').selectOption('guards')
    await page.getByRole('button', { name: 'Terapkan' }).click()
    await settle(page)
    const row = page.getByRole('row').filter({ hasText: 'sebelum/sesudah' }).filter({ hasText: '/api/v1/guards/:id' })
    await row.getByRole('button', { name: /Buka detail/ }).first().click()
    await expect(dialog(page, 'Catatan audit')).toBeVisible()
    await shot(page, 'web-audit')
  })
})

test.describe('platform admin', () => {
  for (const [tab, name] of [
    ['channels', 'web-channels'],
    ['ops', 'web-ops-alerts'],
    ['backups', 'web-backups'],
  ] as const) {
    test(`settings ${tab}`, async ({ page }) => {
      await open(page, 'platform-admin', `/settings?tab=${tab}`)
      await shot(page, name)
    })
  }
})

test.describe('other roles', () => {
  test('supervisor dashboard', async ({ page }) => {
    await open(page, 'supervisor', '/', { sos: true })
    await shot(page, 'web-supervisor-dashboard')
  })

  test('HR monthly report', async ({ page }) => {
    await open(page, 'hr', '/reports?tab=monthly')
    await settle(page)
    await shot(page, 'web-hr-monthly')
  })

  test('client dashboard', async ({ page }) => {
    await open(page, 'client-readonly', '/')
    await shot(page, 'web-client-dashboard')
  })

  test('client patrol', async ({ page }) => {
    await open(page, 'client-readonly', '/patrol?view=plan')
    await expect(page.getByRole('img', { name: /Denah / }).first()).toBeVisible()
    await shot(page, 'web-client-patrol')
  })
})

test.describe('phone', () => {
  test.use({ viewport: { width: 390, height: 844 }, hasTouch: true })

  test('phone dashboard', async ({ page }) => {
    // Finish the setup guide first so the phone shows the day's figures.
    await open(page, 'admin', '/guards', { sos: true })
    await setMissingPin(page, true)
    await page.goto('/')
    await settle(page)
    await shot(page, 'web-phone-dashboard')
  })

  test('phone menu', async ({ page }) => {
    await open(page, 'admin', '/', { sos: true })
    await page.getByRole('button', { name: 'Buka menu' }).click()
    await expect(page.getByRole('dialog', { name: 'Navigasi utama' })).toBeVisible()
    await shot(page, 'web-phone-menu')
  })
})
