// TAQ-21: a supervisor reviews a fraud signal; roles without access do not get the page.
export default async function ({ page, expect, openAs, goto, shot, note, world }) {
  await openAs('supervisor', '/flags')
  await page.getByLabel('Status').selectOption('')
  await page.getByRole('button', { name: 'Terapkan filter' }).click()
  await page.getByRole('button', { name: /^Tandai sudah ditinjau:/ }).first().waitFor()
  await shot('supervisor sees open flags')
  const open = await page.getByRole('button', { name: /^Tandai sudah ditinjau:/ }).count()
  await page.getByRole('button', { name: /^Tandai sudah ditinjau:/ }).first().click()
  await page.getByText(/^Ditinjau oleh /).first().waitFor()
  await shot('flag reviewed')
  // The review is stored apart: the flagged records keep their flags and review state.
  const att = (await world())?.attendance ?? []
  const backdated = att.find((r) => r.flags?.includes('backdated_on_time'))
  expect(backdated?.needs_review).toBe(true)
  expect(backdated?.reviewed_at).toBeUndefined()
  expect(await page.getByRole('button', { name: /^Tandai sudah ditinjau:/ }).count()).toBe(open - 1)
  const href = await page.getByRole('link', { name: /^Buka catatan:/ }).first().getAttribute('href')
  note(`open flags ${open} -> ${open - 1}; first record link ${href}; flagged attendance record untouched`)
  for (const role of ['hr', 'client-readonly']) {
    await openAs(role, '/flags')
    await shot(`${role} denied`)
    expect(await page.getByRole('button', { name: 'Terapkan filter' }).count()).toBe(0)
    expect(await page.getByRole('link', { name: 'Sinyal kecurangan' }).count()).toBe(0)
    note(`${role}: no filter form, no nav link`)
  }
}
