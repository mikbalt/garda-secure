import { useCallback, useMemo, useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { SiteContext } from './site'
import { apiGet } from '../api/client'
import { readStored, STORAGE_KEYS, writeStored } from '../lib/storage'
import type { Site } from '../types'

export function SiteProvider({ children }: { children: ReactNode }) {
  const { data, isLoading, error } = useQuery({
    queryKey: ['sites'],
    queryFn: () => apiGet<Site[]>('/sites'),
  })
  const [chosen, setChosen] = useState<string>(() => readStored(STORAGE_KEYS.site) ?? '')

  const setSiteId = useCallback((id: string) => {
    setChosen(id)
    writeStored(STORAGE_KEYS.site, id)
  }, [])

  const value = useMemo(() => {
    const sites = data ?? []
    const site = sites.find((s) => s.id === chosen) ?? sites[0] ?? null
    return { sites, site, siteId: site?.id ?? '', setSiteId, isLoading, error }
  }, [data, chosen, setSiteId, isLoading, error])

  return <SiteContext.Provider value={value}>{children}</SiteContext.Provider>
}
