export interface Guard {
  id: string
  keycloak_user_id?: string
  name: string
  employee_id?: string
  role: string
  status: string
  created_at: string
  updated_at: string
}

export interface Site {
  id: string
  name: string
  client_id?: string
  keycloak_realm?: string
  address?: string
  timezone: string
  config_json?: Record<string, unknown>
  created_at: string
}

export interface Shift {
  id: string
  guard_id: string
  site_id: string
  start_time: string
  end_time: string
  repeat_rule?: string
  created_at: string
}

export interface AttendanceRecord {
  id: string
  shift_id?: string
  guard_id: string
  site_id: string
  clock_in_at?: string
  clock_out_at?: string
  status: 'pending' | 'present' | 'absent' | 'late' | 'early_out'
  selfie_object_key?: string
  gps_lat?: number
  gps_lng?: number
  override_reason?: string
  overridden_by?: string
  created_at: string
}

export interface Checkpoint {
  id: string
  name: string
  qr_code: string
  sla_minutes?: number
  order_index: number
}

export interface Route {
  id: string
  site_id: string
  name: string
  checkpoints: Checkpoint[]
  active: boolean
  created_at: string
}

export interface CheckpointScan {
  id: string
  route_id: string
  checkpoint_id: string
  guard_id: string
  scanned_at: string
  method: 'qr' | 'manual'
  photo_object_key?: string
  gps_lat?: number
  gps_lng?: number
}

export type IncidentSeverity = 'low' | 'medium' | 'high' | 'critical'
export type IncidentStatus = 'open' | 'in_progress' | 'resolved'

export interface Incident {
  id: string
  site_id: string
  guard_id: string
  type: string
  severity: IncidentSeverity
  description?: string
  photo_object_keys: string[]
  location?: string
  status: IncidentStatus
  created_at: string
  updated_at: string
}

export interface DefectReport {
  id: string
  site_id: string
  guard_id: string
  defect_type?: string
  location?: string
  description?: string
  photo_object_key?: string
  status: 'open' | 'in_progress' | 'resolved'
  created_at: string
}

export interface VisitorLog {
  id: string
  site_id: string
  guard_id: string
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

export interface SosAlert {
  id: string
  guard_id: string
  site_id: string
  fired_at: string
  gps_lat?: number
  gps_lng?: number
  acknowledged_by?: string
  acknowledged_at?: string
}

export interface DashboardSummary {
  attendance_today: number
  attendance_total: number
  active_patrols: number
  open_incidents: number
  sos_alerts_unacknowledged: number
  weekly_attendance: { date: string; present: number; absent: number }[]
}
