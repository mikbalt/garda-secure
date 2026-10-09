/**
 * Runtime configuration. Production reads `window.__SP_CONFIG__` written into
 * /config.js by the web container at start-up; development falls back to the
 * VITE_* variables (see .env.example).
 */

export interface RealmOption {
  id: string
  label: string
}

export interface AppConfig {
  apiUrl: string
  keycloakUrl: string
  keycloakClientId: string
  realms: RealmOption[]
  /** Demo mode: no backend or Keycloak; sample data in the browser. */
  demo: boolean
}

declare global {
  interface Window {
    __SP_CONFIG__?: Partial<AppConfig>
  }
}

/** Parses "realm:Label,realm2:Label 2" into realm options. */
export function parseRealms(raw: string | undefined): RealmOption[] {
  if (!raw) return []
  return raw
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
    .map((item) => {
      const idx = item.indexOf(':')
      const id = (idx >= 0 ? item.slice(0, idx) : item).trim()
      const label = (idx >= 0 ? item.slice(idx + 1) : '').trim() || id
      return { id, label }
    })
    .filter((r) => r.id !== '')
}

function nonEmpty(v: string | undefined): string | undefined {
  return v && v.trim() !== '' ? v.trim() : undefined
}

function load(): AppConfig {
  const rt = (typeof window !== 'undefined' && window.__SP_CONFIG__) || {}
  const env = import.meta.env
  const envRealms = parseRealms(env.VITE_KEYCLOAK_REALMS as string | undefined)
  const legacyRealm = nonEmpty(env.VITE_KEYCLOAK_REALM as string | undefined)
  const realms =
    rt.realms && rt.realms.length > 0
      ? rt.realms.filter((r) => r && r.id)
      : envRealms.length > 0
        ? envRealms
        : legacyRealm
          ? [{ id: legacyRealm, label: legacyRealm }]
          : [{ id: 'securepatrol', label: 'SecurePatrol' }]

  return {
    apiUrl: (nonEmpty(rt.apiUrl) ?? nonEmpty(env.VITE_API_URL as string | undefined) ?? '/api/v1').replace(/\/+$/, ''),
    keycloakUrl: nonEmpty(rt.keycloakUrl) ?? nonEmpty(env.VITE_KEYCLOAK_URL as string | undefined) ?? '/auth',
    keycloakClientId:
      nonEmpty(rt.keycloakClientId) ?? nonEmpty(env.VITE_KEYCLOAK_CLIENT_ID as string | undefined) ?? 'securepatrol-web',
    realms,
    demo: rt.demo === true || String(rt.demo) === 'true' || env.VITE_DEMO === 'true',
  }
}

export const config: AppConfig = load()
