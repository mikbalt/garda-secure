import type { Key } from '../i18n/context'
import type { Incident } from '../types'

export type AmendableField = 'type' | 'severity' | 'location' | 'description'

export const AMENDABLE: { field: AmendableField; label: Key }[] = [
  { field: 'type', label: 'inc.category' },
  { field: 'severity', label: 'severity.label' },
  { field: 'location', label: 'common.location' },
  { field: 'description', label: 'common.description' },
]

type Originals = Pick<Incident, 'original_type' | 'original_severity' | 'original_location' | 'original_description'>

/** Fields whose effective value differs from the filed one (D-043: `original_*` present). */
export function amendedFields(i: Originals): AmendableField[] {
  return AMENDABLE.filter(({ field }) => {
    const v = (i as Record<string, unknown>)[`original_${field}`]
    return v !== undefined && v !== null
  }).map(({ field }) => field)
}

/** "-6.294400, 107.153100" — incident GPS as plain text (on-prem: no map links). */
export function gpsText(lat: number | null | undefined, lng: number | null | undefined): string | null {
  if (lat == null || lng == null || !Number.isFinite(lat) || !Number.isFinite(lng)) return null
  return `${lat.toFixed(6)}, ${lng.toFixed(6)}`
}
