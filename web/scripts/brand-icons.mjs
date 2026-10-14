#!/usr/bin/env node
// Renders every web/tablet icon from the single source branding/icon.svg
// (see branding/README.md). Uses Playwright's Chromium, already a dev
// dependency of the web app; set PW_CHANNEL=chrome to use the installed
// Google Chrome instead. Run via branding/generate.sh or `npm run icons`.
import { mkdirSync, readFileSync, writeFileSync, copyFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { chromium } from '@playwright/test'

const web = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const repo = resolve(web, '..')
const brandDir = join(repo, 'branding')
const source = join(brandDir, 'icon.svg')
const svg = readFileSync(source, 'utf8').replace(/<\?xml[^>]*>/, '')

/**
 * One render job.
 * size:   canvas in px (square)
 * scale:  share of the canvas the icon covers (centered), 1 = full bleed
 * hide:   element ids of icon.svg to hide (adaptive-icon layers)
 * transparent: keep the canvas transparent outside the icon
 */
const jobs = [
  // Master PNG (iOS App Store icon, launcher source, README preview).
  { out: join(brandDir, 'icon-1024.png'), size: 1024 },
  // Android adaptive icon layers. flutter_launcher_icons insets the
  // foreground by 16% on the 108dp canvas, so it covers the visible 72dp.
  { out: join(brandDir, 'generated', 'android-foreground.png'), size: 1024, hide: ['background'], transparent: true },
  { out: join(brandDir, 'generated', 'android-background.png'), size: 1024, hide: ['mark'] },
  // Web (PWA manifest, iOS home screen, favicon).
  { out: join(web, 'public', 'icons', 'icon-192.png'), size: 192 },
  { out: join(web, 'public', 'icons', 'icon-512.png'), size: 512 },
  { out: join(web, 'public', 'icons', 'maskable-512.png'), size: 512 },
  { out: join(web, 'public', 'apple-touch-icon.png'), size: 180 },
  { out: join(web, 'public', 'icons', 'favicon-32.png'), size: 32 },
  // In-app brand mark of the tablet app (shared/widgets/brand.dart).
  { out: join(repo, 'mobile', 'assets', 'brand', 'icon.png'), size: 256 },
]
const icoSizes = [16, 32, 48]

function page(size, scale = 1, hide = [], transparent = false) {
  const inner = Math.round(size * scale)
  const hidden = hide.map((id) => `#${id}{display:none}`).join('')
  return `<!doctype html><html><head><style>
    html,body{margin:0;padding:0;width:${size}px;height:${size}px;overflow:hidden;background:${transparent ? 'transparent' : '#ffffff'}}
    body{display:flex;align-items:center;justify-content:center}
    svg{display:block;width:${inner}px;height:${inner}px}
    ${hidden}
  </style></head><body>${svg}</body></html>`
}

/** ICO container with embedded PNG images (supported by every current browser). */
function ico(pngs) {
  const header = Buffer.alloc(6)
  header.writeUInt16LE(0, 0)
  header.writeUInt16LE(1, 2)
  header.writeUInt16LE(pngs.length, 4)
  const entries = []
  let offset = 6 + 16 * pngs.length
  for (const { size, data } of pngs) {
    const e = Buffer.alloc(16)
    e.writeUInt8(size >= 256 ? 0 : size, 0)
    e.writeUInt8(size >= 256 ? 0 : size, 1)
    e.writeUInt8(0, 2)
    e.writeUInt8(0, 3)
    e.writeUInt16LE(1, 4)
    e.writeUInt16LE(32, 6)
    e.writeUInt32LE(data.length, 8)
    e.writeUInt32LE(offset, 12)
    offset += data.length
    entries.push(e)
  }
  return Buffer.concat([header, ...entries, ...pngs.map((p) => p.data)])
}

const browser = await chromium.launch({ channel: process.env.PW_CHANNEL || undefined })
try {
  const ctx = await browser.newContext({ deviceScaleFactor: 1 })
  const tab = await ctx.newPage()
  const render = async (size, scale, hide, transparent) => {
    await tab.setViewportSize({ width: size, height: size })
    await tab.setContent(page(size, scale, hide, transparent))
    return tab.screenshot({ omitBackground: !!transparent, clip: { x: 0, y: 0, width: size, height: size } })
  }
  for (const j of jobs) {
    mkdirSync(dirname(j.out), { recursive: true })
    writeFileSync(j.out, await render(j.size, j.scale, j.hide, j.transparent))
    console.log('wrote', j.out.slice(repo.length + 1))
  }
  const pngs = []
  for (const size of icoSizes) pngs.push({ size, data: await render(size) })
  writeFileSync(join(web, 'public', 'favicon.ico'), ico(pngs))
  console.log('wrote web/public/favicon.ico')
} finally {
  await browser.close()
}

// The SVG itself is the modern favicon and the web app's <BrandMark>.
copyFileSync(source, join(web, 'public', 'favicon.svg'))
console.log('wrote web/public/favicon.svg')
