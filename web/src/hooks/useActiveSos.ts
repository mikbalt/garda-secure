import { useQuery } from '@tanstack/react-query'
import { apiErrorStatus, apiGet } from '../api/client'
import { useSite } from '../context/site'
import { mapSettled } from '../lib/async'
import { useAuth } from './useAuth'
import type { SosAlert } from '../types'

/** Poll interval of the SOS banner. */
export const SOS_POLL_MS = 10_000
/** Parallel per-site requests while the realm-wide endpoint is missing. */
const SITE_CONCURRENCY = 4

/**
 * Whether the API has the realm-wide `GET /sos/active` (WEB-3). Detected on
 * the first poll: a 404 means an older API, and the per-site fallback is
 * used from then on. Once the endpoint is everywhere, set this to `true`
 * and delete the fallback.
 */
let activeEndpoint: boolean | undefined

/** Test hook. */
export function resetSosEndpointDetection(v?: boolean): void {
  activeEndpoint = v
}

function rows(data: unknown): SosAlert[] {
  if (Array.isArray(data)) return data as SosAlert[]
  const r = (data as { rows?: unknown } | null)?.rows
  return Array.isArray(r) ? (r as SosAlert[]) : []
}

const newestFirst = (a: SosAlert, b: SosAlert) => b.fired_at.localeCompare(a.fired_at)

/**
 * Unacknowledged SOS alerts of every site the user can see, newest first —
 * independent of the selected site.
 */
export async function fetchActiveSos(siteIds: readonly string[]): Promise<SosAlert[]> {
  if (activeEndpoint !== false) {
    try {
      const data = await apiGet<unknown>('/sos/active')
      activeEndpoint = true
      return rows(data).filter((a) => !a.acknowledged_at).sort(newestFirst)
    } catch (err) {
      if (apiErrorStatus(err) !== 404 || activeEndpoint === true) throw err
      activeEndpoint = false
    }
  }
  if (siteIds.length === 0) return []
  const results = await mapSettled(siteIds, SITE_CONCURRENCY, (id) => apiGet<SosAlert[]>('/sos', { site_id: id, unack_only: true }))
  const ok = results.filter((r): r is PromiseFulfilledResult<SosAlert[]> => r.status === 'fulfilled')
  if (ok.length === 0) throw (results[0] as PromiseRejectedResult).reason
  const seen = new Set<string>()
  return ok
    .flatMap((r) => r.value ?? [])
    .filter((a) => !a.acknowledged_at && !seen.has(a.id) && (seen.add(a.id), true))
    .sort(newestFirst)
}

/** Polls active SOS alerts of all visible sites (admin / supervisor). */
export function useActiveSos() {
  const { can } = useAuth()
  const { sites } = useSite()
  const siteIds = sites.map((s) => s.id).sort()
  const enabled = can('viewSos') && sites.length > 0
  return useQuery({
    queryKey: ['sos', 'active', siteIds.join(',')],
    queryFn: () => fetchActiveSos(siteIds),
    enabled,
    refetchInterval: SOS_POLL_MS,
    refetchIntervalInBackground: true,
    staleTime: 0,
  })
}
