import { describe, expect, it } from 'vitest'
import { clearLocalSession, keycloakLogoutUrl, startIntent } from './session'

class MemStorage implements Storage {
  private m = new Map<string, string>()
  get length() {
    return this.m.size
  }
  clear() {
    this.m.clear()
  }
  getItem(k: string) {
    return this.m.get(k) ?? null
  }
  key(i: number) {
    return Array.from(this.m.keys())[i] ?? null
  }
  removeItem(k: string) {
    this.m.delete(k)
  }
  setItem(k: string, v: string) {
    this.m.set(k, v)
  }
}

describe('sign-in recovery', () => {
  it('clears app and keycloak-js session state but keeps preferences', () => {
    const local = new MemStorage()
    for (const k of ['sp.realm', 'sp.site', 'sp.lastActivity', 'sp.demo.role', 'kc-callback-abc', 'sp.lang', 'sp.sos.muted', 'other.app']) local.setItem(k, 'x')
    const session = new MemStorage()
    session.setItem('anything', '1')
    clearLocalSession(local, session)
    expect(['sp.lang', 'sp.sos.muted', 'other.app'].every((k) => local.getItem(k) === 'x')).toBe(true)
    expect(['sp.realm', 'sp.site', 'sp.lastActivity', 'sp.demo.role', 'kc-callback-abc'].some((k) => local.getItem(k) !== null)).toBe(false)
    expect(session.length).toBe(0)
  })

  it('tolerates missing storage', () => {
    expect(() => clearLocalSession(null, null)).not.toThrow()
  })

  it('builds the Keycloak end-session URL for the realm', () => {
    const url = new URL(keycloakLogoutUrl('https://patrol.example/auth/', 'client one', 'securepatrol-web', 'https://patrol.example'))
    expect(url.pathname).toBe('/auth/realms/client%20one/protocol/openid-connect/logout')
    expect(url.searchParams.get('client_id')).toBe('securepatrol-web')
    expect(url.searchParams.get('post_logout_redirect_uri')).toBe('https://patrol.example/')
  })

  it('reads the start intent from the query string', () => {
    expect(startIntent('?reset')).toBe('reset')
    expect(startIntent('?reset=1&org')).toBe('reset')
    expect(startIntent('?org')).toBe('org')
    expect(startIntent('?tab=x')).toBeNull()
    expect(startIntent('')).toBeNull()
  })
})
