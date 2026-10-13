/**
 * Demo mode: people & access endpoints (F1 users + tablet accounts, F4 site
 * scoping in /me, F6 attendance override history seed, D-046 visitor detail
 * audit, D-049 tablet health). Registered by mockApi.ts with its router and
 * world, so the rules match docs/api.md (roles, self-protection, error codes).
 */
import type { AttendanceRecord, AuditRow, Device, DeviceHealth, Site } from '../types'
import { getUserRoles } from '../keycloak'
import { slug } from '../pages/settings/accountUtil'
import { iso, uid, wib, type World } from './world'

type Params = Record<string, string | undefined>
type Body = Record<string, unknown>
type Handler = (p: { params: Params; query: Params; body: Body }) => unknown

export interface MockKit {
  on: (method: string, path: string, fn: Handler) => void
  world: World
  /** Throws an API error `{error, code, field?}` with the status. */
  fail: (status: number, code: string, msg: string, field?: string) => never
  /** A paged array answer with `X-Total-Count`. */
  paged: (rows: unknown[], total: number) => unknown
}

interface DemoUser {
  id: string
  username: string
  email: string
  first_name: string
  last_name: string
  enabled: boolean
  roles: string[]
  sites: string[]
  otp_configured: boolean
  required_actions: string[]
  created_at: string
}
interface DemoDeviceAccount {
  id: string
  username: string
  name: string
  enabled: boolean
  created_at: string
}
type DemoOverride = Record<string, unknown> & { attendance_id: string }
type PeopleWorld = World & {
  users?: DemoUser[]
  device_accounts?: DemoDeviceAccount[]
  attendance_overrides?: DemoOverride[]
  people_seeded?: boolean
}

/** Keycloak subject of the demo admin (`/me`), so self-protection can be tried. */
export const DEMO_SELF_ID = '00000000-0000-4000-8000-00000000a001'
const WEB_ROLES = ['admin', 'supervisor', 'hr', 'client-readonly']
const STAFF = ['admin', 'supervisor', 'hr', 'platform-admin']
const role = () => getUserRoles()[0] ?? 'admin'
const daysAgo = (n: number) => iso(new Date(Date.now() - n * 86_400_000))

/** Demo site scoping (D-042): the client account sees the first location only. */
export function demoSitesClaim(sites: Site[]): string[] {
  return role() === 'client-readonly' ? sites.slice(0, 1).map((s) => s.id) : []
}

/** `GET /me` of the demo principal. */
export function demoMe(sites: Site[]) {
  const claim = demoSitesClaim(sites)
  return {
    subject: role() === 'admin' ? DEMO_SELF_ID : `demo-${role()}`,
    username: `demo.${role()}`,
    realm: 'demo',
    client_id: 'securepatrol-web',
    roles: getUserRoles(),
    sites: claim,
    sites_restricted: claim.length > 0,
  }
}

/** `/sites` narrowed by the demo site claim. */
export function visibleSites(sites: Site[]): Site[] {
  const claim = demoSitesClaim(sites)
  return claim.length ? sites.filter((s) => claim.includes(s.id)) : sites
}

/**
 * Health beacon of a demo tablet: the first active tablet is healthy, the
 * second needs attention (clock 3 min ahead, parked items, an error).
 */
export function demoHealth(d: Device, index: number): Pick<Device, 'health' | 'health_at'> {
  if (d.status !== 'active') return {}
  const now = Date.now()
  const at = iso(new Date(now - (index + 2) * 60_000))
  const base: DeviceHealth = {
    app_version: '1.4.0', app_build: '42', platform: 'android', os_version: '14', outbox_pending: 0, outbox_failed: 0,
    outbox_paused: false, sos_pending: 0, last_sync: iso(new Date(now - 60_000)), roster_synced_at: iso(new Date(now - 40 * 60_000)),
    clock_offset_ms: -800, private_ca: true, pin_failures: 0, errors: [], received_at: at,
  }
  if (index % 2 === 1) {
    Object.assign(base, {
      app_version: '1.3.2', app_build: '37', os_version: '12', outbox_pending: 3, outbox_failed: 2, clock_offset_ms: 192_000, pin_failures: 2,
      errors: ['2026-10-12T02:14:09Z upload photo: SocketException: Connection reset by peer', '2026-10-12T02:15:40Z item parked after 50 attempts (POST /incidents)'],
    })
  }
  return { health: base, health_at: at }
}

