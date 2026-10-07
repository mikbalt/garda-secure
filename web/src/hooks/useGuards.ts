import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { apiGet } from '../api/client'
import { useAuth } from './useAuth'
import type { Guard } from '../types'

/**
 * Guards of a site plus an id → name lookup. Only roles that may read guards
 * (admin, supervisor, hr) fetch them; others get an empty list.
 */
export function useGuards(siteId: string) {
  const { can } = useAuth()
  const allowed = can('viewGuards')
  const query = useQuery({
    queryKey: ['guards', siteId],
    queryFn: () => apiGet<Guard[]>('/guards', { site_id: siteId }),
    enabled: allowed && !!siteId,
  })
  const names = useMemo(() => {
    const m = new Map<string, string>()
    for (const g of query.data ?? []) m.set(g.id, g.name)
    return m
  }, [query.data])
  return { ...query, guards: query.data ?? [], guardName: (id: string) => names.get(id) ?? '—' }
}
