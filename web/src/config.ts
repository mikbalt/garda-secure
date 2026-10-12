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
  /** Runtime config asked for demo mode but a real backend is configured (WEB-9). */
  demoRefused: boolean
  /** Sign out after this many minutes without user input (0 = never). */
  idleLogoutMinutes: number
}

/** Shape of /config.js (all optional; values are strings or JSON types). */
export interface RuntimeConfig {
  apiUrl?: string
  keycloakUrl?: string
  keycloakClientId?: string
  realms?: RealmOption[]
  demo?: boolean | string
  /** Allow demo mode even with an absolute API / Keycloak URL configured. */
  demoForce?: boolean | string
  idleLogoutMinutes?: number | string
}

declare global {
  interface Window {
    __SP_CONFIG__?: RuntimeConfig
  }
}

export const DEFAULT_IDLE_LOGOUT_MINUTES = 30

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

const truthy = (v: unknown) => v === true || String(v).toLowerCase() === 'true'
const absolute = (v: string | undefined) => !!v && /^https?:\/\//i.test(v.trim())

/** Minutes until idle logout: a non-negative integer, else the default. */
export function parseIdleMinutes(...candidates: unknown[]): number {
  for (const c of candidates) {
    if (c === undefined || c === null || String(c).trim() === '') continue
    const n = Number(c)
    if (Number.isFinite(n) && n >= 0) return Math.floor(n)
  }
  return DEFAULT_IDLE_LOGOUT_MINUTES
}

/**
 * Demo mode from the runtime config is refused when that config also points
 * at a real server (absolute API or Keycloak URL), unless `demoForce` — a
 * production container must never turn into an unauthenticated demo by a
 * stray DEMO=true. The build-time `VITE_DEMO` (npm run dev:demo) always wins.
 */
export function resolveDemo(rt: RuntimeConfig, envDemo: unknown): { demo: boolean; refused: boolean } {
  if (truthy(envDemo)) return { demo: true, refused: false }
  if (!truthy(rt.demo)) return { demo: false, refused: false }
  if (truthy(rt.demoForce) || !(absolute(rt.apiUrl) || absolute(rt.keycloakUrl))) return { demo: true, refused: false }
  return { demo: false, refused: true }
}

export function loadConfig(rt: RuntimeConfig, env: Record<string, unknown>): AppConfig {
  const str = (k: string) => (typeof env[k] === 'string' ? (env[k] as string) : undefined)
  const envRealms = parseRealms(str('VITE_KEYCLOAK_REALMS'))
  const legacyRealm = nonEmpty(str('VITE_KEYCLOAK_REALM'))
  const realms =
    rt.realms && rt.realms.length > 0
      ? rt.realms.filter((r) => r && r.id)
      : envRealms.length > 0
        ? envRealms
        : legacyRealm
          ? [{ id: legacyRealm, label: legacyRealm }]
          : [{ id: 'securepatrol', label: 'SecurePatrol' }]
  const { demo, refused } = resolveDemo(rt, env.VITE_DEMO)

  return {
    apiUrl: (nonEmpty(rt.apiUrl) ?? nonEmpty(str('VITE_API_URL')) ?? '/api/v1').replace(/\/+$/, ''),
    keycloakUrl: nonEmpty(rt.keycloakUrl) ?? nonEmpty(str('VITE_KEYCLOAK_URL')) ?? '/auth',
    keycloakClientId: nonEmpty(rt.keycloakClientId) ?? nonEmpty(str('VITE_KEYCLOAK_CLIENT_ID')) ?? 'securepatrol-web',
    realms,
    demo,
    demoRefused: refused,
    idleLogoutMinutes: parseIdleMinutes(rt.idleLogoutMinutes, env.VITE_IDLE_LOGOUT_MINUTES),
  }
}

export const config: AppConfig = loadConfig((typeof window !== 'undefined' && window.__SP_CONFIG__) || {}, import.meta.env)

if (config.demoRefused) {
  console.warn('SecurePatrol: DEMO=true ignored — a real API/Keycloak URL is configured (set DEMO_FORCE=true to override).')
}
