import { useCallback, useEffect, useState } from 'react'
import { App } from '../App'
import { initKeycloak, rememberRealm, resolveRealm } from '../keycloak'
import { config } from '../config'
import { useT } from '../i18n/context'
import { FullScreenMessage, RealmPicker } from './RealmPicker'

type Phase = { kind: 'pick' } | { kind: 'auth'; realm: string } | { kind: 'ready' } | { kind: 'error'; message: string }

/**
 * Chooses the realm (D-005), initialises Keycloak, then renders the app.
 */
export function Bootstrap() {
  const t = useT()
  const [phase, setPhase] = useState<Phase>(() => {
    const realm = resolveRealm()
    return realm ? { kind: 'auth', realm } : { kind: 'pick' }
  })

  const authRealm = phase.kind === 'auth' ? phase.realm : null

  useEffect(() => {
    if (!authRealm) return
    let cancelled = false
    initKeycloak(authRealm)
      .then((ok) => {
        if (!cancelled && ok) setPhase({ kind: 'ready' })
      })
      .catch((err: unknown) => {
        if (!cancelled) setPhase({ kind: 'error', message: err instanceof Error ? err.message : String(err ?? '') })
      })
    return () => {
      cancelled = true
    }
  }, [authRealm])

  const choose = useCallback((realm: string) => {
    rememberRealm(realm)
    setPhase({ kind: 'auth', realm })
  }, [])

  switch (phase.kind) {
    case 'pick':
      return <RealmPicker onChoose={choose} />
    case 'auth':
      return (
        <div role="status" aria-live="polite" className="flex min-h-full items-center justify-center text-sm text-gray-700">
          {t('app.loading')}
        </div>
      )
    case 'error': {
      const actions = [{ label: t('app.retry'), onClick: () => window.location.reload() }]
      if (config.realms.length > 1) {
        actions.push({
          label: t('org.switch'),
          onClick: () => {
            rememberRealm(null)
            setPhase({ kind: 'pick' })
          },
        })
      }
      return <FullScreenMessage title={t('app.authError')} body={phase.message || t('app.authErrorBody')} actions={actions} />
    }
    case 'ready':
      return <App />
  }
}
