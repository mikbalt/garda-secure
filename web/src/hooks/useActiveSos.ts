import { useQuery } from '@tanstack/react-query'
import { apiGet } from '../api/client'
import { useSite } from '../context/site'
import { useAuth } from './useAuth'
import type { SosAlert } from '../types'

/** Poll interval of the SOS banner. */
export const SOS_POLL_MS = 10_000

function rows(data: unknown): SosAlert[] {
  if (Array.isArray(data)) return data as SosAlert[]
  const r = (data as { rows?: unknown } | null)?.rows
  return Array.isArray(r) ? (r as SosAlert[]) : []
}

const newestFirst = (a: SosAlert, b: SosAlert) => b.fired_at.localeCompare(a.fired_at)

/**
 * Unacknowledged SOS alerts of every site the user can see, newest first —
 * independent of the selected site (`GET /sos/active`, D-040: realm-wide,
 * narrowed by site scoping, ≤ 100 rows).
 */
export async function fetchActiveSos(): Promise<SosAlert[]> {
  const data = await apiGet<unknown>('/sos/active')
  return rows(data)
    .filter((a) => !a.acknowledged_at)
    .sort(newestFirst)
}

/** Polls active SOS alerts of all visible sites (admin / supervisor). */
export function useActiveSos() {
  const { can } = useAuth()
  const { sites } = useSite()
  const enabled = can('viewSos') && sites.length > 0
  return useQuery({
    queryKey: ['sos', 'active'],
    queryFn: fetchActiveSos,
    enabled,
    refetchInterval: SOS_POLL_MS,
    refetchIntervalInBackground: true,
    staleTime: 0,
  })
}
