// API types for user / tablet-account management, tablet health, attendance
// override history and visitor details — mirror docs/api.md (F1, F4, F6,
// D-046, D-049). Re-exported from ./index.

import type { AttendanceRecord, AttendanceStatus, Device, Photo, VisitorLog } from './index'

// ── Users (GET/POST/PUT /users) ──────────────────────────────────────────────

/** Web roles an admin can assign (`platform-admin` only by a platform admin). */
export type AssignableRole = 'admin' | 'supervisor' | 'hr' | 'client-readonly' | 'platform-admin'

/** Keycloak required actions the API sets (others are shown verbatim). */
export type RequiredAction = 'UPDATE_PASSWORD' | 'CONFIGURE_TOTP' | 'VERIFY_EMAIL' | 'UPDATE_PROFILE' | (string & {})

export interface ManagedUser {
  id: string
  username: string
  email: string
  first_name: string
  last_name: string
  enabled: boolean
  roles: string[]
  /** Allowed sites; empty = the whole realm (D-042). */
  sites: string[]
  otp_configured: boolean
  required_actions: RequiredAction[]
  created_at: string
}

export interface UserCreateInput {
  username: string
  email?: string
  first_name?: string
  last_name?: string
  roles: string[]
  sites: string[]
  send_invite: boolean
  temporary_password?: string
}

export interface UserCreateResult {
  user: ManagedUser
  invite_sent: boolean
  /** `invite_failed`: the e-mail could not be sent; set a password instead. */
  invite_error?: string
  /** Only when not invited: shown once. */
  temporary_password?: string
}

export interface UserUpdateInput {
  first_name?: string
  last_name?: string
  email?: string
  enabled?: boolean
  roles?: string[]
  sites?: string[]
}

export type PasswordResetResult = { temporary_password: string; email_sent?: undefined } | { email_sent: true; temporary_password?: undefined }

// ── Tablet accounts (GET/POST/PUT /device-accounts) ──────────────────────────

export interface DeviceAccount {
  id: string
  /** `device-<slug>` */
  username: string
  name: string
  enabled: boolean
  created_at: string
  /** The registered tablet using this account (see /devices). */
  device?: Device
}

export interface DeviceAccountSecret {
  account: DeviceAccount
  /** Random 24-character password, shown once. */
  password: string
}

// ── Tablet health beacon (POST /device/telemetry → GET /devices) ─────────────

export interface DeviceHealth {
  app_version: string
  app_build?: string
  platform?: string
  os_version?: string
  outbox_pending: number
  outbox_failed: number
  outbox_paused?: boolean
  sos_pending: number
  last_sync?: string
  roster_synced_at?: string
  /** Tablet clock minus server clock, in ms. */
  clock_offset_ms: number
  private_ca: boolean
  pin_failures?: number
  db_key_reset?: boolean
  errors?: string[]
  sent_at?: string
  received_at: string
}

// ── Attendance override history (F6) ─────────────────────────────────────────

export interface AttendanceOverride {
  id: string
  attendance_id: string
  shift_id?: string
  site_id: string
  /** null/absent = there was no record before the override. */
  before_status?: AttendanceStatus | null
  before_clock_in_at?: string | null
  before_clock_out_at?: string | null
  before_reason?: string | null
  after_status: AttendanceStatus
  after_clock_in_at?: string | null
  after_clock_out_at?: string | null
  reason: string
  actor: string
  created_at: string
}

export interface AttendanceHistory {
  attendance: AttendanceRecord | null
  /** Oldest first. */
  overrides: AttendanceOverride[]
}

// ── Visitor detail (GET /visitors/:id, admin/supervisor) ─────────────────────

export interface VisitorDetail {
  /** With the full (unmasked) ID number; viewing it is audited. */
  visitor: VisitorLog
  photos: Photo[]
}
