/** Helpers shared by the people pages (guards, schedules, attendance). */

/** Up to two initials of a person's name ("Budi Santoso" → "BS"). */
export function initials(name: string | undefined | null): string {
  const parts = (name ?? '').trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) return '?'
  return parts
    .slice(0, 2)
    .map((p) => p[0]!.toUpperCase())
    .join('')
}

const AVATAR_TONES = [
  'bg-primary-50 text-primary-700 ring-primary-100',
  'bg-emerald-50 text-emerald-700 ring-emerald-100',
  'bg-amber-50 text-amber-800 ring-amber-100',
  'bg-violet-50 text-violet-700 ring-violet-100',
  'bg-rose-50 text-rose-700 ring-rose-100',
  'bg-cyan-50 text-cyan-700 ring-cyan-100',
  'bg-orange-50 text-orange-700 ring-orange-100',
]

/** Stable avatar colour for a name (same person → same colour everywhere). */
export function avatarTone(name: string | undefined | null): string {
  let h = 0
  for (const ch of name ?? '') h = (h * 31 + ch.charCodeAt(0)) >>> 0
  return AVATAR_TONES[h % AVATAR_TONES.length]
}

export type DayPeriod = 'morning' | 'afternoon' | 'night'

/** Shift period by start hour: pagi 05–11, siang 11–17, malam otherwise. */
export function periodOf(hhmm: string): DayPeriod {
  const h = Number(hhmm.slice(0, 2))
  if (h >= 5 && h < 11) return 'morning'
  if (h >= 11 && h < 17) return 'afternoon'
  return 'night'
}

/** Chip classes per shift period (light background, readable text). */
export const periodChip: Record<DayPeriod, string> = {
  morning: 'bg-amber-50 text-amber-900 ring-amber-200 hover:bg-amber-100',
  afternoon: 'bg-sky-50 text-sky-900 ring-sky-200 hover:bg-sky-100',
  night: 'bg-indigo-50 text-indigo-900 ring-indigo-200 hover:bg-indigo-100',
}

/** One-off (non-pattern) shift chips: white with a dashed border. */
export const periodDashed: Record<DayPeriod, string> = {
  morning: 'border border-dashed border-amber-400 bg-white text-amber-900 hover:bg-amber-50',
  afternoon: 'border border-dashed border-sky-400 bg-white text-sky-900 hover:bg-sky-50',
  night: 'border border-dashed border-indigo-400 bg-white text-indigo-900 hover:bg-indigo-50',
}

export const periodDot: Record<DayPeriod, string> = {
  morning: 'bg-amber-400',
  afternoon: 'bg-sky-500',
  night: 'bg-indigo-500',
}

/** Case/diacritic-insensitive "contains" match. */
export function matches(haystack: (string | undefined | null)[], needle: string): boolean {
  const n = needle.trim().toLowerCase()
  if (!n) return true
  return haystack.some((h) => (h ?? '').toLowerCase().includes(n))
}

type Lang = 'id' | 'en'
const locale = (lang: Lang) => (lang === 'id' ? 'id-ID' : 'en-GB')
const utcDate = (date: string) => new Date(`${date}T00:00:00Z`)

/** "Jumat, 9 Oktober 2026" for a calendar date (YYYY-MM-DD). */
export function formatLongDay(date: string, lang: Lang): string {
  return new Intl.DateTimeFormat(locale(lang), { timeZone: 'UTC', weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' }).format(utcDate(date))
}

/** "6 – 12 Okt 2026" / "29 Sep – 5 Okt 2026" for a date range. */
export function formatDayRange(from: string, to: string, lang: Lang): string {
  const fmt = (d: string, o: Intl.DateTimeFormatOptions) => new Intl.DateTimeFormat(locale(lang), { timeZone: 'UTC', ...o }).format(utcDate(d))
  const end = fmt(to, { day: 'numeric', month: 'short', year: 'numeric' })
  if (from.slice(0, 7) === to.slice(0, 7)) return `${fmt(from, { day: 'numeric' })} – ${end}`
  if (from.slice(0, 4) === to.slice(0, 4)) return `${fmt(from, { day: 'numeric', month: 'short' })} – ${end}`
  return `${fmt(from, { day: 'numeric', month: 'short', year: 'numeric' })} – ${end}`
}

/** Short weekday + day of month for column headers: ["Sen", "06"]. */
export function dayHeader(date: string, lang: Lang): [string, string] {
  const wd = new Intl.DateTimeFormat(locale(lang), { timeZone: 'UTC', weekday: 'short' }).format(utcDate(date))
  return [wd.replace(/\.$/, ''), date.slice(8, 10)]
}

/** Hours between two instants, rounded to 0.5. */
export function hoursBetween(startIso: string, endIso: string): number {
  return Math.round(((Date.parse(endIso) - Date.parse(startIso)) / 3_600_000) * 2) / 2
}
