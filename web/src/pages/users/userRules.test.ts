import { describe, expect, it } from 'vitest'
import type { ManagedUser } from '../../types'
import {
  assignableRoles,
  diffUser,
  displayName,
  emailError,
  isManageable,
  lockedOwnRoles,
  otpMissing,
  requiredActionLabel,
  selfUpdateError,
  sitesError,
  sortRoles,
  usernameError,
} from './userRules'

const user = (over: Partial<ManagedUser> = {}): ManagedUser => ({
  id: 'u1',
  username: 'rina',
  email: 'rina@example.com',
  first_name: 'Rina',
  last_name: 'Kusuma',
  enabled: true,
  roles: ['admin'],
  sites: [],
  otp_configured: true,
  required_actions: [],
  created_at: '2026-10-01T00:00:00Z',
  ...over,
})

describe('role allow-list', () => {
  it('offers platform-admin only to a platform admin', () => {
    expect(assignableRoles(['admin'])).toEqual(['admin', 'supervisor', 'hr', 'client-readonly'])
    expect(assignableRoles(['admin', 'platform-admin'])).toContain('platform-admin')
  })

  it('keeps platform admins out of reach of realm admins', () => {
    expect(isManageable(user({ roles: ['admin', 'platform-admin'] }), ['admin'])).toBe(false)
    expect(isManageable(user({ roles: ['admin', 'platform-admin'] }), ['admin', 'platform-admin'])).toBe(true)
    expect(isManageable(user({ roles: ['hr'] }), ['admin'])).toBe(true)
  })

  it('sorts roles most privileged first', () => {
    expect(sortRoles(['client-readonly', 'admin', 'weird', 'platform-admin'])).toEqual(['platform-admin', 'admin', 'client-readonly', 'weird'])
  })
})

describe('input checks mirror the API', () => {
  it('validates usernames like the API', () => {
    expect(usernameError('rina.k')).toBeNull()
    expect(usernameError('  Rina@Corp ')).toBeNull() // normalised to lower case
    expect(usernameError('ab')).toBe('invalid_username')
    expect(usernameError('device-gate')).toBe('invalid_username')
    expect(usernameError('service-account-x')).toBe('invalid_username')
    expect(usernameError('rina kusuma')).toBe('invalid_username')
    expect(usernameError('-rina')).toBe('invalid_username')
  })

  it('requires an e-mail address for invitations only', () => {
    expect(emailError('', true)).toBe('email_required')
    expect(emailError('', false)).toBeNull()
    expect(emailError('not-an-email', false)).toBe('invalid_email')
    expect(emailError('a@b.co', true)).toBeNull()
  })

  it('requires sites from a site-scoped admin (sites_required)', () => {
    expect(sitesError([], true)).toBe('sites_required')
    expect(sitesError(['s1'], true)).toBeNull()
    expect(sitesError([], false)).toBeNull()
  })
})

describe('self-protection (403 cannot_modify_self)', () => {
  const me = user({ roles: ['admin', 'platform-admin'], sites: ['s1'] })

  it('locks the own admin roles', () => {
    expect(lockedOwnRoles(me, true)).toEqual(['admin', 'platform-admin'])
    expect(lockedOwnRoles(user(), true)).toEqual(['admin'])
    expect(lockedOwnRoles(me, false)).toEqual([])
  })

  it('refuses disabling, removing admin roles or re-scoping yourself', () => {
    expect(selfUpdateError(me, { enabled: false }, true)).toBe('cannot_modify_self')
    expect(selfUpdateError(me, { roles: ['platform-admin'] }, true)).toBe('cannot_modify_self')
    expect(selfUpdateError(me, { roles: ['admin'] }, true)).toBe('cannot_modify_self')
    expect(selfUpdateError(me, { sites: [] }, true)).toBe('cannot_modify_self')
    expect(selfUpdateError(me, { first_name: 'X', roles: ['admin', 'platform-admin', 'hr'], sites: ['s1'] }, true)).toBeNull()
    expect(selfUpdateError(me, { enabled: false }, false)).toBeNull()
  })
})

describe('diffUser', () => {
  it('sends only changed fields', () => {
    const before = user({ roles: ['supervisor'], sites: ['a', 'b'] })
    expect(diffUser(before, { first_name: 'Rina', last_name: 'Kusuma', email: 'RINA@example.com', enabled: true, roles: ['supervisor'], sites: ['b', 'a'] })).toEqual({})
    expect(diffUser(before, { first_name: ' Rini ', last_name: 'Kusuma', email: 'rina@example.com', enabled: false, roles: ['hr'], sites: [] })).toEqual({
      first_name: 'Rini',
      enabled: false,
      roles: ['hr'],
      sites: [],
    })
  })
})

describe('display helpers', () => {
  it('falls back to the username', () => {
    expect(displayName(user())).toBe('Rina Kusuma')
    expect(displayName(user({ first_name: '', last_name: '' }))).toBe('rina')
  })

  it('flags staff without an authenticator', () => {
    expect(otpMissing(user({ otp_configured: false, roles: ['hr'] }))).toBe(true)
    expect(otpMissing(user({ otp_configured: false, roles: ['client-readonly'] }))).toBe(false)
  })

  it('translates known required actions and keeps unknown ones', () => {
    const t = (k: string) => `T:${k}`
    expect(requiredActionLabel('CONFIGURE_TOTP', t)).toBe('T:acc.ra.CONFIGURE_TOTP')
    expect(requiredActionLabel('TERMS_AND_CONDITIONS', t)).toBe('TERMS_AND_CONDITIONS')
  })
})
