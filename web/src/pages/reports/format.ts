import { formatNumber, localeOf, type Lang } from '../../lib/time'

/** "2026-10" → "Oktober 2026". */
export function formatMonth(month: string, lang: Lang): string {
  const [y, m] = month.split('-').map(Number)
  if (!y || !m) return month
  return new Intl.DateTimeFormat(localeOf(lang), { timeZone: 'UTC', month: 'long', year: 'numeric' }).format(new Date(Date.UTC(y, m - 1, 1)))
}

/** Adds n months to "YYYY-MM". */
export function addMonths(month: string, n: number): string {
  const [y, m] = month.split('-').map(Number)
  const d = new Date(Date.UTC(y, m - 1 + n, 1))
  return d.toISOString().slice(0, 7)
}

/** Last calendar day of "YYYY-MM" as "YYYY-MM-DD". */
export function monthEnd(month: string): string {
  const [y, m] = month.split('-').map(Number)
  return new Date(Date.UTC(y, m, 0)).toISOString().slice(0, 10)
}

export function pct(v: number | undefined | null, lang: Lang): string {
  if (v === undefined || v === null || Number.isNaN(v)) return '—'
  return `${formatNumber(v, lang, 1)}%`
}

/** Ratio as a percentage, or undefined when the denominator is 0. */
export function ratio(part: number, whole: number): number | undefined {
  return whole > 0 ? (part / whole) * 100 : undefined
}

export function average(values: number[]): number | undefined {
  return values.length ? values.reduce((a, b) => a + b, 0) / values.length : undefined
}

export type PctTone = 'green' | 'yellow' | 'red'

/** Traffic-light tone for a compliance percentage. */
export function pctTone(v: number): PctTone {
  if (v >= 90) return 'green'
  if (v >= 75) return 'yellow'
  return 'red'
}
