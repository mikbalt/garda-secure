/**
 * Client-side rules of the user-management page — mirror the API's checks
 * (docs/api.md "Users and tablet accounts", D-041) so the UI never offers an
 * action the server refuses. The server stays the authority.
 */
import type { Key } from '../../i18n/context'
import type { AssignableRole, ManagedUser, UserUpdateInput } from '../../types'

/** Roles every realm admin may assign, most privileged first. */
export const REALM_ROLES: readonly AssignableRole[] = ['admin', 'supervisor', 'hr', 'client-readonly']

/** Roles that must enrol an authenticator app (OTP) at first login. */
export const STAFF_ROLES: readonly string[] = ['admin', 'supervisor', 'hr', 'platform-admin']

/** Roles the caller may assign: `platform-admin` only for a platform admin. */
export function assignableRoles(callerRoles: readonly string[]): AssignableRole[] {
  return callerRoles.includes('platform-admin') ? [...REALM_ROLES, 'platform-admin'] : [...REALM_ROLES]
}

/** The caller may not edit a platform admin unless being one (403 role_not_allowed). */
export function isManageable(user: Pick<ManagedUser, 'roles'>, callerRoles: readonly string[]): boolean {
  return !user.roles.includes('platform-admin') || callerRoles.includes('platform-admin')
}

const USERNAME_RE = /^[a-z0-9][a-z0-9._@-]{2,63}$/

/** API code for an invalid username, or null (3–64 chars a-z 0-9 . _ @ -, reserved prefixes). */
export function usernameError(raw: string): 'invalid_username' | null {
  const u = raw.trim().toLowerCase()
  if (!USERNAME_RE.test(u) || u.startsWith('device-') || u.startsWith('service-account-')) return 'invalid_username'
  return null
}

/** Normalised username as the API stores it. */
export const normaliseUsername = (raw: string) => raw.trim().toLowerCase()

const EMAIL_RE = /^[^\s@<>()[\],;:"]+@[^\s@<>()[\],;:"]+\.[^\s@<>()[\],;:"]+$/

/** API code for an e-mail problem, or null. An invitation needs an address. */
export function emailError(raw: string, required: boolean): 'email_required' | 'invalid_email' | null {
  const e = raw.trim()
  if (!e) return required ? 'email_required' : null
  return EMAIL_RE.test(e) && e.length <= 254 ? null : 'invalid_email'
}

/** A site-scoped admin may only create/limit users to (some of) their sites. */
export function sitesError(sites: readonly string[], callerRestricted: boolean): 'sites_required' | null {
  return callerRestricted && sites.length === 0 ? 'sites_required' : null
}

/** Roles the user may not take away from themselves (403 cannot_modify_self). */
export function lockedOwnRoles(user: Pick<ManagedUser, 'roles'>, isSelf: boolean): string[] {
  if (!isSelf) return []
  return ['admin', 'platform-admin'].filter((r) => r === 'admin' || user.roles.includes(r))
}

/**
 * Why an update would be refused for the signed-in user themselves
 * (removing their own admin / platform-admin role, disabling or re-scoping
 * themselves), else null.
 */
export function selfUpdateError(before: Pick<ManagedUser, 'roles' | 'sites' | 'enabled'>, patch: UserUpdateInput, isSelf: boolean): 'cannot_modify_self' | null {
  if (!isSelf) return null
  if (patch.enabled === false) return 'cannot_modify_self'
  if (patch.roles) {
    if (!patch.roles.includes('admin')) return 'cannot_modify_self'
    if (before.roles.includes('platform-admin') && !patch.roles.includes('platform-admin')) return 'cannot_modify_self'
  }
  if (patch.sites && !sameSet(patch.sites, before.sites)) return 'cannot_modify_self'
  return null
}

export function sameSet(a: readonly string[], b: readonly string[]): boolean {
  return a.length === b.length && a.every((x) => b.includes(x))
}

/** Changed fields only (omitted = unchanged on PUT /users/:id). */
export function diffUser(before: ManagedUser, next: Required<UserUpdateInput>): UserUpdateInput {
  const out: UserUpdateInput = {}
  if (next.first_name.trim() !== before.first_name) out.first_name = next.first_name.trim()
  if (next.last_name.trim() !== before.last_name) out.last_name = next.last_name.trim()
  if (next.email.trim().toLowerCase() !== (before.email ?? '').toLowerCase()) out.email = next.email.trim()
  if (next.enabled !== before.enabled) out.enabled = next.enabled
  if (!sameSet(next.roles, before.roles)) out.roles = [...next.roles]
  if (!sameSet(next.sites, before.sites)) out.sites = [...next.sites]
  return out
}

/** Display name: "First Last", else the username. */
export function displayName(u: Pick<ManagedUser, 'first_name' | 'last_name' | 'username'>): string {
  return [u.first_name, u.last_name].filter((s) => s && s.trim()).join(' ') || u.username
}

/** Pending sign-in steps worth showing (password change, OTP enrolment, …). */
export function pendingActions(u: Pick<ManagedUser, 'required_actions'>): string[] {
  return (u.required_actions ?? []).filter(Boolean)
}

/** Whether the user is expected to have OTP (staff role) but has not enrolled yet. */
export function otpMissing(u: Pick<ManagedUser, 'roles' | 'otp_configured'>): boolean {
  return !u.otp_configured && u.roles.some((r) => STAFF_ROLES.includes(r))
}

/** Most privileged role first, for consistent badges. */
export function sortRoles(roles: readonly string[]): string[] {
  const order = ['platform-admin', ...REALM_ROLES]
  return [...roles].sort((a, b) => (order.indexOf(a) + 1 || 99) - (order.indexOf(b) + 1 || 99))
}

/** Translated Keycloak required action ("Must change password", …). */
export function requiredActionLabel(action: string, t: (k: Key) => string): string {
  const known: Record<string, Key> = {
    UPDATE_PASSWORD: 'acc.ra.UPDATE_PASSWORD',
    CONFIGURE_TOTP: 'acc.ra.CONFIGURE_TOTP',
    VERIFY_EMAIL: 'acc.ra.VERIFY_EMAIL',
    UPDATE_PROFILE: 'acc.ra.UPDATE_PROFILE',
  }
  return known[action] ? t(known[action]) : action
}
