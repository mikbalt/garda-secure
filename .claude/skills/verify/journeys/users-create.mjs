// Users & access: an admin creates a supervisor limited to one site, with a
// temporary password that is shown once. Proof: the new row in the list AND
// the new user in the demo backend's data.
export default async function ({ page, expect, openAs, shot, note, world, quiet }) {
  await openAs('admin', '/users')
  const before = JSON.stringify(await world()).includes('spv.sari')
  expect(before, 'spv.sari must not exist in the seed').toBe(false)
  await shot('users list before')

  await page.getByRole('button', { name: 'Tambah pengguna' }).first().click()
  const dialog = page.getByRole('dialog', { name: 'Tambah pengguna' })
  await dialog.getByLabel(/Nama pengguna/).fill('Spv.Sari')
  await dialog.getByLabel(/Nama depan/).fill('Sari')
  await dialog.getByRole('checkbox', { name: /^Supervisor/ }).check()
  await dialog.getByRole('radio', { name: /Hanya lokasi berikut/ }).check()
  await dialog.getByRole('radio', { name: /Atur kata sandi sementara sekarang/ }).check()
  await quiet()
  await shot('form filled')
  await dialog.getByRole('button', { name: 'Buat pengguna' }).click()

  const done = page.getByRole('dialog', { name: 'Sari sudah dibuat' })
  await expect(done.getByText('Hanya ditampilkan sekali')).toBeVisible()
  await expect(done.getByRole('textbox', { name: 'Kata sandi sementara' })).toHaveValue(/^[A-Za-z0-9]{16}$/)
  await shot('temporary password shown once')
  await done.getByRole('button', { name: 'Selesai, sudah saya salin' }).click()

  await page.getByLabel('Cari pengguna').fill('spv.sari')
  const row = page.getByRole('row').filter({ hasText: 'spv.sari' })
  await expect(row).toBeVisible()
  await expect(row.getByText('Gudang Cikarang')).toBeVisible()
  await expect(row.getByText('Wajib ganti kata sandi')).toBeVisible()
  // The search has filtered the list down to the new user.
  await expect(page.getByRole('row').filter({ hasText: 'admin.dimas' })).toHaveCount(0)
  await row.scrollIntoViewIfNeeded()
  await shot('new user in the list')

  expect(JSON.stringify(await world()).includes('spv.sari'), 'user stored in the demo backend').toBe(true)
  note('spv.sari is stored in sp.demo.world, limited to Gudang Cikarang, must change password')
}
