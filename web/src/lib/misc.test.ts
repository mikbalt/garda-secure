import { describe, expect, it } from 'vitest'
import { loadConfig, parseIdleMinutes, parseRealms, resolveDemo } from '../config'
import { mapSettled } from './async'
import { diffJson, flatten, showValue } from './diff'
import { idlePhase, warningMs } from './idle'
import { safeMediaUrl } from './url'

describe('safeMediaUrl (WEB-9)', () => {
  const origin = 'https://patrol.example.com'
  it('accepts https, blob and same-origin URLs', () => {
    expect(safeMediaUrl('https://patrol.example.com:9443/photos/a.jpg?X-Amz=1', { origin })).toBe('https://patrol.example.com:9443/photos/a.jpg?X-Amz=1')
    expect(safeMediaUrl('blob:https://patrol.example.com/123', { origin })).toBe('blob:https://patrol.example.com/123')
    expect(safeMediaUrl('/media/a.jpg', { origin })).toBe('https://patrol.example.com/media/a.jpg')
  })
  it('rejects dangerous or foreign schemes', () => {
    expect(safeMediaUrl('javascript:alert(1)', { origin })).toBeUndefined()
    expect(safeMediaUrl('data:text/html,<script>alert(1)</script>', { origin })).toBeUndefined()
    expect(safeMediaUrl('http://evil.example/a.jpg', { origin })).toBeUndefined()
    expect(safeMediaUrl('ftp://x/a.jpg', { origin })).toBeUndefined()
    expect(safeMediaUrl('', { origin })).toBeUndefined()
  })
  it('allows inline images only when asked (demo)', () => {
    const svg = 'data:image/svg+xml;charset=utf-8,%3Csvg%3E'
    expect(safeMediaUrl(svg, { origin })).toBeUndefined()
    expect(safeMediaUrl(svg, { origin, allowDataImages: true })).toBe(svg)
  })
  it('allows plain http only for the dev server', () => {
    expect(safeMediaUrl('http://localhost:9000/a.jpg', { origin, allowHttp: true })).toBe('http://localhost:9000/a.jpg')
  })
})

describe('runtime config', () => {
  it('parses realms', () => {
    expect(parseRealms('a:Client A, b ,:x, c:')).toEqual([
      { id: 'a', label: 'Client A' },
      { id: 'b', label: 'b' },
      { id: 'c', label: 'c' },
    ])
  })

  it('refuses runtime demo mode next to a real server unless forced', () => {
    expect(resolveDemo({ demo: true }, undefined)).toEqual({ demo: true, refused: false })
    expect(resolveDemo({ demo: true, apiUrl: '/api/v1', keycloakUrl: '/auth' }, undefined)).toEqual({ demo: true, refused: false })
    expect(resolveDemo({ demo: true, keycloakUrl: 'https://patrol.example.com/auth' }, undefined)).toEqual({ demo: false, refused: true })
    expect(resolveDemo({ demo: 'true', apiUrl: 'https://patrol.example.com/api/v1', demoForce: 'true' }, undefined)).toEqual({ demo: true, refused: false })
    expect(resolveDemo({ demo: false }, 'true')).toEqual({ demo: true, refused: false })
    expect(resolveDemo({}, undefined)).toEqual({ demo: false, refused: false })
  })

  it('reads the idle logout minutes', () => {
    expect(parseIdleMinutes(undefined, undefined)).toBe(30)
    expect(parseIdleMinutes('15')).toBe(15)
    expect(parseIdleMinutes(0)).toBe(0)
    expect(parseIdleMinutes('abc', '20')).toBe(20)
    expect(parseIdleMinutes(-5)).toBe(30)
    const cfg = loadConfig({ idleLogoutMinutes: 10, apiUrl: '/api/v1/' }, {})
    expect(cfg.idleLogoutMinutes).toBe(10)
    expect(cfg.apiUrl).toBe('/api/v1')
  })
})

describe('idle logout state', () => {
  const min = 60_000
  it('warns in the last minute and expires at the timeout', () => {
    expect(warningMs(30 * min)).toBe(60_000)
    expect(warningMs(1 * min)).toBe(30_000)
    expect(idlePhase(0 + 28 * min, 0, 30 * min)).toEqual({ kind: 'active' })
    expect(idlePhase(29 * min + 15_000, 0, 30 * min)).toEqual({ kind: 'warning', secondsLeft: 45 })
    expect(idlePhase(30 * min, 0, 30 * min)).toEqual({ kind: 'expired' })
  })
})

describe('mapSettled', () => {
  it('limits concurrency, keeps order and isolates failures', async () => {
    let inFlight = 0
    let peak = 0
    const out = await mapSettled([1, 2, 3, 4, 5, 6], 2, async (n) => {
      inFlight++
      peak = Math.max(peak, inFlight)
      await new Promise((r) => setTimeout(r, 5))
      inFlight--
      if (n === 3) throw new Error('boom')
      return n * 10
    })
    expect(peak).toBe(2)
    expect(out.map((r) => (r.status === 'fulfilled' ? r.value : 'x'))).toEqual([10, 20, 'x', 40, 50, 60])
  })
  it('handles an empty list', async () => {
    await expect(mapSettled([], 4, async () => 1)).resolves.toEqual([])
  })
})

describe('audit diff', () => {
  it('flattens nested objects', () => {
    expect(flatten({ a: { b: 1, c: [1, 2] }, d: null })).toEqual({ 'a.b': 1, 'a.c': [1, 2], d: null })
  })
  it('lists changed fields first', () => {
    const rows = diffJson({ status: 'absent', name: 'A', x: { y: 1 } }, { status: 'present', name: 'A', x: { y: 2 }, z: true })
    expect(rows.filter((r) => r.changed).map((r) => r.path)).toEqual(['status', 'x.y', 'z'])
    expect(rows[rows.length - 1]).toEqual({ path: 'name', before: 'A', after: 'A', changed: false })
  })
  it('handles a missing side', () => {
    expect(diffJson(undefined, { a: 1 })).toEqual([{ path: 'a', before: undefined, after: 1, changed: true }])
    expect(showValue(undefined)).toBe('—')
    expect(showValue(null)).toBe('null')
    expect(showValue({ a: 1 })).toBe('{"a":1}')
  })
})
