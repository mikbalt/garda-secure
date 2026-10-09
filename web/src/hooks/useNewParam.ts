import { useCallback, useEffect, useRef } from 'react'
import { useSearchParams } from 'react-router-dom'

/**
 * Deep link `?new=1` → open a "create" form once (used by the setup guide and
 * empty states). Calls `open` when the parameter is present and removes it so
 * a refresh or back navigation does not reopen the form.
 */
export function useNewParam(open: () => void, ready = true) {
  const [params, setParams] = useSearchParams()
  const openRef = useRef(open)
  openRef.current = open
  const wants = params.get('new') === '1'

  const clear = useCallback(() => {
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev)
        next.delete('new')
        return next
      },
      { replace: true }
    )
  }, [setParams])

  useEffect(() => {
    if (wants && ready) {
      openRef.current()
      clear()
    }
  }, [wants, ready, clear])
}
