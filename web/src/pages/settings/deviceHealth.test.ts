import { describe, expect, it } from 'vitest'
import type { Device, DeviceHealth } from '../../types'
import { deviceHealth, formatOffset } from './deviceHealth'
import { deviceUsername, slug } from './accountUtil'

const NOW = Date.parse('2026-10-12T08:00:00Z')
const ago = (min: number) => new Date(NOW - min * 60_000).toISOString()

const beacon = (over: Partial<DeviceHealth> = {}): DeviceHealth => ({
  app_version: '1.4.0',
  outbox_pending: 0,
  outbox_failed: 0,
  sos_pending: 0,
  last_sync: ago(2),
  roster_synced_at: ago(30),
  clock_offset_ms: 500,
  private_ca: true,
  errors: [],
  received_at: ago(5),
  ...over,
})
const device = (health?: DeviceHealth, health_at = ago(5), status: Device['status'] = 'active') => ({ status, health, health_at })
const keys = (d: ReturnType<typeof device>) => deviceHealth(d, NOW).issues.map((i) => i.key)

describe('deviceHealth', () => {
  it('is ok for a fresh, clean beacon', () => {
    expect(deviceHealth(device(beacon()), NOW)).toEqual({ level: 'ok', issues: [] })
  })

  it('ignores pending and revoked tablets', () => {
    expect(deviceHealth(device(undefined, undefined, 'pending'), NOW).level).toBe('ok')
  })

  it('reports a missing beacon as unknown', () => {
    expect(deviceHealth(device(undefined, undefined), NOW)).toEqual({ level: 'unknown', issues: [{ key: 'noBeacon', level: 'warn' }] })
  })

  it('warns about a beacon older than an hour', () => {
    expect(keys(device(beacon(), ago(61)))).toEqual(['stale'])
    expect(keys(device(beacon(), ago(59)))).toEqual([])
  })

  it('warns about clock skew beyond 2 minutes either way', () => {
    expect(keys(device(beacon({ clock_offset_ms: 121_000 })))).toEqual(['clockSkew'])
    expect(keys(device(beacon({ clock_offset_ms: -125_000 })))).toEqual(['clockSkew'])
    expect(keys(device(beacon({ clock_offset_ms: 119_000 })))).toEqual([])
  })

  it('detects a stuck outbox', () => {
    expect(keys(device(beacon({ outbox_pending: 4, last_sync: ago(90) })))).toEqual(['outboxStuck'])
    expect(keys(device(beacon({ outbox_pending: 4, last_sync: ago(10) })))).toEqual([])
    expect(keys(device(beacon({ outbox_failed: 2 })))).toEqual(['outboxFailed'])
    const paused = deviceHealth(device(beacon({ outbox_paused: true, outbox_pending: 4, last_sync: ago(90) })), NOW)
    expect(paused.level).toBe('critical')
    expect(paused.issues.map((i) => i.key)).toEqual(['outboxPaused'])
  })

  it('treats an unsent SOS as critical', () => {
    expect(deviceHealth(device(beacon({ sos_pending: 1 })), NOW).level).toBe('critical')
  })

  it('escalates an old roster (unlock blocked after 7 days)', () => {
    expect(keys(device(beacon({ roster_synced_at: ago(25 * 60) })))).toEqual(['rosterOld'])
    expect(keys(device(beacon({ roster_synced_at: ago(8 * 24 * 60) })))).toEqual(['rosterBlocked'])
  })

  it('reports errors and a reset database', () => {
    expect(keys(device(beacon({ errors: ['boom'], db_key_reset: true })))).toEqual(['dbKeyReset', 'errors'])
  })
})

describe('formatOffset', () => {
  it('splits into minutes and seconds with a sign', () => {
    expect(formatOffset(192_000)).toEqual({ sign: '+', minutes: 3, seconds: 12 })
    expect(formatOffset(-61_400)).toEqual({ sign: '−', minutes: 1, seconds: 1 })
    expect(formatOffset(200)).toEqual({ sign: '', minutes: 0, seconds: 0 })
  })
})

describe('tablet account usernames (port of kcadmin.Slug)', () => {
  it('slugs like the API', () => {
    expect(slug('Pos Utama Cikarang')).toBe('pos-utama-cikarang')
    expect(slug('  Gate #2 / Lobby_B.  ')).toBe('gate-2-lobby-b')
    expect(slug('Café')).toBe('caf')
    expect(slug('x'.repeat(60))).toHaveLength(40)
  })

  it('previews device-<slug> or nothing', () => {
    expect(deviceUsername('Tablet Lobby')).toBe('device-tablet-lobby')
    expect(deviceUsername('###')).toBe('')
  })
})
