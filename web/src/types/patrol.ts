// Patrol schedules, floor plans, schedule compliance, notification channels
// and paged lists (D-037, D-038, D-045, D-047, D-048) — mirror docs/api.md.

/** Uploaded site floor plan (GET /sites/:id/floorplans). */
export interface Floorplan {
  id: string
  site_id: string
  name: string
  object_key: string
  content_type: string
  /** Pixels, as displayed. */
  width: number
  height: number
  size_bytes: number
  created_by?: string
  created_at: string
  /** Pre-signed image URL (valid ~15 min). */
  url?: string
}

/** One schedule window of a route (GET/PUT /patrol/routes/:id/schedules). */
export interface RouteSchedule {
  id?: string
  route_id?: string
  /** 1 = Monday … 7 = Sunday (the day the window starts). */
  weekdays: number[]
  /** "HH:MM", site-local. */
  start_time: string
  end_time?: string | null
  /** null = one patrol at start_time. */
  every_minutes?: number | null
  grace_minutes: number
  active: boolean
  created_at?: string
  updated_at?: string
}

export interface RouteSchedules {
  route_id: string
  schedules: RouteSchedule[]
  generated_expectations?: number
}

/** Schedule compliance counters (D-038). */
export interface ComplianceCounts {
  expected: number
  started_on_time: number
  late: number
  missed: number
  pending: number
  /** on time / (on time + late + missed); null when nothing is decided. */
  compliance_pct: number | null
}

export interface ComplianceRow extends ComplianceCounts {
  /** Site-local day of the due time. */
  date: string
  route_id: string
  route_name: string
}

export interface ComplianceReport {
  site_id: string
  from: string
  to: string
  rows: ComplianceRow[]
  totals: ComplianceCounts
}

/** GET /notifications/channels (D-047). */
export interface ChannelInfo {
  name: string
  configured: boolean
}

/** A page of a paged list (`X-Total-Count`, `X-Truncated`, D-045). */
export interface Page<T> {
  rows: T[]
  total: number
  truncated: boolean
  offset: number
}
