import { describe, expect, it } from 'vitest'
import { CAPABILITIES, can, canSeeMedia, mediaKindOf, primaryRole, type Capability, type Role } from './roles'

const ROLES: Role[] = ['admin', 'supervisor', 'hr', 'client-readonly', 'platform-admin']

/** Expected matrix (docs/api.md route roles + D-025/D-032). */
const EXPECTED: Record<Capability, Role[]> = {
  viewDashboard: ['admin', 'supervisor', 'hr', 'client-readonly'],
  viewAttendance: ['admin', 'supervisor', 'hr', 'client-readonly'],
  viewSchedules: ['admin', 'supervisor', 'hr'],
  viewGuards: ['admin', 'supervisor', 'hr'],
  viewPatrol: ['admin', 'supervisor', 'client-readonly'],
  viewIncidents: ['admin', 'supervisor', 'client-readonly'],
  viewReports: ['admin', 'supervisor', 'hr', 'client-readonly'],
  viewSettings: ['admin', 'platform-admin'],
  viewSetup: ['admin'],
  viewAudit: ['admin'],
  manageRealmSettings: ['admin'],
  managePlatform: ['platform-admin'],
  reviewAttendance: ['admin', 'supervisor'],
  overrideAttendance: ['admin'],
  editSchedules: ['admin'],
  editGuards: ['admin'],
  editRoutes: ['admin'],
  manageIncidents: ['admin', 'supervisor'],
  viewSos: ['admin', 'supervisor'],
  viewDefects: ['admin', 'supervisor'],
  viewVisitors: ['admin', 'supervisor'],
  viewSelfies: ['admin', 'supervisor', 'hr'],
  viewFieldPhotos: ['admin', 'supervisor'],
  viewIncidentPhotos: ['admin', 'supervisor', 'client-readonly'],
  reportAttendanceDaily: ['admin', 'supervisor', 'hr', 'client-readonly'],
  reportAttendanceMonthly: ['admin', 'supervisor', 'hr', 'client-readonly'],
  reportPatrol: ['admin', 'supervisor', 'client-readonly'],
  reportIncidents: ['admin', 'supervisor', 'client-readonly'],
  reportKpi: ['admin', 'supervisor', 'hr'],
  manageSites: ['admin'],
}

describe('capability matrix', () => {
  it('covers every capability', () => {
    expect(Object.keys(CAPABILITIES).sort()).toEqual(Object.keys(EXPECTED).sort())
  })

  for (const cap of Object.keys(EXPECTED) as Capability[]) {
    it(`${cap}`, () => {
      for (const role of ROLES) expect(can([role], cap), `${role} → ${cap}`).toBe(EXPECTED[cap].includes(role))
    })
  }

  it('grants nothing without a known role', () => {
    for (const cap of Object.keys(EXPECTED) as Capability[]) {
      expect(can([], cap)).toBe(false)
      expect(can(['device', 'offline_access'], cap)).toBe(false)
    }
  })

  it('combines roles (operator holding admin + platform-admin)', () => {
    expect(can(['admin', 'platform-admin'], 'managePlatform')).toBe(true)
    expect(can(['admin', 'platform-admin'], 'manageRealmSettings')).toBe(true)
  })

  it('keeps realm admins away from install-wide settings', () => {
    expect(can(['admin'], 'managePlatform')).toBe(false)
  })
})

describe('photo access by kind', () => {
  it('parses the kind from object keys', () => {
    expect(mediaKindOf('sites/s1/selfie/abc.jpg')).toBe('selfie')
    expect(mediaKindOf('sites/s1/incident/abc.jpg')).toBe('incident')
    expect(mediaKindOf('sites/s1/weird/abc.jpg')).toBeUndefined()
    expect(mediaKindOf(undefined)).toBeUndefined()
  })

  it('hides selfies from client-readonly', () => {
    expect(canSeeMedia(['client-readonly'], 'selfie')).toBe(false)
    expect(canSeeMedia(['hr'], 'selfie')).toBe(true)
    expect(canSeeMedia(['client-readonly'], 'incident')).toBe(true)
    expect(canSeeMedia(['client-readonly'], 'checkpoint')).toBe(true)
  })

  it('limits visitor and defect photos to operations', () => {
    expect(canSeeMedia(['hr'], 'visitor')).toBe(false)
    expect(canSeeMedia(['hr'], 'defect')).toBe(false)
    expect(canSeeMedia(['supervisor'], 'visitor')).toBe(true)
  })

  it('lets the API decide for unknown kinds', () => {
    expect(canSeeMedia(['client-readonly'], undefined)).toBe(true)
  })
})

describe('primaryRole', () => {
  it('picks the most privileged role', () => {
    expect(primaryRole(['hr', 'admin'])).toBe('admin')
    expect(primaryRole(['admin', 'platform-admin'])).toBe('platform-admin')
    expect(primaryRole(['offline_access'])).toBeUndefined()
  })
})
