import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

// Buku Jaga design tokens (TAQ-42/TAQ-43): the palette lives in src/index.css as
// "R G B" channel triplets. These tests keep the contrast table in DESIGN.md honest.
const css = readFileSync(resolve(__dirname, '../index.css'), 'utf8')

function block(selector: string): Record<string, [number, number, number]> {
  const m = css.match(new RegExp(`${selector.replace('.', '\\.')}\\s*\\{([^}]*)\\}`))
  if (!m) throw new Error(`no ${selector} block in index.css`)
  const out: Record<string, [number, number, number]> = {}
  for (const [, name, r, g, b] of m[1].matchAll(/--([\w-]+):\s*(\d+)\s+(\d+)\s+(\d+)\s*;/g)) {
    out[name] = [Number(r), Number(g), Number(b)]
  }
  return out
}

function lum([r, g, b]: [number, number, number]) {
  const f = (v: number) => {
    const s = v / 255
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4
  }
  return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b)
}
export function ratio(a: [number, number, number], b: [number, number, number]) {
  const [hi, lo] = [lum(a), lum(b)].sort((x, y) => y - x)
  return (hi + 0.05) / (lo + 0.05)
}

const themes = { light: block(':root'), dark: block('.dark') }

// [foreground, background, minimum ratio]
const textPairs: [string, string, number][] = [
  ['text', 'paper', 4.5],
  ['text', 'surface', 4.5],
  ['muted', 'paper', 4.5],
  ['muted', 'surface', 4.5],
  ['action-fg', 'action', 4.5],
  ['ok', 'ok-bg', 4.5],
  ['warn', 'warn-bg', 4.5],
  ['overdue', 'overdue-bg', 4.5],
  ['danger', 'danger-bg', 4.5],
  ['sos-fg', 'sos', 4.5],
  // Non-text: strong borders / focus ring must reach 3:1 against the page.
  ['border-strong', 'paper', 3],
  ['focus', 'paper', 3],
]

describe.each(Object.entries(themes))('%s tokens', (_name, t) => {
  it('defines every token', () => {
    for (const [fg, bg] of textPairs) {
      expect(t[fg], fg).toBeDefined()
      expect(t[bg], bg).toBeDefined()
    }
    expect(t['border']).toBeDefined()
  })
  it.each(textPairs)('%s on %s reaches %s:1', (fg, bg, min) => {
    expect(ratio(t[fg], t[bg])).toBeGreaterThanOrEqual(min)
  })
  it('keeps the gray scale complete', () => {
    for (const n of [50, 100, 200, 300, 400, 500, 600, 700, 800, 900, 950]) {
      expect(t[`gray-${n}`], `gray-${n}`).toBeDefined()
    }
  })
  it('keeps gray-500 (meta text) readable on the page and gray-50 background', () => {
    expect(ratio(t['gray-500'], t['gray-50'])).toBeGreaterThanOrEqual(4.5)
    expect(ratio(t['gray-600'], t['gray-50'])).toBeGreaterThanOrEqual(4.5)
  })
})

it('uses no gradients in the stylesheet', () => {
  expect(css).not.toMatch(/gradient\(/)
})
