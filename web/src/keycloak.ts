import Keycloak from 'keycloak-js'
import { config } from './config'
import { readStored, STORAGE_KEYS, writeStored } from './lib/storage'

/**
 * One Keycloak realm per client company (D-005). The realm is chosen before
 * Keycloak is initialised; with a single configured realm it is implicit.
 */

let keycloak: Keycloak | null = null
let currentRealm = ''

/** Realm to use without asking, or null when the user must choose. */
export function resolveRealm(): string | null {
  if (config.demo) return 'demo'
  const stored = readStored(STORAGE_KEYS.realm)
  if (stored && config.realms.some((r) => r.id === stored)) return stored
  if (config.realms.length === 1) return config.realms[0].id
  return null
}

export function rememberRealm(realm: string | null): void {
  writeStored(STORAGE_KEYS.realm, realm)
}

let initPromise: { realm: string; promise: Promise<boolean> } | null = null

/**
 * Initialises Keycloak for the realm (login-required). Repeated calls for the
 * same realm share one initialisation (React StrictMode runs effects twice).
 */
export function initKeycloak(realm: string): Promise<boolean> {
  if (config.demo) {
    currentRealm = realm
    return Promise.resolve(true)
  }
  if (initPromise && initPromise.realm === realm) return initPromise.promise
  currentRealm = realm
  const instance = new Keycloak({ url: config.keycloakUrl, realm, clientId: config.keycloakClientId })
  keycloak = instance
  // CSP (WEB-2): no session-check iframe and no silent check-sso, so keycloak-js
  // creates no iframes at all, not even its third-party-cookie check (frame-src
  // 'self' would allow a same-origin one). The config object (url + realm)
  // means no discovery fetch; token calls go to KEYCLOAK_URL, which must be
  // same-origin (connect-src 'self'). keycloak-js 26 computes PKCE with Web
  // Crypto, so it needs a secure context (HTTPS or localhost).
  const promise = instance
    .init({ onLoad: 'login-required', checkLoginIframe: false, pkceMethod: 'S256' })
    .then((authenticated) => {
      instance.onTokenExpired = () => {
        instance
          .updateToken(30)
          .catch(() => instance.login())
          .catch(redirectFailed)
      }
      return authenticated
    })
    .catch((err: unknown) => {
      initPromise = null
      throw err instanceof Error ? err : new Error(String((err as { error?: string })?.error ?? 'Keycloak init failed'))
    })
  initPromise = { realm, promise }
  return promise
}

function kc(): Keycloak | null {
  return keycloak
}

/** login()/logout() only redirect; since keycloak-js 26 they are async. */
function redirectFailed(err: unknown): void {
  console.error('Keycloak redirect failed', err)
}

/** Returns a fresh access token, refreshing it when it expires within 30 s. */
export async function freshToken(): Promise<string | undefined> {
  if (config.demo) return undefined
  const k = kc()
  if (!k) return undefined
  try {
    await k.updateToken(30)
  } catch {
    // Refresh token expired — the 401 handler sends the user to login.
  }
  return k.token
}

export function login(): void {
  kc()?.login().catch(redirectFailed)
}

/** Demo mode: the role chosen in the demo bar (default admin). */
export function demoRole(): string {
  return readStored(STORAGE_KEYS.demoRole) || 'admin'
}

export function getUserRoles(): string[] {
  if (config.demo) return [demoRole()]
  const k = kc()
  if (!k) return []
  const flat = k.tokenParsed?.['roles']
  const fromClaim = Array.isArray(flat) ? flat.filter((r): r is string => typeof r === 'string') : []
  return Array.from(new Set([...fromClaim, ...(k.realmAccess?.roles ?? [])]))
}

export function getUsername(): string {
  if (config.demo) return `demo.${demoRole()}`
  const p = kc()?.tokenParsed
  return ((p?.['preferred_username'] ?? p?.['name'] ?? '') as string) || ''
}

export function getRealm(): string {
  return currentRealm
}

export function logout(): void {
  if (config.demo) return window.location.reload()
  kc()?.logout({ redirectUri: window.location.origin + '/' }).catch(redirectFailed)
}

/** Logs out and forgets the chosen organisation so the picker shows again. */
export function switchOrganisation(): void {
  rememberRealm(null)
  logout()
}
