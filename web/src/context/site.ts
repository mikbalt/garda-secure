import { createContext, useContext } from 'react'
import type { Site } from '../types'

export interface SiteContextValue {
  sites: Site[]
  site: Site | null
  siteId: string
  setSiteId: (id: string) => void
  isLoading: boolean
  error: unknown
}

export const SiteContext = createContext<SiteContextValue>({
  sites: [],
  site: null,
  siteId: '',
  setSiteId: () => undefined,
  isLoading: true,
  error: null,
})

/** The globally selected site (top-bar selector). */
export function useSite(): SiteContextValue {
  return useContext(SiteContext)
}

/** Time zone of the selected site (falls back to Asia/Jakarta). */
export function useSiteTz(): string {
  return useContext(SiteContext).site?.timezone || 'Asia/Jakarta'
}
