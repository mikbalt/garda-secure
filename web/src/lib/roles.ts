/**
 * Role → capability matrix, mirroring the route roles in docs/api.md.
 * Pages and navigation consult `can(roles, capability)`.
 *
 * `platform-admin` is the install-wide operator (D-025, D-032): channel
 * settings, backups and ops alerts. A realm `admin` administers only its own
 * realm; the operator usually holds both roles.
 */

export type Role = 'admin' | 'supervisor' | 'hr' | 'client-readonly' | 'platform-admin'

export const WEB_ROLES: readonly Role[] = ['admin', 'supervisor', 'hr', 'client-readonly', 'platform-admin']

const ADMIN: Role[] = ['admin']
const PLATFORM: Role[] = ['platform-admin']
const OPS: Role[] = ['admin', 'supervisor']
const OPS_CLIENT: Role[] = ['admin', 'supervisor', 'client-readonly']
const STAFF: Role[] = ['admin', 'supervisor', 'hr']
const WEB: Role[] = ['admin', 'supervisor', 'hr', 'client-readonly']

export const CAPABILITIES = {
  // pages
  viewDashboard: WEB,
  viewAttendance: WEB,
  viewSchedules: STAFF,
  viewGuards: STAFF,
  viewPatrol: OPS_CLIENT,
  viewIncidents: OPS_CLIENT,
  viewReports: WEB,
  /** Settings page: realm sections (admin) and install-wide sections (platform-admin). */
  viewSettings: ['admin', 'platform-admin'] as Role[],
  viewSetup: ADMIN,
  viewAudit: ADMIN,
  /** Own push notifications (/notifications): every web role. */
  viewNotifications: [...WEB, 'platform-admin'] as Role[],
  // settings sections
  manageRealmSettings: ADMIN,
  /** Channels, backups, ops alerts and the ops delivery log (install-wide). */
  managePlatform: PLATFORM,
  // attendance
  reviewAttendance: OPS,
  overrideAttendance: ADMIN,
  // schedules / guards
  editSchedules: ADMIN,
  editGuards: ADMIN,
  // patrol
  editRoutes: ADMIN,
  // incidents, SOS, defects, visitors
  manageIncidents: OPS,
  viewSos: OPS,
  viewDefects: OPS,
  viewVisitors: OPS,
  // photos by kind (GET /media/url, docs/api.md)
  viewSelfies: STAFF,
  viewFieldPhotos: OPS,
  viewIncidentPhotos: OPS_CLIENT,
  // reports
  reportAttendanceDaily: WEB,
  reportAttendanceMonthly: WEB,
  reportPatrol: OPS_CLIENT,
  reportIncidents: OPS_CLIENT,
  reportKpi: STAFF,
  // site creation
  manageSites: ADMIN,
  // people & access (F1): web users and tablet accounts of the realm
  manageUsers: ADMIN,
  manageDeviceAccounts: ADMIN,
  /** Attendance override history (GET /attendance/:id/history). */
  viewAttendanceHistory: STAFF,
  /** Full visitor ID number + photos (GET /visitors/:id, audited). */
  viewVisitorDetail: OPS,
} satisfies Record<string, Role[]>

export type Capability = keyof typeof CAPABILITIES

export function can(roles: readonly string[], cap: Capability): boolean {
  return CAPABILITIES[cap].some((r) => roles.includes(r))
}

/** Photo kinds of `sites/<site>/<kind>/<id>.<ext>` object keys. */
export type MediaKind = 'selfie' | 'checkpoint' | 'incident' | 'defect' | 'visitor'

const MEDIA_CAP: Record<MediaKind, Capability> = {
  selfie: 'viewSelfies',
  visitor: 'viewFieldPhotos',
  defect: 'viewFieldPhotos',
  incident: 'viewIncidentPhotos',
  checkpoint: 'viewIncidentPhotos',
}

/** Kind of a stored photo key, or undefined for an unexpected key. */
export function mediaKindOf(key: string | null | undefined): MediaKind | undefined {
  const kind = key?.split('/')[2]
  return kind && kind in MEDIA_CAP ? (kind as MediaKind) : undefined
}

/**
 * Whether the roles may open a photo of this kind (the API answers 403
 * `media_forbidden` otherwise). Unknown kinds are allowed — the API decides.
 */
export function canSeeMedia(roles: readonly string[], kind: MediaKind | undefined): boolean {
  return kind ? can(roles, MEDIA_CAP[kind]) : true
}

/** The role shown in the user menu (most privileged first). */
export function primaryRole(roles: readonly string[]): Role | undefined {
  const order: Role[] = ['platform-admin', 'admin', 'supervisor', 'hr', 'client-readonly']
  return order.find((r) => roles.includes(r))
}
