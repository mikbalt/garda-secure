import { act, renderHook } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const post = vi.fn()
vi.mock('../api/client', () => ({ default: { post: (...a: unknown[]) => post(...a) } }))
vi.mock('../config', () => ({ config: { apiUrl: '/api/v1', demo: false } }))

const { usePatrolLive, wsUrl } = await import('./usePatrolLive')

class FakeWS {
  static all: FakeWS[] = []
  onopen: (() => void) | null = null
  onmessage: ((e: { data: string }) => void) | null = null
  onclose: (() => void) | null = null
  closed = false
  constructor(public url: string) {
    FakeWS.all.push(this)
  }
  close() {
    this.closed = true
  }
  open() {
    this.onopen?.()
  }
  send(data: unknown) {
    this.onmessage?.({ data: JSON.stringify(data) })
  }
  drop() {
    this.onclose?.()
  }
}

function setup(siteId = 's1') {
  const qc = new QueryClient()
  const invalidate = vi.spyOn(qc, 'invalidateQueries')
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  return { invalidate, ...renderHook(() => usePatrolLive(siteId), { wrapper }) }
}

const flush = () => act(async () => {})

beforeEach(() => {
  FakeWS.all = []
  post.mockReset().mockResolvedValue({ data: { ticket: 'tk', expires_in: 30 } })
  vi.stubGlobal('WebSocket', FakeWS)
  vi.spyOn(Math, 'random').mockReturnValue(1)
  vi.useFakeTimers()
})
afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('wsUrl', () => {
  it('builds wss from an https page and a relative API base', () => {
    expect(wsUrl('/api/v1', { protocol: 'https:', host: 'garda.example' } as Location, 'tk')).toBe('wss://garda.example/api/v1/ws/patrol?ticket=tk')
  })
  it('builds ws from an http page and keeps an absolute base host', () => {
    expect(wsUrl('http://api.local:8080/api/v1', { protocol: 'http:', host: 'x' } as Location, 'a b')).toBe('ws://api.local:8080/api/v1/ws/patrol?ticket=a+b')
  })
})

describe('usePatrolLive', () => {
  it('requests a ticket for the site and reports connected once the socket opens', async () => {
    const { result } = setup('s1')
    await flush()
    expect(post).toHaveBeenCalledWith('/ws/ticket', { site_id: 's1' })
    expect(FakeWS.all[0].url).toContain('/api/v1/ws/patrol?ticket=tk')
    expect(result.current.connected).toBe(false)
    act(() => FakeWS.all[0].open())
    expect(result.current.connected).toBe(true)
  })

  it('invalidates the live query on patrol_changed for its site only', async () => {
    const { invalidate } = setup('s1')
    await flush()
    act(() => FakeWS.all[0].open())
    act(() => FakeWS.all[0].send({ type: 'patrol_changed', site_id: 's2' }))
    expect(invalidate).not.toHaveBeenCalled()
    act(() => FakeWS.all[0].send({ type: 'patrol_changed', site_id: 's1' }))
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['patrol-live', 's1'] })
  })

  it('stays disconnected without throwing when the ticket request fails, then retries', async () => {
    post.mockRejectedValueOnce(new Error('boom'))
    const { result } = setup()
    await flush()
    expect(result.current.connected).toBe(false)
    expect(FakeWS.all).toHaveLength(0)
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000)
    })
    expect(FakeWS.all).toHaveLength(1)
  })

  it('reconnects with doubling backoff after the socket drops', async () => {
    const { result } = setup()
    await flush()
    act(() => FakeWS.all[0].open())
    act(() => FakeWS.all[0].drop())
    expect(result.current.connected).toBe(false)
    await act(async () => {
      await vi.advanceTimersByTimeAsync(999)
    })
    expect(FakeWS.all).toHaveLength(1)
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1)
    })
    expect(FakeWS.all).toHaveLength(2)
    act(() => FakeWS.all[1].drop()) // never opened: next wait is 2 s
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1999)
    })
    expect(FakeWS.all).toHaveLength(2)
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1)
    })
    expect(FakeWS.all).toHaveLength(3)
  })

  it('closes the socket and stops retrying on unmount', async () => {
    const { unmount } = setup()
    await flush()
    act(() => FakeWS.all[0].open())
    unmount()
    expect(FakeWS.all[0].closed).toBe(true)
    FakeWS.all[0].drop()
    await act(async () => {
      await vi.advanceTimersByTimeAsync(60_000)
    })
    expect(FakeWS.all).toHaveLength(1)
  })
})
