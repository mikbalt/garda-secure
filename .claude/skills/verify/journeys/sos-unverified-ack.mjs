// SOS: an alert a supervisor answered on a tablet without a verified PIN
// (D-065) is still open on the web, and says who answered. Acknowledging it
// here closes it. Proof: the amber note, then the row turns handled.
export default async function ({ page, expect, openAs, shot, note }) {
  await openAs('supervisor', '/incidents?tab=sos')
  const trigger = page.getByRole('button', { name: /^Ganti lokasi/ })
  await trigger.click()
  await page.getByRole('group', { name: 'Lokasi' }).getByRole('button', { name: /Kantor Pusat Sudirman/ }).click()
  await expect(trigger).toHaveAccessibleName(/Kantor Pusat Sudirman/)

  const row = page.getByRole('listitem').filter({ hasText: 'PIN tidak terverifikasi' })
  await expect(row).toHaveCount(1)
  await expect(row.getByText(/menanggapi di tablet pukul \d{2}[.:]\d{2}/)).toBeVisible()
  await expect(row.getByText(/^Menunggu /)).toBeVisible()
  await shot('open alert answered on a tablet without a verified PIN')

  await row.getByRole('button', { name: 'Tanggapi' }).click()
  await expect(page.getByText('SOS ditanggapi.').first()).toBeVisible()
  await expect(page.getByText('PIN tidak terverifikasi')).toHaveCount(0)
  await shot('after the web acknowledgement')
  note('unverified tablet response shown on an open alert; the web acknowledgement closed it')
}
