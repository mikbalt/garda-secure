/**
 * Tablet health warnings from the latest telemetry beacon (D-049, docs/api.md
 * `POST /device/telemetry`). The tablet sends one every 15 minutes while it
 * has a connection, so a beacon older than an hour means trouble.
 */
import type { Device } from '../../types'

export const STALE_BEACON_MS = 60 * 60_000
export const CLOCK_SKEW_MS = 2 * 60_000
/** Queued items not leaving the tablet for this long count as stuck. */
export const OUTBOX_STUCK_MS = 60 * 60_000
/** The tablet refuses PIN unlock after 7 days without a roster sync (D-039). */
export const ROSTER_WARN_MS = 24 * 3600_000
export const ROSTER_BLOCK_MS = 7 * 24 * 3600_000

export type HealthLevel = 'ok' | 'warn' | 'critical' | 'unknown'

export type HealthIssue =
  | 'noBeacon'
  | 'stale'
  | 'sosPending'
  | 'outboxPaused'
  | 'outboxFailed'
  | 'outboxStuck'
  | 'clockSkew'
  | 'rosterOld'
  | 'rosterBlocked'
  | 'dbKeyReset'
  | 'errors'

export interface HealthReport {
  level: HealthLevel
  issues: { key: HealthIssue; level: 'warn' | 'critical' }[]
}

const age = (iso: string | undefined | null, now: number) => (iso ? now - Date.parse(iso) : Infinity)

/** Evaluates a device's beacon. Only active tablets are expected to report. */
export function deviceHealth(d: Pick<Device, 'status' | 'health' | 'health_at'>, now = Date.now()): HealthReport {
  if (d.status !== 'active') return { level: 'ok', issues: [] }
  const h = d.health
  if (!h) return { level: 'unknown', issues: [{ key: 'noBeacon', level: 'warn' }] }
  const issues: HealthReport['issues'] = []
  const beaconAge = age(d.health_at ?? h.received_at, now)
  if (beaconAge > STALE_BEACON_MS) issues.push({ key: 'stale', level: 'warn' })
  if (h.sos_pending > 0) issues.push({ key: 'sosPending', level: 'critical' })
  if (h.outbox_paused) issues.push({ key: 'outboxPaused', level: 'critical' })
  if (h.outbox_failed > 0) issues.push({ key: 'outboxFailed', level: 'warn' })
  if (!h.outbox_paused && h.outbox_pending > 0 && age(h.last_sync, now) > OUTBOX_STUCK_MS) issues.push({ key: 'outboxStuck', level: 'warn' })
  if (Math.abs(h.clock_offset_ms ?? 0) > CLOCK_SKEW_MS) issues.push({ key: 'clockSkew', level: 'warn' })
  const roster = age(h.roster_synced_at, now)
  if (h.roster_synced_at && roster > ROSTER_BLOCK_MS) issues.push({ key: 'rosterBlocked', level: 'critical' })
  else if (h.roster_synced_at && roster > ROSTER_WARN_MS) issues.push({ key: 'rosterOld', level: 'warn' })
  if (h.db_key_reset) issues.push({ key: 'dbKeyReset', level: 'warn' })
  if ((h.errors?.length ?? 0) > 0) issues.push({ key: 'errors', level: 'warn' })
  const level = issues.some((i) => i.level === 'critical') ? 'critical' : issues.length ? 'warn' : 'ok'
  return { level, issues }
}

/** "+3 min 20 s" style clock offset, rounded to seconds. */
export function formatOffset(ms: number): { sign: '+' | '−' | ''; minutes: number; seconds: number } {
  const s = Math.round(Math.abs(ms) / 1000)
  return { sign: s === 0 ? '' : ms > 0 ? '+' : '−', minutes: Math.floor(s / 60), seconds: s % 60 }
}
