// Patrol routes: an admin opens a route, sees its schedule, is stopped by the
// grace-period rule, then saves a valid grace. Proof: validation message, the
// "saved" toast, and the value still there when the editor is reopened.
export default async function ({ page, expect, openAs, shot, note }) {
  await openAs('admin', '/patrol?tab=routes')
  await page.getByRole('button', { name: 'Ubah — Patroli Keliling Gudang' }).click()
  const dialog = page.getByRole('dialog', { name: 'Ubah rute' })
  await dialog.getByRole('tab', { name: /Jadwal/ }).click()
  await expect(dialog.getByText(/Setiap 2 jam dari 22:00 sampai 06:00/)).toBeVisible()
  await shot('schedule before')

  const grace = dialog.getByLabel('Toleransi').first()
  await grace.fill('90')
  await expect(dialog.getByText('Toleransi maksimal setengah dari selang waktu.')).toBeVisible()
  await shot('grace too long is rejected')

  await grace.fill('20')
  await dialog.getByRole('button', { name: 'Simpan' }).click()
  await expect(page.getByText('Rute tersimpan.').first()).toBeVisible()
  await shot('route saved')

  await page.getByRole('button', { name: 'Ubah — Patroli Keliling Gudang' }).click()
  const again = page.getByRole('dialog', { name: 'Ubah rute' })
  await again.getByRole('tab', { name: /Jadwal/ }).click()
  await expect(again.getByLabel('Toleransi').first()).toHaveValue('20')
  await shot('grace persisted after reopening')
  note('grace 15 -> 20 min persisted on route Patroli Keliling Gudang')
}
