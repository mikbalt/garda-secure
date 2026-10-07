/**
 * Date/time helpers. The API stores UTC; everything shown or entered in the UI
 * is in the selected site's IANA time zone.
 */

function parts(date: Date, tz: string): Record<string, string> {
  const fmt = new Intl.DateTimeFormat('en-CA', {
    timeZone: tz,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hourCycle: 'h23',
  })
  const out: Record<string, string> = {}
  for (const p of fmt.formatToParts(date)) out[p.type] = p.value
  return out
}

function safeTz(tz: string | undefined): string {
  if (!tz) return 'UTC'
  try {
    new Intl.DateTimeFormat('en', { timeZone: tz })
    return tz
  } catch {
    return 'UTC'
  }
}

/** Calendar date (YYYY-MM-DD) of an instant in a time zone. */
export function localDateOf(iso: string | Date, tz: string): string {
  const p = parts(typeof iso === 'string' ? new Date(iso) : iso, safeTz(tz))
  return `${p.year}-${p.month}-${p.day}`
}

/** Wall-clock time (HH:MM) of an instant in a time zone. */
export function localTimeOf(iso: string | Date, tz: string): string {
  const p = parts(typeof iso === 'string' ? new Date(iso) : iso, safeTz(tz))
  return `${p.hour}:${p.minute}`
}

export function todayIn(tz: string): string {
  return localDateOf(new Date(), tz)
}

export function monthIn(tz: string): string {
  return todayIn(tz).slice(0, 7)
}

function dateToUtc(date: string): Date {
  const [y, m, d] = date.split('-').map(Number)
  return new Date(Date.UTC(y, m - 1, d))
}

function utcToDate(d: Date): string {
  return d.toISOString().slice(0, 10)
}

export function addDays(date: string, n: number): string {
  const d = dateToUtc(date)
  d.setUTCDate(d.getUTCDate() + n)
  return utcToDate(d)
}

/** ISO weekday: 1 = Monday … 7 = Sunday. */
export function isoWeekday(date: string): number {
  const wd = dateToUtc(date).getUTCDay()
  return wd === 0 ? 7 : wd
}

export function startOfWeek(date: string): string {
  return addDays(date, 1 - isoWeekday(date))
}

/** Offset (ms) of tz from UTC at the given instant. */
function tzOffset(instant: number, tz: string): number {
  const p = parts(new Date(instant), tz)
  const asUtc = Date.UTC(+p.year, +p.month - 1, +p.day, +p.hour, +p.minute, +p.second)
  return asUtc - Math.floor(instant / 1000) * 1000
}

/** Converts a site-local date + HH:MM to an RFC 3339 UTC timestamp. */
export function zonedToUtcIso(date: string, time: string, tz: string): string {
  const zone = safeTz(tz)
  const [y, m, d] = date.split('-').map(Number)
  const [hh, mm] = time.split(':').map(Number)
  const wall = Date.UTC(y, m - 1, d, hh, mm)
  let guess = wall - tzOffset(wall, zone)
  // Second pass handles DST transitions between the guess and the result.
  guess = wall - tzOffset(guess, zone)
  return new Date(guess).toISOString().replace(/\.\d{3}Z$/, 'Z')
}

export type Lang = 'id' | 'en'

export function localeOf(lang: Lang): string {
  return lang === 'id' ? 'id-ID' : 'en-GB'
}

export function formatDateTime(iso: string | undefined | null, tz: string, lang: Lang): string {
  if (!iso) return '—'
  return new Intl.DateTimeFormat(localeOf(lang), {
    timeZone: safeTz(tz),
    day: '2-digit',
    month: 'short',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23',
  }).format(new Date(iso))
}

export function formatTime(iso: string | undefined | null, tz: string): string {
  if (!iso) return '—'
  return localTimeOf(iso, tz)
}

/** Formats a calendar date (YYYY-MM-DD) for headings, e.g. "Sen, 07 Okt 2026". */
export function formatDay(date: string, lang: Lang, withWeekday = true): string {
  return new Intl.DateTimeFormat(localeOf(lang), {
    timeZone: 'UTC',
    weekday: withWeekday ? 'short' : undefined,
    day: '2-digit',
    month: 'short',
    year: 'numeric',
  }).format(dateToUtc(date))
}

/** Short weekday name for ISO weekday 1..7. */
export function weekdayName(isoDay: number, lang: Lang): string {
  // 2024-01-01 was a Monday.
  return new Intl.DateTimeFormat(localeOf(lang), { timeZone: 'UTC', weekday: 'short' }).format(
    dateToUtc(addDays('2024-01-01', isoDay - 1))
  )
}

export function formatNumber(n: number | undefined | null, lang: Lang, digits = 0): string {
  if (n === undefined || n === null || Number.isNaN(n)) return '—'
  return new Intl.NumberFormat(localeOf(lang), {
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  }).format(n)
}

export function formatBytes(n: number | undefined | null): string {
  if (!n) return '—'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = n
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

/** Days between two calendar dates (inclusive count). */
export function daysBetween(from: string, to: string): number {
  return Math.round((dateToUtc(to).getTime() - dateToUtc(from).getTime()) / 86_400_000) + 1
}

export function mapsUrl(lat: number, lng: number): string {
  return `https://maps.google.com/?q=${lat.toFixed(6)},${lng.toFixed(6)}`
}

/** RFC 3339 → value for <input type="datetime-local"> in the site's time zone. */
export function isoToLocalInput(iso: string | undefined | null, tz: string): string {
  if (!iso) return ''
  return `${localDateOf(iso, tz)}T${localTimeOf(iso, tz)}`
}

/** <input type="datetime-local"> value in the site's time zone → RFC 3339 UTC. */
export function localInputToIso(value: string, tz: string): string | undefined {
  if (!value) return undefined
  const [date, time] = value.split('T')
  if (!date || !time) return undefined
  return zonedToUtcIso(date, time.slice(0, 5), tz)
}
