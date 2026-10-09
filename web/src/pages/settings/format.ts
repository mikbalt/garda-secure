import { localeOf, type Lang } from '../../lib/time'

/** "2 menit lalu" / "2 minutes ago" for an ISO timestamp (now = Date.now()). */
export function relativeTime(iso: string | undefined | null, lang: Lang, now = Date.now()): string {
  if (!iso) return '—'
  const diff = (Date.parse(iso) - now) / 1000
  const abs = Math.abs(diff)
  const rtf = new Intl.RelativeTimeFormat(localeOf(lang), { numeric: 'auto' })
  if (abs < 45) return rtf.format(0, 'second')
  if (abs < 3600) return rtf.format(Math.round(diff / 60), 'minute')
  if (abs < 86_400) return rtf.format(Math.round(diff / 3600), 'hour')
  if (abs < 86_400 * 30) return rtf.format(Math.round(diff / 86_400), 'day')
  if (abs < 86_400 * 365) return rtf.format(Math.round(diff / (86_400 * 30)), 'month')
  return rtf.format(Math.round(diff / (86_400 * 365)), 'year')
}

/** A tablet counts as online when it synced in the last 10 minutes. */
export const ONLINE_MS = 10 * 60_000

export function isOnline(lastSeen: string | undefined, now = Date.now()): boolean {
  return !!lastSeen && now - Date.parse(lastSeen) < ONLINE_MS
}
