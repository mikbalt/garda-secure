import type { Locator, Page } from '@playwright/test'
import { expect, openAs, test } from './fixtures'

/** Presses Tab until `target` has focus (fails after `max` presses). */
async function tabTo(page: Page, target: Locator, max = 60) {
  for (let i = 0; i < max; i++) {
    if (await target.evaluate((el) => el === document.activeElement).catch(() => false)) return
    await page.keyboard.press('Tab')
  }
  throw new Error('target never received focus')
}

test.describe('keyboard only', () => {
  // Off-canvas navigation exists below the lg breakpoint only.
  test('off-canvas menu: closed is inert, open traps focus, Escape returns @mobile', async ({ page, problems }) => {
    await openAs(page, 'admin', '/')
    const sidebar = page.locator('#app-sidebar')
    await expect(sidebar).toHaveAttribute('inert', '')

    // Skip link first.
    await page.keyboard.press('Tab')
    await expect(page.getByRole('link', { name: 'Langsung ke konten' })).toBeFocused()

    const menu = page.getByRole('button', { name: 'Buka menu' })
    await tabTo(page, menu)
    await page.keyboard.press('Enter')
    await expect(page.getByRole('dialog', { name: 'Navigasi utama' })).toBeVisible()
    expect(await sidebar.evaluate((el) => el.contains(document.activeElement))).toBe(true)

    for (let i = 0; i < 30; i++) await page.keyboard.press('Tab')
    expect(await sidebar.evaluate((el) => el.contains(document.activeElement)), 'focus stays in the menu').toBe(true)

    await page.keyboard.press('Escape')
    await expect(sidebar).toHaveAttribute('inert', '')
    await expect(menu).toBeFocused()
    expect(problems).toEqual([])
  })

  test('navigate, switch tabs with arrows and acknowledge an SOS @desktop', async ({ page, problems }) => {
    await openAs(page, 'admin', '/')

    await tabTo(page, page.getByRole('navigation', { name: 'Navigasi utama' }).getByRole('link', { name: 'Insiden' }))
    await page.keyboard.press('Enter')
    await expect(page).toHaveURL(/\/incidents/)
    await expect(page.getByRole('heading', { level: 1, name: 'Insiden' })).toBeVisible()

    const selected = page.getByRole('tab', { selected: true })
    await tabTo(page, selected)
    await page.keyboard.press('ArrowRight')
    await expect(page).toHaveURL(/tab=sos/)
    await expect(page.getByRole('tabpanel', { name: 'SOS' })).toBeVisible()

    const banner = page.getByRole('region', { name: 'Peringatan SOS' })
    await expect(banner).toBeVisible()
    const before = await banner.getByRole('button', { name: /^Tanggapi/ }).count()
    await tabTo(page, banner.getByRole('button', { name: /^Tanggapi/ }).first())
    await page.keyboard.press('Enter')
    await expect(page.getByText('SOS ditanggapi.').first()).toBeVisible()
    await expect(banner.getByRole('button', { name: /^Tanggapi/ })).toHaveCount(before - 1)
    expect(problems).toEqual([])
  })
})
