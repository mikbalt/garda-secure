import { useQuery } from '@tanstack/react-query'
import { apiGet } from '../api/client'
import type { Me } from '../types'

/**
 * The signed-in principal as the API sees it (`GET /me`): Keycloak subject
 * (= user id in `/users`), roles and the site scoping (D-042). Cached for the
 * session — it only changes with a new token.
 */
export function useMe() {
  return useQuery({
    queryKey: ['me'],
    queryFn: () => apiGet<Me>('/me'),
    staleTime: 10 * 60_000,
    gcTime: 60 * 60_000,
  })
}
