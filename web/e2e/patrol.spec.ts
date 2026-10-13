import { expect, openAs, seriousA11yViolations, settle, test } from './fixtures'

// Floor plans, live plan, schedules, QR regeneration, compliance, exports,
// amended incidents and paging (D-037, D-038, D-043, D-045, D-048). Desktop only.
test.describe('patrol & reports features (demo) @desktop', () => {
  test('floor plans are listed and the live patrol shows on the plan', async ({ page, problems }) => {
    await openAs(page, 'admin', '/patrol?tab=routes')
    const plans = page.locator('section', { has: page.getByRole('heading', { name: 'Denah lokasi' }) })
    await expect(plans.getByText('Gudang – lantai dasar')).toBeVisible()
    await expect(page.getByText(/kode QR tanpa tanda tangan/).first()).toBeVisible()

    await page.goto('/patrol?view=plan')
    await settle(page)
    await expect(page.getByRole('img', { name: /Denah Gudang – lantai dasar/ })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Titik sesuai urutan' })).toBeVisible()
    expect(await seriousA11yViolations(page)).toEqual([])
    expect(problems).toEqual([])
  })

  test('route editor: place a checkpoint with the keyboard and save a schedule', async ({ page, problems }) => {
    await openAs(page, 'admin', '/patrol?tab=routes')
    await page.getByRole('button', { name: 'Ubah — Patroli Keliling Gudang' }).click()
    const dialog = page.getByRole('dialog', { name: 'Ubah rute' })
    await expect(dialog).toBeVisible()

    await dialog.getByRole('tab', { name: /Denah/ }).click()
    const marker = dialog.getByRole('button', { name: /^Titik 1, Gerbang Utama, di X 17 %/ })
    await marker.focus()
    await page.keyboard.press('ArrowRight')
    await expect(dialog.getByRole('button', { name: /^Titik 1, Gerbang Utama, di X 18 %/ })).toBeVisible()

    await dialog.getByRole('tab', { name: /Jadwal/ }).click()
    await expect(dialog.getByText('Setiap 2 jam dari 22:00 sampai 06:00, setiap hari, toleransi 15 mnt → 4 patroli/malam')).toBeVisible()
    await expect(dialog.getByText('Patroli terencana berikutnya')).toBeVisible()
    expect(await seriousA11yViolations(page)).toEqual([])

    // A grace longer than half the interval is caught before saving.
    await dialog.getByLabel('Toleransi').first().fill('90')
    await expect(dialog.getByText('Toleransi maksimal setengah dari selang waktu.')).toBeVisible()
    await dialog.getByLabel('Toleransi').first().fill('20')
    await dialog.getByRole('button', { name: 'Simpan' }).click()
    await expect(page.getByText('Rute tersimpan.').first()).toBeVisible()
    expect(problems).toEqual([])
  })

  test('regenerating QR codes needs a confirmation and opens the print sheet', async ({ page, problems }) => {
    await openAs(page, 'admin', '/patrol?tab=routes')
    await page.getByRole('button', { name: 'Cetak kode QR — Patroli Malam Perimeter' }).click()
    await page.getByRole('button', { name: 'Buat ulang kode QR' }).click()
    const confirm = page.getByRole('dialog', { name: /Buat ulang kode QR "Patroli Malam Perimeter"/ })
    const go = confirm.getByRole('button', { name: 'Buat ulang 4 kode' })
    await expect(go).toBeDisabled()
    await confirm.getByLabel(/Saya mengerti/).check()
    await go.click()
    await expect(page.getByRole('dialog', { name: 'Kode QR — Patroli Malam Perimeter' })).toBeVisible()
    await expect(page.getByText(/kode lama tanpa tanda tangan/)).toHaveCount(0)
    expect(problems).toEqual([])
  })

  test('compliance report and Excel download', async ({ page, problems }) => {
    await openAs(page, 'supervisor', '/reports')
    await expect(page.getByRole('link', { name: /Kepatuhan jadwal patroli/ })).toBeVisible()
    await page.goto('/reports?tab=compliance')
    await settle(page)
    await expect(page.getByRole('heading', { level: 1, name: 'Kepatuhan jadwal patroli' })).toBeVisible()
    await expect(page.getByRole('region', { name: 'Kepatuhan jadwal patroli' })).toBeVisible()
    expect(await seriousA11yViolations(page)).toEqual([])
    await page.getByRole('button', { name: /^Unduh/ }).click()
    const download = page.waitForEvent('download')
    await page.getByRole('button', { name: /Excel \(\.xlsx\)/ }).click()
    expect((await download).suggestedFilename()).toMatch(/\.xlsx$/)
    expect(problems).toEqual([])
  })

  test('monthly recap counts guard-days with a site total (F13)', async ({ page, problems }) => {
    await openAs(page, 'hr', '/reports?tab=monthly')
    await settle(page)
    await expect(page.getByRole('heading', { level: 1, name: 'Rekap bulanan untuk penggajian' })).toBeVisible()
    const table = page.getByRole('region', { name: 'Rekap bulanan untuk penggajian' })
    await expect(table.getByRole('columnheader', { name: 'Hari', exact: true })).toBeVisible()
    await expect(table.getByRole('columnheader', { name: 'Hari pulang awal' })).toBeVisible()
    await expect(table.getByRole('rowheader', { name: /^Total \(\d+ petugas\)$/ })).toBeVisible()
    await expect(page.getByText(/^Dihitung per hari menurut zona waktu lokasi/)).toBeVisible()
    await expect(page.getByText('Hari terjadwal')).toBeVisible()
    expect(await seriousA11yViolations(page)).toEqual([])
    expect(problems).toEqual([])
  })

  test('incidents: paging, amended values, GPS and PDF', async ({ page, problems }) => {
    await openAs(page, 'supervisor', '/incidents')
    await expect(page.getByText(/Menampilkan 50 dari \d+ insiden/)).toBeVisible()
    await page.getByRole('button', { name: /Muat lagi/ }).click()
    await expect(page.getByRole('button', { name: /Muat lagi/ })).toHaveCount(0)

    await page.getByRole('button', { name: /Pencurian/ }).first().click()
    const drawer = page.getByRole('dialog', { name: 'Detail insiden' })
    await expect(drawer.getByText('Dikoreksi setelah dilaporkan').first()).toBeVisible()
    await expect(drawer.getByRole('rowheader', { name: 'Lokasi' })).toBeVisible()
    await expect(drawer.getByText('-6.294310, 107.153020')).toBeVisible()
    expect(await seriousA11yViolations(page)).toEqual([])
    const download = page.waitForEvent('download')
    await drawer.getByRole('button', { name: 'Unduh PDF' }).click()
    expect((await download).suggestedFilename()).toMatch(/\.pdf$/)
    expect(problems).toEqual([])
  })

  test('notification rules list the new events and the channel state', async ({ page, problems }) => {
    await openAs(page, 'admin', '/settings?tab=rules')
    await expect(page.getByRole('heading', { name: 'Insiden dilaporkan (semua tingkat)' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Patroli terjadwal belum dimulai' })).toBeVisible()
    await expect(page.getByRole('list', { name: 'Saluran pengiriman' })).toContainText('belum diatur')
    expect(problems).toEqual([])
  })
})
