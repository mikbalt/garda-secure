// Renders docs/sales-guide/index.html to docs/sales-guide/GARDA-Panduan-Sales.pdf
// (A4, print stylesheet, FAQ answers expanded). Usage, from web/:
//   PW_CHANNEL=chrome npm run guide:pdf      # system Chrome, or Playwright's Chromium
import { chromium } from '@playwright/test'
import { fileURLToPath, pathToFileURL } from 'node:url'
import path from 'node:path'

const dir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../../docs/sales-guide')
const out = path.join(dir, 'GARDA-Panduan-Sales.pdf')

const browser = await chromium.launch({ channel: process.env.PW_CHANNEL || undefined })
try {
  const page = await browser.newPage()
  await page.goto(pathToFileURL(path.join(dir, 'index.html')).href, { waitUntil: 'networkidle' })
  // Lazy images and collapsed FAQ answers must be in the PDF.
  await page.evaluate(async () => {
    document.querySelectorAll('details').forEach((d) => (d.open = true))
    const imgs = Array.from(document.images)
    imgs.forEach((i) => (i.loading = 'eager'))
    await Promise.all(imgs.map((i) => (i.complete ? null : new Promise((r) => { i.onload = i.onerror = r }))))
    await document.fonts.ready
  })
  await page.emulateMedia({ media: 'print' })
  await page.pdf({
    path: out,
    format: 'A4',
    printBackground: true,
    preferCSSPageSize: true,
    displayHeaderFooter: true,
    headerTemplate: '<span></span>',
    footerTemplate:
      '<div style="width:100%;font:8px sans-serif;color:#667;padding:0 13mm;display:flex;justify-content:space-between"><span>GARDA — Panduan Sales &amp; Presales</span><span><span class="pageNumber"></span> / <span class="totalPages"></span></span></div>',
  })
  console.log(`wrote ${path.relative(process.cwd(), out)}`)
} finally {
  await browser.close()
}
