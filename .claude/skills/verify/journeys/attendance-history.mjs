// Attendance: HR opens yesterday's attendance and reads the correction history
// of a record. Proof: both corrections, their reasons and the acting user.
export default async function ({ page, expect, openAs, shot, note }) {
  // Yesterday in Asia/Jakarta, the demo sites' time zone.
  const yesterday = new Date(Date.now() + 7 * 3600_000 - 86_400_000).toISOString().slice(0, 10)
  await openAs('hr', `/attendance?date=${yesterday}`)
  await shot('attendance of yesterday')

  await page.getByRole('button', { name: /^Riwayat — / }).first().click()
  const drawer = page.getByRole('dialog', { name: /^Riwayat koreksi — / })
  await expect(drawer.getByText('2 koreksi, terlama lebih dulu. Waktu menurut zona waktu lokasi.')).toBeVisible()
  await expect(drawer.getByText(/mengantar korban kecelakaan/).first()).toBeVisible()
  await expect(drawer.getByText(/tablet mati/).first()).toBeVisible()
  await expect(drawer.getByText('admin.rina').first()).toBeVisible()
  await shot('correction history')
  note(`correction history for ${yesterday} lists 2 corrections by admin.rina`)
}
