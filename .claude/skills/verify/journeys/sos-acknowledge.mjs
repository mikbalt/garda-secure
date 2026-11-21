// SOS: a supervisor sees the open alerts of every site in the banner and
// acknowledges one. Proof: the toast and one fewer open alert in the banner.
export default async function ({ page, expect, openAs, shot, note }) {
  await openAs('supervisor', '/')
  const banner = page.getByRole('region', { name: 'Peringatan SOS' })
  await expect(banner).toBeVisible()
  await expect(banner.getByText('Gudang Cikarang')).toBeVisible()
  await expect(banner.getByText('Kantor Pusat Sudirman')).toBeVisible()
  const ack = banner.getByRole('button', { name: /^Tanggapi/ })
  const before = await ack.count()
  expect(before, 'the seed has open SOS alerts').toBeGreaterThan(0)
  await shot('banner with open alerts')

  await ack.first().click()
  await expect(page.getByText('SOS ditanggapi.').first()).toBeVisible()
  await expect(ack).toHaveCount(before - 1)
  await shot('after acknowledging')
  note(`open SOS alerts in the banner: ${before} -> ${before - 1}`)
}
