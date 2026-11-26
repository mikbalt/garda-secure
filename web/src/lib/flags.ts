import { translate, type Key } from '../i18n/context'
import { en } from '../i18n/en'
import { localDateOf, type Lang } from './time'
import type { FlagRow } from '../types'

/** Flags the API sets today or announced (D-033, D-035). */
export const KNOWN_FLAGS = [
  'late_sync',
  'clock_ahead',
  'backdated_on_time',
  'gps_far',
  'too_fast',
  'guard_unverified',
  'qr_invalid',
  'qr_unsigned',
  'qr_stale',
] as const

/** Label + explanation of a flag; unknown flags get a generic text. */
export function flagText(flag: string, lang: Lang): { label: string; desc: string } {
  const labelKey = `flag.${flag}` as Key
  const descKey = `flag.${flag}.desc` as Key
  if (labelKey in en && descKey in en) return { label: translate(lang, labelKey), desc: translate(lang, descKey) }
  const human = flag.replace(/[_-]+/g, ' ').replace(/^\w/, (c) => c.toUpperCase())
  return { label: human, desc: translate(lang, 'flag.unknown.desc', { flag }) }
}

/**
 * Where to see the record behind a flag: the page and tab, the flag's site and,
 * where the page filters by day, the day (in the site's time zone).
 */
export function flagRecordLink(row: Pick<FlagRow, 'kind' | 'event_at' | 'site_id'>, tz: string): string {
  const site = `site=${encodeURIComponent(row.site_id)}`
  const date = `date=${localDateOf(row.event_at, tz)}`
  switch (row.kind) {
    case 'attendance':
      return `/attendance?${site}&${date}`
    case 'incident':
      return `/incidents?tab=incidents&${site}&${date}`
    case 'sos':
      return `/incidents?tab=sos&${site}`
    case 'defect':
      return `/incidents?tab=defects&${site}`
    case 'visitor':
      return `/incidents?tab=visitors&${site}`
    case 'scan':
    case 'session':
      return `/patrol?tab=sessions&${site}&${date}`
  }
}
