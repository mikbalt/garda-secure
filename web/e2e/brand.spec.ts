import { expect, openAs, test } from './fixtures'

/** Product name GARDA, PWA manifest and icons (branding/README.md). */
test('title, manifest and icons are GARDA and same-origin @desktop', async ({ page, request, problems }) => {
  await openAs(page, 'admin', '/')
  await expect(page).toHaveTitle('GARDA')
  // Sidebar/drawer brand: the app icon itself, next to the product name.
  const mark = page.locator('img[src$="favicon.svg"]').first()
  await expect(mark).toHaveJSProperty('complete', true)
  expect(await mark.evaluate((img: HTMLImageElement) => img.naturalWidth)).toBeGreaterThan(0)

  const href = await page.locator('link[rel="manifest"]').getAttribute('href')
  expect(href).toBe('/manifest.webmanifest')
  const res = await request.get(href!)
  expect(res.ok()).toBe(true)
  const manifest = await res.json()
  expect(manifest).toMatchObject({ name: 'GARDA', short_name: 'GARDA', display: 'standalone', start_url: '/' })
  const icons: { src: string; purpose?: string }[] = manifest.icons
  expect(icons.some((i) => i.purpose === 'maskable')).toBe(true)
  for (const src of [...icons.map((i) => i.src), '/apple-touch-icon.png', '/favicon.ico']) {
    expect(src.startsWith('/'), `${src} is same-origin`).toBe(true)
    const icon = await request.get(src)
    expect(icon.ok(), src).toBe(true)
    expect(icon.headers()['content-type'], src).toMatch(/^image\//)
  }
  expect(problems).toEqual([])
})
