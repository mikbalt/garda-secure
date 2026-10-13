// API types — mirror docs/api.md and backend/internal/{model,repository,service}.
// Timestamps are RFC 3339 strings (UTC); dates are site-local "YYYY-MM-DD".
import type { ComplianceCounts } from './patrol'

import type { DeviceHealth } from './people'
export * from './people'

// ── Identity ─────────────────────────────────────────────────────────────────

export interface Me {
  subject: string
  username: string
  realm: string
  client_id: string
  roles: string[]
  /** Allowed sites of a site-scoped user (D-042); empty = whole realm. */
  sites?: string[]
  sites_restricted?: boolean
}

// ── Sites ────────────────────────────────────────────────────────────────────

export interface SiteConfig {
  clock_window_minutes: number
  late_grace_minutes: number
  early_out_grace_minutes: number
  absent_after_minutes: number
  idle_lock_minutes: number
  language: 'id' | 'en'
  /** GPS plausibility radius for scans (0 disables `gps_far`). */
  scan_radius_m?: number
}

export interface Site {
  id: string
  name: string
  client_id?: string
  keycloak_realm?: string
  address?: string
  timezone: string
  config_json?: Partial<SiteConfig>
  created_at: string
}

export interface SiteInput {
  name: string
  client_id?: string | null
  address?: string | null
  timezone: string
  config_json: SiteConfig
}

// ── Guards & devices ─────────────────────────────────────────────────────────

export type GuardRole = 'guard' | 'supervisor'
export type ActiveStatus = 'active' | 'inactive'

export interface Guard {
  id: string
  keycloak_user_id?: string
  name: string
  employee_id?: string
  site_id?: string
  role: GuardRole
  has_pin: boolean
  status: ActiveStatus
  created_at: string
  updated_at: string
}

export interface GuardInput {
  name: string
  employee_id?: string | null
  site_id?: string | null
  role: GuardRole
  status: ActiveStatus
  keycloak_user_id?: string | null
}

export type DeviceStatus = 'pending' | 'active' | 'revoked'

export interface Device {
  id: string
  keycloak_realm: string
  keycloak_user_id: string
  name: string
  site_id?: string
  status: DeviceStatus
  approved_by?: string
  approved_at?: string
  last_seen_at?: string
  created_at: string
  updated_at: string
  /** Latest health beacon of the tablet (D-049). */
  health?: DeviceHealth
  health_at?: string
}

// ── Schedules ────────────────────────────────────────────────────────────────

export interface ShiftTemplate {
  id: string
  site_id: string
  guard_id: string
  start_time: string // HH:MM site-local
  end_time: string // HH:MM; <= start = next day
  weekdays: number[] // 1 = Mon … 7 = Sun
  valid_from: string
  valid_until?: string
  active: boolean
  created_at: string
  updated_at: string
}

export interface ShiftTemplateInput {
  site_id?: string
  guard_id: string
  start_time: string
  end_time: string
  weekdays: number[]
  valid_from: string
  valid_until?: string | null
  active: boolean
}

export interface ShiftTemplateResult {
  template: ShiftTemplate
  generated_shifts: number
}

export interface Shift {
  id: string
  guard_id: string
  site_id: string
  template_id?: string
  start_time: string
  end_time: string
  created_at: string
}

// ── Attendance ───────────────────────────────────────────────────────────────

export type AttendanceStatus = 'present' | 'late' | 'absent'
export type DailyStatus = AttendanceStatus | 'pending'

export interface DailyAttendanceRow {
  shift_id: string
  guard_id: string
  guard_name: string
  employee_id?: string
  shift_start: string
  shift_end: string
  attendance_id?: string
  status: DailyStatus
  clock_in_at?: string
  clock_out_at?: string
  late_minutes: number
  early_out_minutes: number
  hours_worked: number
  needs_review: boolean
  override_reason?: string
  selfie_object_key?: string
  /** Plausibility flags of the clock-in (docs/api.md). */
  flags?: string[]
}

export interface DailyAttendance {
  site_id: string
  date: string
  rows: DailyAttendanceRow[]
}

export interface AttendanceRecord {
  id: string
  shift_id?: string
  guard_id: string
  /** Name of the guard (resolved by the API). */
  guard_name?: string
  site_id: string
  clock_in_at?: string
  clock_out_at?: string
  status: AttendanceStatus
  late_minutes: number
  early_out_minutes: number
  selfie_object_key?: string
  gps_lat?: number
  gps_lng?: number
  device_id?: string
  override_reason?: string
  overridden_by?: string
  overridden_at?: string
  needs_review: boolean
  review_note?: string
  reviewed_by?: string
  reviewed_at?: string
  created_at: string
  /** Server receipt time of the clock-in. */
  received_at?: string
  flags?: string[]
}

