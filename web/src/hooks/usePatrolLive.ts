import { useEffect, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import apiClient from '../api/client'
import { config } from '../config'

/** Poll interval of the live view while the socket is up (safety net). */
export const LIVE_POLL_CONNECTED_MS = 60_000
/** Poll interval while the socket is down (D-014). */
export const LIVE_POLL_FALLBACK_MS = 10_000

const BACKOFF_START_MS = 1_000
const BACKOFF_MAX_MS = 30_000

/** `wss://host/api/v1/ws/patrol?ticket=…` from the API base (relative or absolute). */
export function wsUrl(apiBase: string, loc: Pick<Location, 'protocol' | 'host'>, ticket: string): string {
  const u = new URL(apiBase, `${loc.protocol}//${loc.host}`)
  u.protocol = u.protocol === 'https:' ? 'wss:' : 'ws:'
  u.pathname = `${u.pathname.replace(/\/+$/, '')}/ws/patrol`
  u.search = new URLSearchParams({ ticket }).toString()
  return u.toString()
}

/**
 * Live patrol nudges (issue #38): keeps a WebSocket to the site's patrol
 * channel and refetches `['patrol-live', siteId]` whenever the server says
 * something changed. The socket carries no data; REST stays the source of
 * truth. `connected` lets the caller poll slowly while it is up and fall back
 * to fast polling while it is down. Reconnects with backoff.
 */
export function usePatrolLive(siteId: string): { connected: boolean } {
  const qc = useQueryClient()
  const [connected, setConnected] = useState(false)

  useEffect(() => {
    // Demo mode has no backend: stay on polling.
    if (!siteId || config.demo || typeof WebSocket === 'undefined') return
    let cancelled = false
    let ws: WebSocket | null = null
    let timer: ReturnType<typeof setTimeout> | undefined
    let attempt = 0

    const retry = () => {
      if (cancelled) return
      const wait = Math.min(BACKOFF_MAX_MS, BACKOFF_START_MS * 2 ** attempt) * (0.5 + Math.random() * 0.5)
      attempt += 1
      timer = setTimeout(connect, wait)
    }

    async function connect() {
      let ticket: string
      try {
        const { data } = await apiClient.post<{ ticket: string }>('/ws/ticket', { site_id: siteId })
        ticket = data.ticket
      } catch {
        retry()
        return
      }
      if (cancelled) return
      const sock = new WebSocket(wsUrl(config.apiUrl, window.location, ticket))
      ws = sock
      sock.onopen = () => {
        attempt = 0
        setConnected(true)
      }
      sock.onmessage = (e) => {
        try {
          const m = JSON.parse(String(e.data)) as { type?: string; site_id?: string }
          if (m.type === 'patrol_changed' && m.site_id === siteId) void qc.invalidateQueries({ queryKey: ['patrol-live', siteId] })
        } catch {
          /* ignore malformed frames */
        }
      }
      sock.onclose = () => {
        if (ws === sock) ws = null
        if (cancelled) return
        setConnected(false)
        // Catch up on anything missed while the socket was down.
        void qc.invalidateQueries({ queryKey: ['patrol-live', siteId] })
        retry()
      }
    }

    void connect()
    return () => {
      cancelled = true
      clearTimeout(timer)
      setConnected(false)
      ws?.close()
    }
  }, [siteId, qc])

  return { connected }
}
