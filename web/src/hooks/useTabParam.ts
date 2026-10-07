import { useCallback } from 'react'
import { useSearchParams } from 'react-router-dom'

/** Active tab kept in the `?tab=` query parameter (deep-linkable). */
export function useTabParam<K extends string>(allowed: readonly K[], fallback: K): [K, (k: K) => void] {
  const [params, setParams] = useSearchParams()
  const raw = params.get('tab') as K | null
  const active = raw && allowed.includes(raw) ? raw : fallback
  const set = useCallback(
    (k: K) => {
      setParams(
        (prev) => {
          const next = new URLSearchParams(prev)
          next.set('tab', k)
          next.delete('new')
          return next
        },
        { replace: true }
      )
    },
    [setParams]
  )
  return [active, set]
}
