import type { Locator, Page } from '@playwright/test'
import { expect, openAs, test } from './fixtures'

/** Acknowledges every open SOS so the page shows its normal state (no SOS banner). */
async function clearSos(page: Page) {
  const banner = page.getByRole('region', { name: 'Peringatan SOS' })
  const ack = banner.getByRole('button', { name: /^Tanggapi/ })
  while ((await ack.count()) > 0) {
    const before = await ack.count()
    await ack.first().click()
    await expect(ack).toHaveCount(before - 1)
  }
  await expect(banner).toHaveCount(0)
}

/**
 * Asserts `inner` lies completely inside `outer`. Both rects are read in one
 * frame and polled, so late layout shifts (the SOS banner arriving) do not race.
 */
async function expectInside(inner: Locator, outer: Locator) {
  const outerEl = await outer.elementHandle()
  await expect
    .poll(() =>
      inner.evaluate((el, o) => {
        const a = el.getBoundingClientRect()
        const b = (o as Element).getBoundingClientRect()
        return a.left >= b.left - 0.5 && a.top >= b.top - 0.5 && a.right <= b.right + 0.5 && a.bottom <= b.bottom + 0.5
      }, outerEl)
    )
    .toBe(true)
}

/** Asserts `upper` ends above where `lower` starts (read in one frame, polled). */
async function expectAbove(upper: Locator, lower: Locator) {
  const lowerEl = await lower.elementHandle()
  await expect
    .poll(() => upper.evaluate((el, o) => el.getBoundingClientRect().bottom <= (o as Element).getBoundingClientRect().top + 0.5, lowerEl))
    .toBe(true)
}

// Playwright's click() only succeeds when the target itself receives the
// pointer event at its centre, so an element painted on top fails these tests.
test.describe('layout regressions @desktop', () => {
  test('site picker opens above the page and switches site without an SOS banner', async ({ page, problems }) => {
    await openAs(page, 'admin', '/')
    await clearSos(page)

    const trigger = page.getByRole('button', { name: /^Ganti lokasi/ })
    await expect(trigger).toHaveAccessibleName(/Gudang Cikarang/)
    await trigger.click()
    const panel = page.getByRole('group', { name: 'Lokasi' })
    await expect(panel).toBeVisible()
    await panel.getByRole('button', { name: /Kantor Pusat Sudirman/ }).click({ timeout: 5_000 })
    await expect(panel).toBeHidden()
    await expect(trigger).toHaveAccessibleName(/Kantor Pusat Sudirman/)

    // The "manage locations" link at the bottom of the panel is reachable too.
    await trigger.click()
    await panel.getByRole('link', { name: 'Kelola lokasi' }).click({ timeout: 5_000 })
    await expect(page).toHaveURL(/\/settings\?tab=sites/)
    expect(problems).toEqual([])
  })

  test('site picker stays clickable while an SOS banner is shown', async ({ page, problems }) => {
    await openAs(page, 'supervisor', '/')
    await expect(page.getByRole('region', { name: 'Peringatan SOS' })).toBeVisible()
    const trigger = page.getByRole('button', { name: /^Ganti lokasi/ })
    await trigger.click()
    await page.getByRole('group', { name: 'Lokasi' }).getByRole('button', { name: /Kantor Pusat Sudirman/ }).click({ timeout: 5_000 })
    await expect(trigger).toHaveAccessibleName(/Kantor Pusat Sudirman/)
    expect(problems).toEqual([])
  })

  test('user menu options are clickable', async ({ page, problems }) => {
    await openAs(page, 'admin', '/')
    await page.getByRole('button', { name: 'Menu pengguna' }).click()
    const menu = page.getByRole('group', { name: 'Menu pengguna' })
    await menu.getByRole('button', { name: 'English' }).click({ timeout: 5_000 })
    await expect(page.getByRole('navigation', { name: 'Main navigation' }).getByRole('link', { name: 'Audit log' })).toBeVisible()
    expect(problems).toEqual([])
  })

  test.describe('laptop 1366x820', () => {
    test.use({ viewport: { width: 1366, height: 820 } })

    test('last nav item is visible and not covered by the setup card', async ({ page, problems }) => {
      await openAs(page, 'admin', '/')
      const nav = page.getByRole('navigation', { name: 'Navigasi utama' })
      const last = nav.getByRole('link', { name: 'Log audit' })
      await expect(last).toBeInViewport({ ratio: 1 })

      const card = page.locator('#app-sidebar').getByRole('link', { name: /Lanjutkan setup/ })
      await expect(card).toBeVisible()
      await expectAbove(last, card)

      await last.click({ timeout: 5_000 })
      await expect(page).toHaveURL(/\/audit/)
      expect(problems).toEqual([])
    })

    test('attendance: the correction button stays visible and clickable', async ({ page, problems }) => {
      await openAs(page, 'admin', '/attendance')
      // Production height: the SOS banner pushes the table down.
      await expect(page.getByRole('region', { name: 'Peringatan SOS' })).toBeVisible()
      const table = page.getByRole('region', { name: /^Kehadiran — / })
      const fix = table.getByRole('button', { name: /^Koreksi — / }).first()
      await expect(fix).toBeVisible()
      await expectInside(fix, table)
      await expect(fix).toBeInViewport({ ratio: 1 })

      // The table still scrolls horizontally (keyboard-focusable region); the action column stays pinned.
      await table.evaluate((el) => (el.scrollLeft = el.scrollWidth))
      await expectInside(fix, table)

      await fix.click({ timeout: 5_000 })
      await expect(page.getByRole('dialog', { name: 'Koreksi kehadiran' })).toBeVisible()
      expect(problems).toEqual([])
    })
  })
})

test('dashboard setup card keeps its text readable on a phone @mobile', async ({ page, problems }) => {
  await openAs(page, 'admin', '/')
  const title = page.getByText(/^Selesaikan setup /)
  const card = title.locator('xpath=ancestor::div[contains(@class,"rounded-2xl")][1]')
  const cta = card.getByRole('link', { name: /Lengkapi/ })
  await expect(cta).toBeVisible()
  // The text gets (almost) the full card width instead of a narrow column beside the button...
  const cardEl = await card.elementHandle()
  const ratio = await title.evaluate((el, c) => el.getBoundingClientRect().width / (c as Element).getBoundingClientRect().width, cardEl)
  expect(ratio).toBeGreaterThan(0.6)
  // ...and the actions sit below it, not next to it.
  await expectAbove(title, cta)
  await expectInside(cta, card)
  expect(problems).toEqual([])
})