// ── Patrol ───────────────────────────────────────────────────────────────────

export interface Checkpoint {
  id: string
  name: string
  qr_code: string
  sla_minutes?: number | null
  order_index: number
  lat?: number | null
  lng?: number | null
  /** Signed `SP1:` code (D-037); false = legacy custom code. */
  qr_signed?: boolean
  qr_version?: number
  /** Floor-plan position (D-048): relative 0..1 from the top-left. */
  floorplan_id?: string | null
  plan_x?: number | null
  plan_y?: number | null
}

export interface PatrolRoute {
  id: string
  site_id: string
  name: string
  checkpoints: Checkpoint[]
  active: boolean
  created_at: string
}

export interface CheckpointInput {
  id?: string
  name: string
  qr_code?: string
  sla_minutes?: number | null
  order_index: number
  lat?: number | null
  lng?: number | null
  floorplan_id?: string
  plan_x?: number
  plan_y?: number
}

export interface RouteInput {
  site_id?: string
  name: string
  active: boolean
  checkpoints: CheckpointInput[]
}

export type SessionStatus = 'in_progress' | 'completed' | 'abandoned'
export type CheckpointStatus = 'on_time' | 'late' | 'overdue' | 'missed' | 'pending'

export interface PatrolSession {
  id: string
  route_id: string
  site_id: string
  guard_id: string
  device_id?: string
  started_at: string
  ended_at?: string
  status: SessionStatus
  created_at: string
  /** Session flags (e.g. guard_unverified). */
  flags?: string[]
}

export interface CheckpointProgress extends Checkpoint {
  scan_id?: string
  scanned_at?: string
  method?: 'qr' | 'manual'
  scanned_by?: string
  due_at?: string
  status: CheckpointStatus
  /** Plausibility flags of the scan (gps_far, too_fast, …). */
  scan_flags?: string[]
}

export interface SessionProgress {
  session: PatrolSession
  route_name: string
  checkpoints: CheckpointProgress[]
  total: number
  scanned: number
  completion_pct: number
  next_index?: number
}

export interface LivePatrol {
  guard_name: string
  /** Latest scan of the session. */
  last_scan_at?: string | null
  progress: SessionProgress
}

export interface SessionSummary {
  session_id: string
  route_id: string
  route_name: string
  guard_id: string
  guard_name: string
  started_at: string
  ended_at?: string
  status: SessionStatus
  total_checkpoints: number
  scanned_count: number
  sla_breaches: number
  completion_pct: number
  /** Scans with plausibility flags. */
  flagged_scans?: number
  /** Session flags (e.g. guard_unverified). */
  flags?: string[]
  /** Scheduled patrol the session satisfied (D-038); null = unscheduled. */
  expectation_id?: string | null
  due_at?: string | null
  late_start?: boolean
}

// ── Incidents, SOS, defects, visitors ────────────────────────────────────────

export type Severity = 'low' | 'medium' | 'high' | 'critical'
export type WorkflowStatus = 'open' | 'in_progress' | 'resolved'

export interface Incident {
  id: string
  site_id: string
  guard_id: string
  /** Name of the guard (resolved by the API). */
  guard_name?: string
  device_id?: string
  type: string
  severity: Severity
  description?: string
  photo_object_keys: string[]
  location?: string
  status: WorkflowStatus
  occurred_at: string
  created_at: string
  updated_at: string
  received_at?: string
  flags?: string[]
  /** D-043: effective values; `original_*` hold the filed value of changed fields. */
  amended?: boolean
  original_type?: string
  original_severity?: Severity
  original_description?: string
  original_location?: string
  gps_lat?: number | null
  gps_lng?: number | null
}

export interface IncidentAmendment {
  id: string
  incident_id: string
  changes: Record<string, string>
  reason: string
  amended_by: string
  created_at: string
}

/** GET /media/url and incident photos. */
export interface Photo {
  key: string
  url?: string
  uploaded: boolean
}

export interface IncidentDetail {
  incident: Incident
  guard_name: string
  amendments: IncidentAmendment[]
  photos: Photo[]
}

export interface SosAlert {
  id: string
  guard_id: string
  /** Name of the guard (resolved by the API). */
  guard_name?: string
  site_id: string
  device_id?: string
  fired_at: string
  received_at: string
  gps_lat?: number
  gps_lng?: number
  acknowledged_by?: string
  acknowledged_at?: string
  flags?: string[]
}

