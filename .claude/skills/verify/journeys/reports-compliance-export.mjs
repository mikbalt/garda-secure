// Reports: a supervisor opens the patrol-schedule compliance report and
// downloads it as Excel. Proof: the report region and a non-empty .xlsx saved
// into the evidence dir.
import fs from 'node:fs'
import path from 'node:path'

export default async function ({ page, expect, openAs, goto, shot, note, download, evidenceDir }) {
  await openAs('supervisor', '/reports')
  await expect(page.getByRole('link', { name: /Kepatuhan jadwal patroli/ })).toBeVisible()
  await shot('reports index')

  await goto('/reports?tab=compliance')
  await expect(page.getByRole('heading', { level: 1, name: 'Kepatuhan jadwal patroli' })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Kepatuhan jadwal patroli' })).toBeVisible()
  await shot('compliance report')

  await page.getByRole('button', { name: /^Unduh/ }).click()
  const file = await download(() => page.getByRole('button', { name: /Excel \(\.xlsx\)/ }).click())
  expect(file).toMatch(/\.xlsx$/)
  const bytes = fs.readFileSync(path.join(evidenceDir, file))
  // .xlsx is a zip archive: "PK" magic.
  expect(bytes.subarray(0, 2).toString('latin1')).toBe('PK')
  note(`${file}: ${bytes.length} bytes, zip magic ok`)
}
