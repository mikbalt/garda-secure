import { AxiosError, AxiosHeaders, type AxiosResponse, type InternalAxiosRequestConfig } from 'axios'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { SosAlert } from '../types'

const apiGet = vi.fn()
vi.mock('../api/client', async (orig) => ({ ...(await orig<typeof import('../api/client')>()), apiGet: (...a: unknown[]) => apiGet(...a) }))

const { fetchActiveSos } = await import('./useActiveSos')

function httpError(status: number): AxiosError {
  const config = { headers: new AxiosHeaders() } as InternalAxiosRequestConfig
  return new AxiosError('x', 'ERR_BAD_RESPONSE', config, {}, { status, statusText: '', data: {}, headers: {}, config } as AxiosResponse)
}

const alert = (id: string, site: string, firedMin: number, ack = false): SosAlert => ({
  id,
  guard_id: 'g',
  site_id: site,
  fired_at: new Date(Date.UTC(2026, 9, 12, 10, firedMin)).toISOString(),
  received_at: new Date(Date.UTC(2026, 9, 12, 10, firedMin)).toISOString(),
  acknowledged_at: ack ? '2026-10-12T11:00:00Z' : undefined,
})

describe('fetchActiveSos (GET /sos/active)', () => {
  beforeEach(() => apiGet.mockReset())

  it('uses the realm-wide endpoint once, newest first, without acknowledged alerts', async () => {
    apiGet.mockResolvedValueOnce([alert('a', 's1', 1), alert('b', 's2', 5), alert('c', 's2', 3, true)])
    const out = await fetchActiveSos()
    expect(apiGet).toHaveBeenCalledTimes(1)
    expect(apiGet).toHaveBeenCalledWith('/sos/active')
    expect(out.map((a) => a.id)).toEqual(['b', 'a'])
  })

  it('accepts a {rows} body', async () => {
    apiGet.mockResolvedValueOnce({ rows: [alert('a', 's1', 1)] })
    expect((await fetchActiveSos()).map((a) => a.id)).toEqual(['a'])
  })

  it('propagates errors (no per-site fallback any more)', async () => {
    apiGet.mockRejectedValueOnce(httpError(404))
    await expect(fetchActiveSos()).rejects.toBeInstanceOf(AxiosError)
    expect(apiGet).toHaveBeenCalledTimes(1)
  })
})
