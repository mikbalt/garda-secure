import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  browserPushInfo,
  derivePushState,
  detectEnv,
  disablePush,
  enablePush,
  fetchPushConfig,
  keyMatches,
  pushApi,
  safeAppPath,
  thisDeviceLabel,
  type PushEnv,
} from '../lib/push'
import { notificationPermission } from '../lib/alarm'

export const PUSH_KEYS = {
  config: ['push', 'config'] as const,
  subscriptions: ['push', 'subscriptions'] as const,
  browser: ['push', 'browser'] as const,
  preferences: ['push', 'preferences'] as const,
}

/** Notification permission, refreshed when it changes in the site settings or the tab regains focus. */
function usePermission() {
  const [permission, setPermission] = useState(notificationPermission)
  useEffect(() => {
    const refresh = () => setPermission(notificationPermission())
    let status: PermissionStatus | undefined
    let cancelled = false
    try {
      navigator.permissions
        ?.query({ name: 'notifications' as PermissionName })
        .then((s) => {
          if (cancelled) return
          status = s
          s.onchange = refresh
        })
        .catch(() => undefined)
    } catch {
      /* Permissions API unavailable */
    }
    window.addEventListener('focus', refresh)
    return () => {
      cancelled = true
      if (status) status.onchange = null
      window.removeEventListener('focus', refresh)
    }
  }, [])
  return [permission, setPermission] as const
}

/**
 * Push notifications of this browser: derived state (see `PushState`), the
 * user's subscriptions on the server, and enable / disable actions.
 */
export function usePush({ env: envOverride }: { env?: PushEnv } = {}) {
  const env = useMemo(() => envOverride ?? detectEnv(), [envOverride])
  const qc = useQueryClient()
  const [permission, setPermission] = usePermission()
  const usable = env.supported && !(env.ios && !env.standalone)

  const config = useQuery({ queryKey: PUSH_KEYS.config, queryFn: fetchPushConfig, enabled: usable, staleTime: 5 * 60_000 })
  const rows = useQuery({ queryKey: PUSH_KEYS.subscriptions, queryFn: () => pushApi.list() })
  const browser = useQuery({ queryKey: PUSH_KEYS.browser, queryFn: browserPushInfo, enabled: usable, staleTime: Infinity })

  const vapidKey = config.data?.webpush?.vapid_public_key
  const matches = keyMatches(browser.data?.key, vapidKey)
  const state = derivePushState({
    env,
    permission,
    config: config.data,
    error: usable && (config.isError || rows.isError),
    browserEndpoint: browser.data ? browser.data.endpoint : undefined,
    serverEndpoints: rows.data?.map((r) => r.endpoint),
    keyMatches: matches,
  })

  const refresh = async () => {
    setPermission(notificationPermission())
    await Promise.all([qc.invalidateQueries({ queryKey: PUSH_KEYS.browser }), qc.invalidateQueries({ queryKey: PUSH_KEYS.subscriptions })])
  }

  const enable = useMutation({
    mutationFn: () => enablePush({ vapidKey: vapidKey ?? '', label: thisDeviceLabel(), rows: rows.data }),
    onSettled: refresh,
  })
  const disable = useMutation({
    mutationFn: () => disablePush({ rows: rows.data ?? [] }),
    onSettled: refresh,
  })

  const endpoint = browser.data?.endpoint ?? null
  return {
    state,
    env,
    permission,
    config: config.data,
    /** The user's subscriptions on the server (all devices). */
    rows: rows.data ?? [],
    rowsQuery: rows,
    /** Why the state is `error`. */
    error: config.error ?? rows.error,
    /** Endpoint of this browser's subscription, if any. */
    endpoint,
    /** Demo mode: this browser's subscription is simulated. */
    simulated: !!browser.data?.simulated,
    /** The browser is subscribed with an older server key: it must re-enable. */
    keyRotated: !!browser.data?.endpoint && !matches,
    enable,
    disable,
    busy: enable.isPending || disable.isPending,
    retry: () => Promise.all([config.refetch(), rows.refetch(), browser.refetch()]),
  }
}

/**
 * Notification clicks: the service worker focuses an open GARDA window and
 * posts `{type: 'garda:navigate', url}`; navigate inside the SPA.
 */
export function useServiceWorkerNavigation() {
  const navigate = useNavigate()
  useEffect(() => {
    if (typeof navigator === 'undefined' || !('serviceWorker' in navigator)) return
    const sw = navigator.serviceWorker
    const onMessage = (e: MessageEvent) => {
      const d = e.data as { type?: unknown; url?: unknown } | null
      if (d && d.type === 'garda:navigate') navigate(safeAppPath(d.url))
    }
    sw.addEventListener('message', onMessage)
    return () => sw.removeEventListener('message', onMessage)
  }, [navigate])
}
