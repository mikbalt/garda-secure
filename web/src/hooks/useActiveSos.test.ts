import { AxiosError, AxiosHeaders, type AxiosResponse, type InternalAxiosRequestConfig } from 'axios'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { SosAlert } from '../types'

const apiGet = vi.fn()
vi.mock('../api/client', async (orig) => ({ ...(await orig<typeof import('../api/client')>()), apiGet: (...a: unknown[]) => apiGet(...a) }))

const { fetchActiveSos, resetSosEndpointDetection } = await import('./useActiveSos')

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

describe('fetchActiveSos (WEB-3)', () => {
  beforeEach(() => {
    apiGet.mockReset()
    resetSosEndpointDetection()
  })

  it('uses GET /sos/active when the API has it', async () => {
    apiGet.mockResolvedValueOnce([alert('a', 's1', 1), alert('b', 's2', 5), alert('c', 's2', 3, true)])
    const out = await fetchActiveSos(['s1', 's2'])
    expect(apiGet).toHaveBeenCalledTimes(1)
    expect(apiGet).toHaveBeenCalledWith('/sos/active')
    expect(out.map((a) => a.id)).toEqual(['b', 'a'])
  })

  it('falls back to per-site polling on 404 and remembers it', async () => {
    apiGet.mockImplementation(async (path: string, params?: { site_id?: string }) => {
      if (path === '/sos/active') throw httpError(404)
      if (params?.site_id === 's1') return [alert('a', 's1', 1)]
      if (params?.site_id === 's2') return [alert('b', 's2', 9)]
      return []
    })
    const first = await fetchActiveSos(['s1', 's2', 's3'])
    expect(first.map((a) => a.id)).toEqual(['b', 'a'])
    expect(apiGet).toHaveBeenCalledWith('/sos', { site_id: 's2', unack_only: true })
    apiGet.mockClear()
    await fetchActiveSos(['s1'])
    expect(apiGet).not.toHaveBeenCalledWith('/sos/active')
  })

  it('keeps alerts of reachable sites when one site fails', async () => {
    resetSosEndpointDetection(false)
    apiGet.mockImplementation(async (_p: string, params?: { site_id?: string }) => {
      if (params?.site_id === 'bad') throw httpError(500)
      return [alert(`x-${params?.site_id}`, params?.site_id ?? '', 2)]
    })
    const out = await fetchActiveSos(['bad', 'ok'])
    expect(out.map((a) => a.id)).toEqual(['x-ok'])
  })

  it('throws when every site fails', async () => {
    resetSosEndpointDetection(false)
    apiGet.mockRejectedValue(httpError(500))
    await expect(fetchActiveSos(['a', 'b'])).rejects.toBeInstanceOf(AxiosError)
  })

  it('does not fall back on other errors', async () => {
    apiGet.mockRejectedValueOnce(httpError(403))
    await expect(fetchActiveSos(['s1'])).rejects.toBeInstanceOf(AxiosError)
    expect(apiGet).toHaveBeenCalledTimes(1)
  })
})
