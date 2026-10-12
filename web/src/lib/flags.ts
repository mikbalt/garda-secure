import { translate, type Key } from '../i18n/context'
import { en } from '../i18n/en'
import type { Lang } from './time'

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
] as const

/** Label + explanation of a flag; unknown flags get a generic text. */
export function flagText(flag: string, lang: Lang): { label: string; desc: string } {
  const labelKey = `flag.${flag}` as Key
  const descKey = `flag.${flag}.desc` as Key
  if (labelKey in en && descKey in en) return { label: translate(lang, labelKey), desc: translate(lang, descKey) }
  const human = flag.replace(/[_-]+/g, ' ').replace(/^\w/, (c) => c.toUpperCase())
  return { label: human, desc: translate(lang, 'flag.unknown.desc', { flag }) }
}