export function registerPeopleMocks({ on, world, fail, paged }: MockKit) {
  const w = world as PeopleWorld
  const has = (...roles: string[]) => getUserRoles().some((r) => roles.includes(r))
  const admin = () => {
    if (!has('admin')) fail(403, 'forbidden', 'forbidden')
  }
  const callerRestricted = () => demoSitesClaim(w.sites).length > 0
  const selfId = () => demoMe(w.sites).subject

  // ── seed (lazily, so saved demo worlds get it too) ──────────────────────────
  function users(): DemoUser[] {
    if (w.users) return w.users
    const [a, b] = w.sites
    const mk = (u: Partial<DemoUser> & Pick<DemoUser, 'username' | 'first_name' | 'last_name' | 'roles'>, age: number): DemoUser => ({
      id: uid(), email: '', enabled: true, sites: [], otp_configured: true, required_actions: [], created_at: daysAgo(age), ...u,
    })
    w.users = [
      mk({ id: DEMO_SELF_ID, username: 'demo.admin', first_name: 'Rina', last_name: 'Kusuma', email: 'rina.kusuma@logistiknusantara.co.id', roles: ['admin'] }, 120),
      mk({ username: 'spv.budi', first_name: 'Budi', last_name: 'Santoso', email: 'budi.santoso@garda.co.id', roles: ['supervisor'], sites: a ? [a.id] : [] }, 90),
      mk({ username: 'spv.andi', first_name: 'Andi', last_name: 'Firmansyah', email: 'andi.f@garda.co.id', roles: ['supervisor'], sites: b ? [b.id] : [], enabled: false }, 75),
      mk({ username: 'hr.maya', first_name: 'Maya', last_name: 'Sari', email: 'maya.sari@garda.co.id', roles: ['hr'], otp_configured: false, required_actions: ['CONFIGURE_TOTP'] }, 30),
      mk({ username: 'klien.logistik', first_name: 'Hendra', last_name: 'Wijaya', email: 'hendra.w@logistiknusantara.co.id', roles: ['client-readonly'], sites: a ? [a.id] : [], otp_configured: false }, 60),
      mk({ username: 'ops.operator', first_name: 'Operator', last_name: 'Platform', email: 'noc@operator.example', roles: ['admin', 'platform-admin'] }, 200),
      mk({ username: 'admin.dimas', first_name: 'Dimas', last_name: 'Prakoso', email: 'dimas@garda.co.id', roles: ['admin'], otp_configured: false, required_actions: ['UPDATE_PASSWORD', 'CONFIGURE_TOTP'] }, 1),
    ]
    return w.users
  }

  function accounts(): DemoDeviceAccount[] {
    if (w.device_accounts) return w.device_accounts
    w.device_accounts = [
      ...w.devices.map((d) => ({ id: d.keycloak_user_id, username: `device-${slug(d.name)}`, name: d.name, enabled: d.status !== 'revoked', created_at: d.created_at })),
      { id: uid(), username: 'device-pos-parkir-sudirman', name: 'Pos Parkir Sudirman', enabled: true, created_at: daysAgo(2) },
      { id: uid(), username: 'device-tablet-lama-gudang', name: 'Tablet Lama Gudang', enabled: false, created_at: daysAgo(200) },
    ]
    return w.device_accounts
  }

  /** One corrected attendance yesterday at the first location, with its F6 history. */
  function seedOverrides() {
    if (w.people_seeded) return
    w.people_seeded = true
    const site = w.sites[0]
    if (!site) return
    const yStart = wib(-1, 0).getTime()
    const yEnd = wib(0, 0).getTime()
    const candidates = w.attendance
      .filter((a) => a.site_id === site.id && a.clock_in_at && a.clock_out_at && !a.needs_review && !a.override_reason)
      .filter((a) => {
        const shift = w.shifts.find((s) => s.id === a.shift_id)
        const t = shift ? Date.parse(shift.start_time) : NaN
        return t >= yStart && t < yEnd && new Date(t).getUTCHours() === 0 // 07:00 WIB morning shifts
      })
    const rec: AttendanceRecord | undefined = candidates[0]
    if (!rec) return
    const shift = w.shifts.find((s) => s.id === rec.shift_id)!
    const start = Date.parse(shift.start_time)
    const lateIn = iso(new Date(start + 22 * 60_000))
    const firstOut = rec.clock_out_at!
    const fixedOut = iso(new Date(Date.parse(shift.end_time) + 5 * 60_000))
    const r1 = 'Terlambat karena mengantar korban kecelakaan ke klinik — disetujui supervisor'
    const r2 = 'Absen pulang tercatat terlalu cepat, tablet mati; dikonfirmasi lewat buku mutasi'
    const list = (w.attendance_overrides ??= [])
    list.push(
      {
        id: uid(), attendance_id: rec.id, shift_id: shift.id, site_id: site.id,
        before_status: 'late', before_clock_in_at: lateIn, before_clock_out_at: firstOut, before_reason: null,
        after_status: 'present', after_clock_in_at: lateIn, after_clock_out_at: firstOut,
        reason: r1, actor: 'admin.rina', created_at: iso(new Date(Date.parse(shift.end_time) + 70 * 60_000)),
      },
      {
        id: uid(), attendance_id: rec.id, shift_id: shift.id, site_id: site.id,
        before_status: 'present', before_clock_in_at: lateIn, before_clock_out_at: firstOut, before_reason: r1,
        after_status: 'present', after_clock_in_at: lateIn, after_clock_out_at: fixedOut,
        reason: r2, actor: 'admin.rina', created_at: iso(new Date(Date.parse(shift.end_time) + 95 * 60_000)),
      },
    )
    Object.assign(rec, {
      status: 'present', late_minutes: 0, clock_in_at: lateIn, clock_out_at: fixedOut, override_reason: r2,
      overridden_by: 'admin.rina', overridden_at: list[list.length - 1].created_at,
    })
  }
  seedOverrides()

  // ── users ───────────────────────────────────────────────────────────────────
  const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
  const checkEmail = (raw: unknown, required: boolean): string => {
    const e = String(raw ?? '').trim()
    if (!e) {
      if (required) fail(400, 'email_required', 'an e-mail address is required to send an invitation', 'email')
      return ''
    }
    if (!EMAIL_RE.test(e)) fail(400, 'invalid_email', 'invalid e-mail address', 'email')
    return e.toLowerCase()
  }
  const checkRoles = (raw: unknown): string[] => {
    const out: string[] = []
    for (const r of (raw as string[] | undefined) ?? []) {
      if (r === 'platform-admin' && !has('platform-admin')) fail(403, 'role_not_allowed', 'only a platform admin can grant or revoke platform-admin', 'roles')
      if (r !== 'platform-admin' && !WEB_ROLES.includes(r)) fail(403, 'role_not_allowed', `role "${r}" cannot be assigned here`, 'roles')
      if (!out.includes(r)) out.push(r)
    }
    return out.sort()
  }
  const checkSites = (raw: unknown): string[] => {
    const claim = demoSitesClaim(w.sites)
    const out: string[] = []
    for (const id of (raw as string[] | undefined) ?? []) {
      if (!w.sites.some((s) => s.id === id) || (claim.length && !claim.includes(id))) fail(400, 'invalid_site', `site ${id} is not a site of your organisation`, 'sites')
      if (!out.includes(id)) out.push(id)
    }
    if (callerRestricted() && out.length === 0) fail(403, 'sites_required', 'you can only manage users limited to your sites', 'sites')
    return out.sort()
  }
  const userFor = (id?: string): DemoUser => {
    const u = users().find((x) => x.id === id)
    if (!u) return fail(404, 'user_not_found', 'user not found')
    if (u.roles.includes('platform-admin') && !has('platform-admin')) fail(403, 'role_not_allowed', 'only a platform admin can manage a platform admin')
    return u
  }
  const password = (n: number) => {
    const alphabet = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789'
    const bytes = crypto.getRandomValues(new Uint8Array(n))
    return Array.from(bytes, (x) => alphabet[x % alphabet.length]).join('')
  }
  const requiredActions = (roles: string[]) => ['UPDATE_PASSWORD', ...(roles.some((r) => STAFF.includes(r)) ? ['CONFIGURE_TOTP'] : [])]

  on('GET', '/users', ({ query }) => {
    admin()
    const first = Number(query.first ?? 0)
    const max = Number(query.max ?? 100)
    if (!Number.isInteger(first) || first < 0) fail(400, 'invalid_first', 'first must be a non-negative number')
    if (!Number.isInteger(max) || max < 1 || max > 500) fail(400, 'invalid_max', 'max must be between 1 and 500')
    const search = (query.search ?? '').trim().toLowerCase()
    const claim = demoSitesClaim(w.sites)
    const rows = users()
      .filter((u) => !claim.length || (u.sites.length > 0 && u.sites.every((s) => claim.includes(s))))
      .filter((u) => !query.role || u.roles.includes(query.role))
      .filter((u) => query.enabled === undefined || String(u.enabled) === query.enabled)
      .filter((u) => !search || `${u.username} ${u.email} ${u.first_name} ${u.last_name}`.toLowerCase().includes(search))
      .sort((x, y) => x.username.localeCompare(y.username))
    return paged(rows.slice(first, first + max), rows.length)
  })
  on('GET', '/users/:id', ({ params }) => (admin(), userFor(params.id)))
  on('POST', '/users', ({ body }) => {
    admin()
    const username = String(body.username ?? '').trim().toLowerCase()
    if (!/^[a-z0-9][a-z0-9._@-]{2,63}$/.test(username) || username.startsWith('device-') || username.startsWith('service-account-')) {
      fail(400, 'invalid_username', 'username: 3-64 characters a-z, 0-9 and . _ @ -', 'username')
    }
    const email = checkEmail(body.email, !!body.send_invite)
    const roles = checkRoles(body.roles)
    if (!roles.length) fail(400, 'roles_required', 'choose at least one role', 'roles')
    const sites = checkSites(body.sites)
    const temp = String(body.temporary_password ?? '')
    if (temp && temp.length < 8) fail(400, 'password_policy', 'the password does not meet the realm password policy (minimum length 8)', 'temporary_password')
    if (users().some((u) => u.username === username)) fail(409, 'username_taken', 'this username is already used', 'username')
    if (email && users().some((u) => u.email === email)) fail(409, 'email_taken', 'this e-mail address is already used', 'email')
    const u: DemoUser = {
      id: uid(), username, email, first_name: String(body.first_name ?? '').trim(), last_name: String(body.last_name ?? '').trim(),
      enabled: true, roles, sites, otp_configured: false, required_actions: requiredActions(roles), created_at: iso(new Date()),
    }
    users().push(u)
    return body.send_invite ? { user: u, invite_sent: true } : { user: u, invite_sent: false, temporary_password: temp || password(16) }
  })
  on('PUT', '/users/:id', ({ params, body }) => {
    admin()
    const u = userFor(params.id)
    const self = u.id === selfId()
    let roles = u.roles
    if (body.roles !== undefined) {
      roles = checkRoles(body.roles)
      if (!roles.length) fail(400, 'roles_required', 'choose at least one role', 'roles')
      if (self && (!roles.includes('admin') || (u.roles.includes('platform-admin') && !roles.includes('platform-admin')))) {
        fail(403, 'cannot_modify_self', 'you cannot remove your own admin roles')
      }
    }
    if (body.enabled === false && self) fail(403, 'cannot_modify_self', 'you cannot disable your own account')
    let sites = u.sites
    if (body.sites !== undefined) {
      sites = checkSites(body.sites)
      if (self && (sites.length !== u.sites.length || sites.some((s) => !u.sites.includes(s)))) fail(403, 'cannot_modify_self', 'you cannot change your own sites')
    }
    let email = u.email
    if (body.email !== undefined) {
      email = checkEmail(body.email, false)
      if (email && users().some((x) => x.id !== u.id && x.email === email)) fail(409, 'email_taken', 'this e-mail address is already used', 'email')
    }
    Object.assign(u, {
      roles, sites, email,
      first_name: body.first_name !== undefined ? String(body.first_name).trim() : u.first_name,
      last_name: body.last_name !== undefined ? String(body.last_name).trim() : u.last_name,
      enabled: body.enabled !== undefined ? !!body.enabled : u.enabled,
    })
    if (!u.otp_configured && roles.some((r) => STAFF.includes(r)) && !u.required_actions.includes('CONFIGURE_TOTP')) u.required_actions.push('CONFIGURE_TOTP')
    return u
  })
  on('POST', '/users/:id/reset-password', ({ params, body }) => {
    admin()
    const u = userFor(params.id)
    if (u.id === selfId()) fail(403, 'cannot_modify_self', 'change your own password in your Keycloak account')
    if (body.send_email) {
      if (!u.email) fail(400, 'email_required', 'the user has no e-mail address')
      return { email_sent: true }
    }
    const temp = String(body.temporary_password ?? '')
    if (temp && temp.length < 8) fail(400, 'password_policy', 'the password does not meet the realm password policy', 'temporary_password')
    if (!u.required_actions.includes('UPDATE_PASSWORD')) u.required_actions.push('UPDATE_PASSWORD')
    return { temporary_password: temp || password(16) }
  })
  on('POST', '/users/:id/reset-otp', ({ params }) => {
    admin()
    const u = userFor(params.id)
    if (u.id === selfId()) fail(403, 'cannot_modify_self', 'reset your own authenticator in your Keycloak account')
    const removed = u.otp_configured ? 1 : 0
    Object.assign(u, { otp_configured: false, required_actions: [...new Set([...u.required_actions, 'CONFIGURE_TOTP'])] })
    return { user_id: u.id, otp_removed: removed, otp_configured: false }
  })
  on('POST', '/users/:id/logout', ({ params }) => (admin(), userFor(params.id), { logged_out: true }))

  // ── tablet accounts ─────────────────────────────────────────────────────────
  const accountFor = (id?: string) => accounts().find((a) => a.id === id) ?? fail(404, 'user_not_found', 'device account not found')
  const withDevice = (a: DemoDeviceAccount) => ({ ...a, device: w.devices.find((d) => d.keycloak_user_id === a.id) })
  on('GET', '/device-accounts', () => (admin(), accounts().map(withDevice)))
  on('POST', '/device-accounts', ({ body }) => {
    admin()
    const name = String(body.name ?? '').trim()
    const s = slug(name)
    if (!s || name.length > 100) fail(400, 'invalid_name', 'name is required (max 100 characters, letters or digits)', 'name')
    if (accounts().some((a) => a.username === `device-${s}`)) fail(409, 'username_taken', 'this username is already used', 'username')
    const a: DemoDeviceAccount = { id: uid(), username: `device-${s}`, name, enabled: true, created_at: iso(new Date()) }
    accounts().push(a)
    return { account: withDevice(a), password: password(24) }
  })
  on('POST', '/device-accounts/:id/reset-password', ({ params }) => (admin(), { account: withDevice(accountFor(params.id)), password: password(24) }))
  on('PUT', '/device-accounts/:id', ({ params, body }) => {
    admin()
    const a = accountFor(params.id)
    if (typeof body.enabled !== 'boolean') fail(400, 'invalid_body', 'enabled is required')
    a.enabled = body.enabled as boolean
    return withDevice(a)
  })
}

/** Audit row for reading a visitor's full ID number (GET /visitors/:id is audited, D-046). */
export function visitorReadAudit(visitorId: string): AuditRow {
  const r = role()
  return {
    id: uid(), user_id: `demo/${r}`, realm: 'demo', actor: `demo.${r}`, roles: getUserRoles(), action: 'GET /api/v1/visitors/:id', method: 'GET',
    route: '/api/v1/visitors/:id', resource_type: 'visitors', resource_id: visitorId, params: { id: visitorId }, request_id: uid().slice(-12),
    ip: '127.0.0.1', user_agent: navigator.userAgent, status_code: 200, created_at: iso(new Date()),
  }
}
