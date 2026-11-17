/**
 * Lightweight API client for E2E test helpers.
 * Uses the Keycloak admin API to fetch a token, then calls the GARDA REST API.
 */
import { KC_URL, REALM, API_URL } from '../../playwright.real.config'

export interface Site {
  id: string
  name: string
  address: string
  timezone: string
}

export interface Guard {
  id: string
  name: string
  badge_number: string
  site_id: string
}

/** Fetch an admin token via Keycloak Resource Owner Password grant (test use only). */
export async function getAdminToken(): Promise<string> {
  const res = await fetch(`${KC_URL}/realms/${REALM}/protocol/openid-connect/token`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: new URLSearchParams({
      client_id: 'securepatrol-web',
      grant_type: 'password',
      username: 'admin@garda.test',
      password: 'Garda@Admin1',
      scope: 'openid',
    }),
  })
  if (!res.ok) throw new Error(`getAdminToken failed: ${res.status} ${await res.text()}`)
  const data = (await res.json()) as { access_token: string }
  return data.access_token
}

export async function apiGet<T>(path: string, token: string): Promise<T> {
  const res = await fetch(`${API_URL}${path}`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!res.ok) throw new Error(`GET ${path} → ${res.status}`)
  return res.json() as Promise<T>
}

export async function apiPost<T>(path: string, body: unknown, token: string): Promise<T> {
  const res = await fetch(`${API_URL}${path}`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) throw new Error(`POST ${path} → ${res.status}: ${await res.text()}`)
  return res.json() as Promise<T>
}

export async function apiDelete(path: string, token: string): Promise<void> {
  const res = await fetch(`${API_URL}${path}`, {
    method: 'DELETE',
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!res.ok && res.status !== 404) throw new Error(`DELETE ${path} → ${res.status}`)
}

export async function apiPut<T>(path: string, body: unknown, token: string): Promise<T> {
  const res = await fetch(`${API_URL}${path}`, {
    method: 'PUT',
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) throw new Error(`PUT ${path} → ${res.status}: ${await res.text()}`)
  return res.json() as Promise<T>
}

// ── seed helpers ─────────────────────────────────────────────────────────────

/** Prefix for all E2E-created records so cleanup is safe. */
export const E2E = '[E2E]'

export async function seedSite(token: string, overrides: Partial<{ name: string; address: string; timezone: string }> = {}): Promise<Site> {
  return apiPost<Site>('/sites', {
    name: `${E2E} Lokasi Test`,
    address: 'Jl. Test No. 1, Jakarta',
    timezone: 'Asia/Jakarta',
    attendance_rules: { late_minutes: 15, absent_minutes: 60 },
    ...overrides,
  }, token)
}

export async function seedGuard(token: string, siteId: string, overrides: Partial<{ name: string; badge_number: string }> = {}): Promise<Guard> {
  return apiPost<Guard>('/guards', {
    name: `${E2E} Petugas Test`,
    badge_number: `E2E-${Date.now()}`,
    site_id: siteId,
    status: 'active',
    ...overrides,
  }, token)
}

/** Delete all E2E sites (and their cascade children) created by tests. */
export async function cleanupE2ESites(token: string): Promise<void> {
  const sites = await apiGet<Site[]>('/sites', token)
  const e2eSites = sites.filter((s) => s.name.startsWith(E2E))
  for (const site of e2eSites) {
    // Backend does not have DELETE /sites yet — this will be a no-op until
    // issue #34 (site deactivation) is implemented. Guards are cleaned up here.
    const guards = await apiGet<Guard[]>(`/guards?site_id=${site.id}`, token).catch(() => [] as Guard[])
    for (const g of guards) {
      await apiDelete(`/guards/${g.id}`, token).catch(() => {})
    }
  }
}
