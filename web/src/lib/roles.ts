/**
 * Role → capability matrix, mirroring the route roles in docs/api.md.
 * Pages and navigation consult `can(roles, capability)`.
 */

export type Role = 'admin' | 'supervisor' | 'hr' | 'client-readonly'

const ADMIN: Role[] = ['admin']
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
  viewSettings: ADMIN,
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
  // reports
  reportAttendanceDaily: WEB,
  reportAttendanceMonthly: WEB,
  reportPatrol: OPS_CLIENT,
  reportIncidents: OPS_CLIENT,
  reportKpi: STAFF,
  // site creation
  manageSites: ADMIN,
} satisfies Record<string, Role[]>

export type Capability = keyof typeof CAPABILITIES

export function can(roles: readonly string[], cap: Capability): boolean {
  return CAPABILITIES[cap].some((r) => roles.includes(r))
}
