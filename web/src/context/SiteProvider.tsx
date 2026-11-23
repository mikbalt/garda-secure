import { useCallback, useMemo, useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { SiteContext } from './site'
import { apiGet } from '../api/client'
import { useAuth } from '../hooks/useAuth'
import { readStored, STORAGE_KEYS, writeStored } from '../lib/storage'
import type { Site } from '../types'

const SITES_RETRY_MS = 15_000

export function SiteProvider({ children }: { children: ReactNode }) {
  const { can } = useAuth()
  const enabled = can('viewSites')
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['sites'],
    queryFn: () => apiGet<Site[]>('/sites'),
    // A platform-admin-only user has no sites and the API answers 403 (TAQ-41).
    enabled,
    // Every site page hangs on this list: after a failed load (API restart,
    // network blip) keep trying instead of staying broken until a reload.
    refetchInterval: (query) => (query.state.status === 'error' ? SITES_RETRY_MS : false),
  })
  const [chosen, setChosen] = useState<string>(() => readStored(STORAGE_KEYS.site) ?? '')

  const setSiteId = useCallback((id: string) => {
    setChosen(id)
    writeStored(STORAGE_KEYS.site, id)
  }, [])

  const value = useMemo(() => {
    const sites = data ?? []
    const site = sites.find((s) => s.id === chosen) ?? sites[0] ?? null
    return { sites, site, siteId: site?.id ?? '', setSiteId, isLoading, error, retry: () => void refetch() }
  }, [data, chosen, setSiteId, isLoading, error, refetch])

  return <SiteContext.Provider value={value}>{children}</SiteContext.Provider>
}
