/**
 * Recovery for a browser stuck between the app and Keycloak (stale chosen
 * organisation, half-finished login, old session cookie). Used by the
 * "Reset sign-in" action and the `/?reset` URL; `/?org` only re-opens the
 * organisation picker.
 */

/** localStorage keys kept by a reset: per-browser preferences, not session state. */
const KEEP = new Set(['sp.lang', 'sp.sos.muted'])

/** Removes the app's and keycloak-js's session state from this browser. */
export function clearLocalSession(storage: Storage | null = safeLocal(), session: Storage | null = safeSession()): void {
  if (storage) {
    const keys: string[] = []
    for (let i = 0; i < storage.length; i++) {
      const k = storage.key(i)
      if (k && (k.startsWith('kc-callback-') || (k.startsWith('sp.') && !KEEP.has(k)))) keys.push(k)
    }
    for (const k of keys) {
      try {
        storage.removeItem(k)
      } catch {
        /* ignore */
      }
    }
  }
  try {
    session?.clear()
  } catch {
    /* ignore */
  }
}

/**
 * Keycloak end-session URL for the realm. Without an id_token Keycloak shows a
 * "Do you want to log out?" page, then returns to the app's start page.
 */
export function keycloakLogoutUrl(keycloakUrl: string, realm: string, clientId: string, origin: string): string {
  const base = keycloakUrl.replace(/\/+$/, '')
  const q = new URLSearchParams({ client_id: clientId, post_logout_redirect_uri: origin + '/' })
  return `${base}/realms/${encodeURIComponent(realm)}/protocol/openid-connect/logout?${q.toString()}`
}

/** What the start URL asks for: `?reset` (full reset) or `?org` (choose organisation). */
export function startIntent(search: string): 'reset' | 'org' | null {
  const p = new URLSearchParams(search)
  if (p.has('reset')) return 'reset'
  if (p.has('org')) return 'org'
  return null
}

function safeLocal(): Storage | null {
  try {
    return window.localStorage
  } catch {
    return null
  }
}

function safeSession(): Storage | null {
  try {
    return window.sessionStorage
  } catch {
    return null
  }
}
