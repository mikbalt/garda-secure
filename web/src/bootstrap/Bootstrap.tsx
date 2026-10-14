import { useCallback, useEffect, useState } from 'react'
import { App } from '../App'
import apiClient from '../api/client'
import { initKeycloak, login, rememberRealm, resolveRealm } from '../keycloak'
import { config } from '../config'
import { useT } from '../i18n/context'
import { clearLocalSession, keycloakLogoutUrl, startIntent } from '../lib/session'
import { dropPushForAccountSwitch } from '../lib/push'
import { readStored, STORAGE_KEYS } from '../lib/storage'
import { FullScreenMessage, LoadingScreen, RealmPicker } from './RealmPicker'

type Phase =
  | { kind: 'pick' }
  | { kind: 'auth'; realm: string }
  | { kind: 'stuck'; realm: string }
  | { kind: 'resetting' }
  | { kind: 'ready' }
  | { kind: 'error'; message: string }

/** Keycloak normally redirects within a second or two; after this the user gets a way out. */
const STUCK_AFTER_MS = 20_000

/**
 * Clears this browser's sign-in state (app + keycloak-js) and signs out of the
 * Keycloak session for the realm, then returns to the start page.
 */
function resetSignIn(realm: string | null): void {
  clearLocalSession()
  if (config.demo || !realm || !config.keycloakUrl) {
    window.location.replace('/')
    return
  }
  window.location.replace(keycloakLogoutUrl(config.keycloakUrl, realm, config.keycloakClientId, window.location.origin))
}

/** Drops `?reset` / `?org` from the address bar so a reload does not repeat it. */
function stripIntent(): void {
  try {
    const url = new URL(window.location.href)
    url.searchParams.delete('reset')
    url.searchParams.delete('org')
    window.history.replaceState(window.history.state, '', url.toString())
  } catch {
    /* ignore */
  }
}

function initialPhase(): Phase {
  const intent = startIntent(window.location.search)
  if (intent === 'reset') return { kind: 'resetting' }
  if (intent === 'org') {
    stripIntent()
    rememberRealm(null)
    // Choosing another organisation: stop this browser receiving the previous account's push.
    void dropPushForAccountSwitch({ server: false })
    return config.realms.length === 1 && !config.demo ? { kind: 'auth', realm: config.realms[0].id } : { kind: 'pick' }
  }
  const realm = resolveRealm()
  return realm ? { kind: 'auth', realm } : { kind: 'pick' }
}

/**
 * Chooses the realm (D-005), initialises Keycloak, then renders the app.
 * Recovery: `/?reset` (or the "Reset sign-in" action) clears a stuck sign-in,
 * `/?org` re-opens the organisation picker.
 */
export function Bootstrap() {
  const t = useT()
  const [phase, setPhase] = useState<Phase>(initialPhase)

  const authRealm = phase.kind === 'auth' ? phase.realm : null

  useEffect(() => {
    if (phase.kind !== 'resetting') return
    // Read before clearing: the realm decides which Keycloak session to end.
    const realm = readStored(STORAGE_KEYS.realm) ?? config.realms[0]?.id ?? null
    stripIntent()
    resetSignIn(realm)
  }, [phase.kind])

  useEffect(() => {
    if (!authRealm) return
    let cancelled = false
    const stuck = window.setTimeout(() => {
      if (!cancelled) setPhase({ kind: 'stuck', realm: authRealm })
    }, STUCK_AFTER_MS)
    initKeycloak(authRealm)
      .then(async (ok) => {
        if (config.demo) {
          // Demo mode answers the API from sample data in the browser.
          const { installDemoApi } = await import('../demo/mockApi')
          installDemoApi(apiClient)
        }
        if (cancelled) return
        window.clearTimeout(stuck)
        if (ok) setPhase({ kind: 'ready' })
        else login() // not signed in after the callback: start a fresh login instead of waiting
      })
      .catch((err: unknown) => {
        window.clearTimeout(stuck)
        if (!cancelled) setPhase({ kind: 'error', message: err instanceof Error ? err.message : String(err ?? '') })
      })
    return () => {
      cancelled = true
      window.clearTimeout(stuck)
    }
  }, [authRealm])

  const choose = useCallback((realm: string) => {
    rememberRealm(realm)
    setPhase({ kind: 'auth', realm })
  }, [])

  const recovery = (realm: string | null) => {
    const actions = [
      { label: t('app.retry'), onClick: () => window.location.reload() },
      { label: t('app.reset'), onClick: () => resetSignIn(realm) },
    ]
    if (config.realms.length > 1) {
      actions.push({
        label: t('org.switch'),
        onClick: () => {
          rememberRealm(null)
          void dropPushForAccountSwitch({ server: false })
          setPhase({ kind: 'pick' })
        },
      })
    }
    return actions
  }

  switch (phase.kind) {
    case 'pick':
      return <RealmPicker onChoose={choose} />
    case 'auth':
      return <LoadingScreen label={t('app.loading')} />
    case 'resetting':
      return <LoadingScreen label={t('app.resetting')} />
    case 'stuck':
      return <FullScreenMessage title={t('app.stuckTitle')} body={t('app.stuckBody')} actions={recovery(phase.realm)} />
    case 'error':
      return (
        <FullScreenMessage
          title={t('app.authError')}
          body={`${phase.message || t('app.authErrorBody')} — ${t('app.stuckBody')}`}
          actions={recovery(readStored(STORAGE_KEYS.realm) ?? config.realms[0]?.id ?? null)}
        />
      )
    case 'ready':
      return <App />
  }
}