export interface DefectReport {
  id: string
  site_id: string
  guard_id: string
  /** Name of the guard (resolved by the API). */
  guard_name?: string
  device_id?: string
  defect_type?: string
  location?: string
  description?: string
  photo_object_key?: string
  status: WorkflowStatus
  created_at: string
  updated_at: string
}

export interface VisitorLog {
  id: string
  site_id: string
  guard_id: string
  /** Name of the guard (resolved by the API). */
  guard_name?: string
  device_id?: string
  visitor_name: string
  id_type?: string
  id_number?: string
  plate?: string
  host_name?: string
  entry_at?: string
  exit_at?: string
  purpose?: string
  photo_object_key?: string
  created_at: string
}

// ── Reports & dashboard ──────────────────────────────────────────────────────

export interface DailyCount {
  date: string
  present: number
  late: number
  absent: number
}

export interface DashboardSummary {
  shifts_today: number
  attended_today: number
  absent_today: number
  active_patrols: number
  open_incidents: number
  sos_alerts_unacknowledged: number
  open_defects: number
  attendance_pending_review: number
  weekly_attendance: DailyCount[]
  incidents_by_severity: Partial<Record<Severity, number>>
}

export interface MonthlyRecapRow {
  guard_id: string
  guard_name: string
  employee_id?: string
  scheduled_shifts: number
  present: number
  late: number
  absent: number
  early_out: number
  pending: number
  total_hours: number
  patrol_sessions: number
  patrol_completion_pct: number
}

export interface GuardKpiRow {
  guard_id: string
  guard_name: string
  attendance_rate_pct: number
  on_time_rate_pct: number
  patrol_completion_pct: number
  incidents_filed: number
  sla_breaches: number
  /** Scheduled patrols due in the range that the guard's sessions satisfied (D-038). */
  scheduled_patrols?: number
  /** Share started within the grace period; null when none. */
  schedule_compliance_pct?: number | null
}

export interface RowsReport<T> {
  site_id: string
  rows: T[]
  date?: string
  month?: string
  /** Monthly recap: the site's schedule compliance totals of the month. */
  patrol_compliance?: ComplianceCounts
}

// ── Administration ───────────────────────────────────────────────────────────

export type Channel = 'email' | 'whatsapp'

export interface EventTypes {
  event_types: string[]
  channels: Channel[]
}

export interface NotificationRule {
  id: string
  site_id: string
  event_type: string
  channel: Channel
  recipient: string
  enabled: boolean
  created_at: string
}

export type DeliveryStatus = 'queued' | 'sent' | 'failed'

export interface NotificationLogEntry {
  id: string
  site_id?: string
  event_type: string
  channel: Channel
  recipient: string
  subject: string
  body: string
  status: DeliveryStatus
  attempts: number
  last_error?: string
  created_at: string
  sent_at?: string
}

export interface ChannelSettings {
  smtp: { host: string; port: string; user: string; password: string; from: string }
  whatsapp: {
    api_url: string
    token: string
    phone_number_id: string
    template_name: string
    template_lang: string
  }
}

export type BackupStatus = 'pending' | 'running' | 'succeeded' | 'failed'

export interface Backup {
  id: string
  object_key: string
  size_bytes?: number
  status: BackupStatus
  trigger: 'scheduled' | 'manual'
  error?: string
  requested_by?: string
  created_at: string
  started_at?: string
  finished_at?: string
  /** Keycloak database dump of the same run (when configured). */
  keycloak_object_key?: string
  keycloak_size_bytes?: number
}

/** Install-wide operations alert recipient (platform-admin, D-027). */
export interface OpsRule {
  id: string
  event_type: string
  channel: Channel
  recipient: string
  enabled: boolean
  created_by?: string
  created_at: string
}

/** One row of GET /audit (append-only audit log of the realm). */
export interface AuditRow {
  id: string
  user_id: string
  realm?: string
  actor?: string
  guard_id?: string
  roles: string[]
  action: string
  method?: string
  route?: string
  resource_type?: string
  resource_id?: string
  params?: Record<string, unknown>
  request_id?: string
  /** Redacted JSON body of the request (staff writes). */
  summary?: unknown
  before?: unknown
  after?: unknown
  ip: string
  user_agent: string
  status_code: number
  created_at: string
}

export interface AuditPage {
  rows: AuditRow[]
  limit: number
  offset: number
  has_more: boolean
}

export * from './patrol'
