/**
 * Demo-mode API: an axios adapter that answers every endpoint in docs/api.md
 * from the in-memory World. Loaded with a dynamic import only when
 * config.demo is on, so it never ships in the normal request path.
 */
import type { AxiosAdapter, AxiosInstance, AxiosResponse, InternalAxiosRequestConfig } from 'axios'
import type {
  AuditRow,
  CheckpointProgress,
  DailyAttendanceRow,
  GuardKpiRow,
  MonthlyRecapRow,
  PatrolRoute,
  PatrolSession,
  SessionProgress,
  SessionSummary,
  Severity,
} from '../types'
import { getUserRoles } from '../keycloak'
import { emptyWorld, iso, seedWorld, uid, wib, type World } from './world'
import { demoHealth, demoMe, registerPeopleMocks, visibleSites, visitorReadAudit } from './mockPeople'

const STORE_KEY = 'sp.demo.world'
const MODE_KEY = 'sp.demo.mode'

export type DemoMode = 'sample' | 'empty'

function load(): World {
  try {
    const raw = sessionStorage.getItem(STORE_KEY)
    // Older saved worlds lack newer collections: fill them in.
    if (raw) return { ...emptyWorld(), ...(JSON.parse(raw) as Partial<World>) } as World
  } catch {
    /* fall through */
  }
  return demoMode() === 'empty' ? emptyWorld() : seedWorld()
}

const w: World = load()

function persist() {
  try {
    sessionStorage.setItem(STORE_KEY, JSON.stringify(w))
  } catch {
    /* storage full or blocked — keep in memory */
  }
}

export function demoMode(): DemoMode {
  try {
    return (localStorage.getItem(MODE_KEY) as DemoMode) || 'sample'
  } catch {
    return 'sample'
  }
}

/** Resets the demo data, optionally switching between sample and empty. */
export function resetDemo(mode: DemoMode = demoMode()) {
  try {
    localStorage.setItem(MODE_KEY, mode)
    sessionStorage.removeItem(STORE_KEY)
    localStorage.removeItem('sp.site')
  } catch {
    /* ignore */
  }
  window.location.reload()
}

// ── HTTP helpers ─────────────────────────────────────────────────────────────

class HttpError {
  constructor(public status: number, public code: string, public msg: string, public field?: string) {}
}
/** A page of a paged list: the body is `rows`, `X-Total-Count` is `total`. */
class Paged {
  constructor(public rows: unknown[], public total: number) {}
}
const notFound = () => new HttpError(404, 'not_found', 'not found')
const bad = (code: string, msg: string) => new HttpError(400, code, msg)

/** Role checks like the API (docs/api.md), using the demo role. */
const hasRole = (...roles: string[]) => getUserRoles().some((r) => roles.includes(r))
function requireRoles(...roles: string[]) {
  if (!hasRole(...roles)) throw new HttpError(403, 'forbidden', 'forbidden')
}
function requirePlatform() {
  if (!hasRole('platform-admin')) throw new HttpError(403, 'platform_admin_required', 'platform-admin role required')
}

type Params = Record<string, string | undefined>
type Body = Record<string, unknown>
type Handler = (p: { params: Params; query: Params; body: Body }) => unknown

const routes: { method: string; re: RegExp; keys: string[]; fn: Handler }[] = []
function on(method: string, path: string, fn: Handler) {
  const keys: string[] = []
  const re = new RegExp('^' + path.replace(/:(\w+)/g, (_, k: string) => (keys.push(k), '([^/]+)')) + '/?$')
  routes.push({ method, re, keys, fn })
}

const now = () => new Date()
const byId = <T extends { id: string }>(list: T[], id?: string) => {
  const v = list.find((x) => x.id === id)
  if (!v) throw notFound()
  return v
}
const siteOf = (q: Params) => byId(w.sites, q.site_id)
const guardName = (id: string) => w.guards.find((g) => g.id === id)?.name ?? '—'

/** [from, to) for site-local dates (WIB in the demo). */
function dayRange(date?: string): [Date, Date] {
  const start = date ? new Date(Date.parse(date + 'T00:00:00+07:00')) : wib(0, 0)
  return [start, new Date(start.getTime() + 86_400_000)]
}
function dateRange(from?: string, to?: string): [Date, Date] {
  return [dayRange(from)[0], dayRange(to)[1]]
}
function monthRange(month?: string): [Date, Date] {
  const m = month ?? new Date(Date.now() + 7 * 3600_000).toISOString().slice(0, 7)
  const [y, mo] = m.split('-').map(Number)
  return [new Date(Date.UTC(y, mo - 1, 1) - 7 * 3600_000), new Date(Date.UTC(y, mo, 1) - 7 * 3600_000)]
}
const inRange = (t: string | undefined, [a, b]: [Date, Date]) => !!t && Date.parse(t) >= a.getTime() && Date.parse(t) < b.getTime()

// ── Domain computations (ports of the backend logic) ─────────────────────────

function progress(s: PatrolSession): SessionProgress {
  const route = byId(w.routes, s.route_id)
  const scans = w.scans.filter((x) => x.session_id === s.id)
  const ended = s.status !== 'in_progress'
  let anchor = Date.parse(s.started_at)
  let scanned = 0
  let next: number | undefined
  const checkpoints: CheckpointProgress[] = route.checkpoints.map((cp, i) => {
    const sc = scans.filter((x) => x.checkpoint_id === cp.id).sort((a, b) => a.scanned_at.localeCompare(b.scanned_at))[0]
    const due = cp.sla_minutes ? anchor + cp.sla_minutes * 60_000 : undefined
    const out: CheckpointProgress = { ...cp, due_at: due ? new Date(due).toISOString() : undefined, status: 'pending' }
    if (sc) {
      scanned++
      out.scan_id = sc.id
      out.scanned_at = sc.scanned_at
      out.method = sc.method
      out.scanned_by = sc.guard_id
      out.scan_flags = sc.flags?.length ? sc.flags : undefined
      out.status = due && Date.parse(sc.scanned_at) > due ? 'late' : 'on_time'
      anchor = Date.parse(sc.scanned_at)
    } else {
      out.status = ended ? 'missed' : due && Date.now() > due ? 'overdue' : 'pending'
      if (next === undefined && !ended) next = i
      if (due) anchor = due
    }
    return out
  })
  const total = route.checkpoints.length
  return {
    session: s, route_name: route.name, checkpoints, total, scanned,
    completion_pct: total ? Math.round((scanned / total) * 1000) / 10 : 0, next_index: next,
  }
}

function summary(s: PatrolSession): SessionSummary {
  const p = progress(s)
  return {
    session_id: s.id, route_id: s.route_id, route_name: p.route_name, guard_id: s.guard_id, guard_name: guardName(s.guard_id),
    started_at: s.started_at, ended_at: s.ended_at, status: s.status, total_checkpoints: p.total, scanned_count: p.scanned,
    sla_breaches: p.checkpoints.filter((c) => c.status === 'late' || c.status === 'overdue').length, completion_pct: p.completion_pct,
    flagged_scans: w.scans.filter((x) => x.session_id === s.id && (x.flags?.length ?? 0) > 0).length,
    // D-035: session flags and the scheduled patrol it satisfied (none in the demo data).
    flags: (s as { flags?: string[] }).flags ?? [], expectation_id: null, due_at: null, late_start: false,
  } as SessionSummary
}

// ── Patrol schedules (D-035), kept on the world object ───────────────────────

type RouteScheduleInput = { weekdays: number[]; start_time: string; end_time?: string | null; every_minutes?: number | null; grace_minutes?: number; active?: boolean }
const ws = w as World & { route_schedules?: Record<string, (RouteScheduleInput & { id: string; route_id: string })[]> }
const schedulesOf = (routeId: string) => (ws.route_schedules ??= {})[routeId] ?? []

/** Number of patrols a window plans on one day (weekday 1 = Monday). */
function patrolsPerDay(sc: RouteScheduleInput, weekday: number): number {
  if (sc.active === false || !sc.weekdays.includes(weekday)) return 0
  if (!sc.every_minutes || !sc.end_time) return 1
  const [sh, sm] = sc.start_time.split(':').map(Number)
  const [eh, em] = sc.end_time.split(':').map(Number)
  let span = eh * 60 + em - (sh * 60 + sm)
  if (span <= 0) span += 1440
  return Math.ceil(span / sc.every_minutes)
}

type Compliance = { expected: number; started_on_time: number; late: number; missed: number; pending: number; compliance_pct: number | null }
const pct = (c: Compliance) => (c.started_on_time + c.late + c.missed ? Math.round((c.started_on_time / (c.started_on_time + c.late + c.missed)) * 1000) / 10 : null)

/** Demo approximation of /reports/patrol-compliance: sessions of the day count as on-time starts. */
function compliance(siteId: string, range: [Date, Date]) {
  const rows: (Compliance & { date: string; route_id: string; route_name: string })[] = []
  const totals: Compliance = { expected: 0, started_on_time: 0, late: 0, missed: 0, pending: 0, compliance_pct: null }
  for (let t = range[0].getTime(); t < range[1].getTime() && t <= Date.now(); t += 86_400_000) {
    const date = new Date(t + 7 * 3600_000).toISOString().slice(0, 10)
    const weekday = ((new Date(t + 7 * 3600_000).getUTCDay() + 6) % 7) + 1
    for (const r of w.routes.filter((x) => x.site_id === siteId && x.active)) {
      const expected = schedulesOf(r.id).reduce((n, sc) => n + patrolsPerDay(sc, weekday), 0)
      if (!expected) continue
      const started = Math.min(expected, w.sessions.filter((x) => x.route_id === r.id && inRange(x.started_at, dayRange(date))).length)
      const row = { date, route_id: r.id, route_name: r.name, expected, started_on_time: started, late: 0, missed: expected - started, pending: 0, compliance_pct: null as number | null }
      row.compliance_pct = pct(row)
      rows.push(row)
      for (const k of ['expected', 'started_on_time', 'late', 'missed', 'pending'] as const) totals[k] += row[k]
    }
  }
  totals.compliance_pct = pct(totals)
  return { rows, totals }
}

function dailyRows(siteId: string, range: [Date, Date]): DailyAttendanceRow[] {
  return w.shifts
    .filter((s) => s.site_id === siteId && inRange(s.start_time, range))
    .sort((a, b) => a.start_time.localeCompare(b.start_time))
    .map((s) => {
      const g = w.guards.find((x) => x.id === s.guard_id)
      const a = w.attendance.find((x) => x.shift_id === s.id)
      const hours = a?.clock_in_at && a.clock_out_at ? (Date.parse(a.clock_out_at) - Date.parse(a.clock_in_at)) / 3_600_000 : 0
      return {
        shift_id: s.id, guard_id: s.guard_id, guard_name: g?.name ?? '—', employee_id: g?.employee_id,
        shift_start: s.start_time, shift_end: s.end_time, attendance_id: a?.id, status: a?.status ?? 'pending',
        clock_in_at: a?.clock_in_at, clock_out_at: a?.clock_out_at, late_minutes: a?.late_minutes ?? 0,
        early_out_minutes: a?.early_out_minutes ?? 0, hours_worked: Math.round(hours * 10) / 10,
        needs_review: a?.needs_review ?? false, override_reason: a?.override_reason, selfie_object_key: a?.selfie_object_key,
        flags: a?.flags ?? [],
      }
    })
}

function monthly(siteId: string, range: [Date, Date]): MonthlyRecapRow[] {
  const rows = new Map<string, MonthlyRecapRow>()
  for (const r of dailyRows(siteId, range)) {
    const m = rows.get(r.guard_id) ?? {
      guard_id: r.guard_id, guard_name: r.guard_name, employee_id: r.employee_id, scheduled_shifts: 0, present: 0, late: 0,
      absent: 0, early_out: 0, pending: 0, total_hours: 0, patrol_sessions: 0, patrol_completion_pct: 0,
    }
    m.scheduled_shifts++
    if (r.status === 'pending') m.pending++
    else m[r.status]++
    if (r.early_out_minutes > 0) m.early_out++
    m.total_hours = Math.round((m.total_hours + r.hours_worked) * 10) / 10
    rows.set(r.guard_id, m)
  }
  for (const s of w.sessions.filter((x) => x.site_id === siteId && inRange(x.started_at, range))) {
    const m = rows.get(s.guard_id)
    if (!m) continue
    const p = progress(s)
    m.patrol_completion_pct = Math.round(((m.patrol_completion_pct * m.patrol_sessions + p.completion_pct) / (m.patrol_sessions + 1)) * 10) / 10
    m.patrol_sessions++
  }
  return [...rows.values()].sort((a, b) => a.guard_name.localeCompare(b.guard_name))
}

// ── Routes ───────────────────────────────────────────────────────────────────

on('GET', '/me', () => demoMe(w.sites))

// Sites
on('GET', '/sites', () => visibleSites(w.sites))
on('GET', '/sites/:id', ({ params }) => byId(w.sites, params.id))
on('POST', '/sites', ({ body }) => {
  if (!String(body.name ?? '').trim()) throw bad('name_required', 'name is required')
  const s = { id: uid(), keycloak_realm: 'demo', created_at: iso(now()), timezone: 'Asia/Jakarta', ...body } as World['sites'][0]
  w.sites.push(s)
  return s
})
on('PUT', '/sites/:id', ({ params, body }) => Object.assign(byId(w.sites, params.id), body))

// Guards
on('GET', '/guards', ({ query }) => w.guards.filter((g) => !query.site_id || g.site_id === query.site_id))
on('GET', '/guards/:id', ({ params }) => byId(w.guards, params.id))
on('POST', '/guards', ({ body }) => {
  if (!body.site_id) throw bad('site_id_required', 'site_id is required')
  const g = { id: uid(), has_pin: false, role: 'guard', status: 'active', created_at: iso(now()), updated_at: iso(now()), ...body } as World['guards'][0]
  w.guards.push(g)
  return g
})
on('PUT', '/guards/:id', ({ params, body }) => Object.assign(byId(w.guards, params.id), body, { updated_at: iso(now()) }))
on('PUT', '/guards/:id/pin', ({ params, body }) => {
  if (!/^\d{4,6}$/.test(String(body.pin ?? ''))) throw bad('invalid_pin', 'PIN must be 4–6 digits')
  if (['1234', '0000', '1111', '123456'].includes(String(body.pin))) throw bad('weak_pin', 'PIN is too easy to guess')
  Object.assign(byId(w.guards, params.id), { has_pin: true, pin_locked_until: undefined })
  return { has_pin: true }
})

// Devices
on('GET', '/devices', () => w.devices.map((d, i) => ({ ...d, ...demoHealth(d, i) })))
on('PUT', '/devices/:id', ({ params, body }) => {
  const d = byId(w.devices, params.id)
  if (body.status === 'active' && !body.site_id) throw bad('site_required', 'assign a site before activating the device')
  return Object.assign(d, body, { updated_at: iso(now()), approved_at: d.approved_at ?? (body.status === 'active' ? iso(now()) : undefined) })
})

// Schedules
on('GET', '/shift-templates', ({ query }) => w.templates.filter((t) => t.site_id === query.site_id))
on('POST', '/shift-templates', ({ body }) => {
  const t = { id: uid(), active: true, created_at: iso(now()), updated_at: iso(now()), ...body } as World['templates'][0]
  w.templates.push(t)
  return { template: t, generated_shifts: generate(t) }
})
on('PUT', '/shift-templates/:id', ({ params, body }) => {
  const t = Object.assign(byId(w.templates, params.id), body, { updated_at: iso(now()) })
  w.shifts = w.shifts.filter((s) => !(s.template_id === t.id && Date.parse(s.start_time) > Date.now() && !w.attendance.some((a) => a.shift_id === s.id)))
  return { template: t, generated_shifts: generate(t) }
})
on('DELETE', '/shift-templates/:id', ({ params }) => {
  w.templates = w.templates.filter((t) => t.id !== params.id)
  w.shifts = w.shifts.filter((s) => !(s.template_id === params.id && Date.parse(s.start_time) > Date.now()))
  return null
})
function generate(t: World['templates'][0]): number {
  if (!t.active) return 0
  const [sh, sm] = t.start_time.split(':').map(Number)
  const [eh, em] = t.end_time.split(':').map(Number)
  let n = 0
  for (let day = 0; day <= 14; day++) {
    const start = wib(day, sh, sm)
    if (start.getTime() <= Date.now()) continue
    const wd = ((new Date(start.getTime() + 7 * 3600_000).getUTCDay() + 6) % 7) + 1
    if (!t.weekdays.includes(wd)) continue
    const end = eh * 60 + em <= sh * 60 + sm ? wib(day + 1, eh, em) : wib(day, eh, em)
    if (w.shifts.some((s) => s.template_id === t.id && s.start_time === iso(start))) continue
    w.shifts.push({ id: uid(), guard_id: t.guard_id, site_id: t.site_id, template_id: t.id, start_time: iso(start), end_time: iso(end), created_at: iso(now()) })
    n++
  }
  return n
}
on('GET', '/shifts', ({ query }) =>
  w.shifts
    .filter((s) => s.site_id === query.site_id && inRange(s.start_time, dateRange(query.from, query.to)) && (!query.guard_id || s.guard_id === query.guard_id))
    .sort((a, b) => a.start_time.localeCompare(b.start_time)),
)
on('POST', '/shifts', ({ body }) => {
  const s = { id: uid(), created_at: iso(now()), ...body } as World['shifts'][0]
  w.shifts.push(s)
  return s
})
on('PUT', '/shifts/:id', ({ params, body }) => {
  if (w.attendance.some((a) => a.shift_id === params.id)) throw new HttpError(409, 'shift_has_attendance', 'shift already has attendance')
  return Object.assign(byId(w.shifts, params.id), body, { template_id: undefined })
})
on('DELETE', '/shifts/:id', ({ params }) => {
  if (w.attendance.some((a) => a.shift_id === params.id)) throw new HttpError(409, 'shift_has_attendance', 'shift already has attendance')
  w.shifts = w.shifts.filter((s) => s.id !== params.id)
  return null
})

// Attendance
on('GET', '/attendance/daily', ({ query }) => ({ site_id: query.site_id, date: query.date ?? iso(wib(0, 0)).slice(0, 10), rows: dailyRows(siteOf(query).id, dayRange(query.date)) }))
on('GET', '/attendance/review', ({ query }) => w.attendance.filter((a) => a.site_id === query.site_id && a.needs_review).map((a) => ({ ...a, guard_name: guardName(a.guard_id) })))
on('POST', '/attendance/:id/review', ({ params, body }) =>
  Object.assign(byId(w.attendance, params.id), { needs_review: false, reviewed_by: 'demo', reviewed_at: iso(now()), review_note: body.note ?? undefined }),
)
on('PUT', '/attendance/shifts/:shiftId/override', ({ params, body }) => {
  if (!String(body.reason ?? '').trim()) throw bad('reason_required', 'a reason is required for an attendance override')
  const shift = byId(w.shifts, params.shiftId)
  let a = w.attendance.find((x) => x.shift_id === shift.id)
  if (!a) {
    a = { id: uid(), shift_id: shift.id, guard_id: shift.guard_id, site_id: shift.site_id, status: 'present', late_minutes: 0, early_out_minutes: 0, needs_review: false, created_at: iso(now()) }
    w.attendance.push(a)
  }
  const before = { ...a }
  const fresh = !w.attendance.some((x) => x.id === a!.id && x.overridden_at) && !before.clock_in_at && !before.override_reason
  const out = Object.assign(a, { status: body.status, override_reason: body.reason, overridden_by: 'demo/admin', overridden_at: iso(now()), needs_review: false, clock_in_at: body.clock_in_at ?? a.clock_in_at, clock_out_at: body.clock_out_at ?? a.clock_out_at })
  ;(fx.attendance_overrides ??= []).push({
    id: uid(), attendance_id: a.id, shift_id: shift.id, site_id: shift.site_id,
    before_status: fresh ? undefined : before.status, before_clock_in_at: before.clock_in_at, before_clock_out_at: before.clock_out_at, before_reason: before.override_reason,
    after_status: out.status, after_clock_in_at: out.clock_in_at, after_clock_out_at: out.clock_out_at,
    reason: String(body.reason), actor: 'demo/admin', created_at: iso(now()),
  })
  return out
})
on('GET', '/attendance/:id/history', ({ params }) => {
  const a = byId(w.attendance, params.id)
  return { attendance: a, overrides: (fx.attendance_overrides ?? []).filter((o) => o.attendance_id === a.id) }
})
on('GET', '/attendance/shifts/:shiftId/history', ({ params }) => {
  const a = w.attendance.find((x) => x.shift_id === params.shiftId)
  return { attendance: a ?? null, overrides: a ? (fx.attendance_overrides ?? []).filter((o) => o.attendance_id === a.id) : [] }
})

// Patrol
on('GET', '/patrol/routes', ({ query }) => w.routes.filter((r) => r.site_id === query.site_id && (query.active !== 'true' || r.active)))
on('GET', '/patrol/routes/:id', ({ params }) => byId(w.routes, params.id))
/** Demo stand-in for the server's signed payload SP1:<checkpoint id>:<22-char signature>. */
const signedQR = (id: string, version: number) => `SP1:${id}:${btoa(`${id}:${version}`).replace(/[^A-Za-z0-9]/g, '').padEnd(22, 'A').slice(-22)}`
type QRCheckpoint = PatrolRoute['checkpoints'][0] & { qr_signed?: boolean; qr_version?: number }
const normalizeRoute = (body: Body, base?: PatrolRoute): PatrolRoute => {
  const stored = new Map(((base?.checkpoints ?? []) as QRCheckpoint[]).map((c) => [c.id, c]))
  const cps = ((body.checkpoints as QRCheckpoint[]) ?? []).map((c, i) => {
    const old = c.id ? stored.get(c.id) : undefined
    const id = c.id || uid()
    const custom = !!c.qr_code && !c.qr_code.startsWith('SP1:')
    // D-035: new codes are signed; signed codes stay; custom codes are legacy.
    if (old?.qr_signed || !custom) {
      const version = old?.qr_signed ? (old.qr_version ?? 1) : (old?.qr_version ?? 0) + 1
      return { ...c, id, qr_code: signedQR(id, version), qr_signed: true, qr_version: version, order_index: i }
    }
    return { ...c, id, qr_signed: false, qr_version: old?.qr_version ?? 0, order_index: i }
  })
  if (!String(body.name ?? '').trim()) throw bad('name_required', 'route name is required')
  if (cps.length === 0) throw bad('checkpoints_required', 'a route needs at least one checkpoint')
  return { ...(base ?? { id: uid(), site_id: String(body.site_id), created_at: iso(now()) }), name: String(body.name), active: body.active !== false, checkpoints: cps } as PatrolRoute
}
on('POST', '/patrol/routes', ({ body }) => {
  const r = normalizeRoute(body)
  w.routes.push(r)
  return r
})
on('PUT', '/patrol/routes/:id', ({ params, body }) => {
  const r = normalizeRoute(body, byId(w.routes, params.id))
  w.routes = w.routes.map((x) => (x.id === r.id ? r : x))
  return r
})
on('POST', '/patrol/routes/:id/regenerate-qr', ({ params }) => {
  const r = byId(w.routes, params.id)
  r.checkpoints = (r.checkpoints as QRCheckpoint[]).map((c) => {
    const version = (c.qr_version ?? 0) + 1
    return { ...c, qr_code: signedQR(c.id, version), qr_signed: true, qr_version: version }
  })
  return r
})
on('GET', '/patrol/routes/:id/schedules', ({ params }) => ({ route_id: params.id, schedules: schedulesOf(byId(w.routes, params.id).id) }))
on('PUT', '/patrol/routes/:id/schedules', ({ params, body }) => {
  const r = byId(w.routes, params.id)
  const list = ((body.schedules as RouteScheduleInput[]) ?? []).map((sc, i) => {
    if (!sc.weekdays?.length) throw bad('invalid_weekdays', `schedule ${i + 1}: weekdays must be 1 (Monday) … 7 (Sunday)`)
    if (!/^\d{2}:\d{2}$/.test(sc.start_time ?? '')) throw bad('invalid_start_time', `schedule ${i + 1}: start_time must be HH:MM`)
    if (sc.every_minutes && !sc.end_time) throw bad('end_time_required', `schedule ${i + 1}: end_time is required with every_minutes`)
    return { id: uid(), route_id: r.id, end_time: null, every_minutes: null, grace_minutes: 15, active: true, created_at: iso(now()), updated_at: iso(now()), ...sc }
  })
  ;(ws.route_schedules ??= {})[r.id] = list
  return { route_id: r.id, schedules: list, generated_expectations: list.reduce((n, sc) => n + 2 * patrolsPerDay(sc, sc.weekdays[0]), 0) }
})
on('GET', '/patrol/live', ({ query }) =>
  w.sessions
    .filter((s) => s.site_id === query.site_id && s.status === 'in_progress')
    .map((s) => {
      const p = progress(s)
      const last = p.checkpoints.map((c) => c.scanned_at).filter(Boolean).sort().pop()
      return { guard_name: guardName(s.guard_id), last_scan_at: last, progress: p }
    }),
)
on('GET', '/patrol/sessions', ({ query }) =>
  w.sessions.filter((s) => s.site_id === query.site_id && inRange(s.started_at, dateRange(query.from, query.to))).sort((a, b) => b.started_at.localeCompare(a.started_at)).map(summary),
)
on('GET', '/patrol/sessions/:id', ({ params }) => progress(byId(w.sessions, params.id)))

// Incidents & field reports
on('GET', '/incidents', ({ query }) =>
  w.incidents
    .map(effective)
    .filter((i) => i.site_id === query.site_id && (!query.type || i.type === query.type) && (!query.severity || i.severity === query.severity) && (!query.status || i.status === query.status) && (!query.from || inRange(i.occurred_at, dateRange(query.from, query.to))))
    .sort((a, b) => b.occurred_at.localeCompare(a.occurred_at)),
)
on('GET', '/incidents/:id', ({ params }) => {
  const i = effective(byId(w.incidents, params.id))
  return { incident: i, guard_name: guardName(i.guard_id), amendments: w.amendments.filter((a) => a.incident_id === i.id), photos: i.photo_object_keys.map((key) => ({ key, uploaded: true, url: photoUrl(key) })) }
})
on('GET', '/incidents/:id/pdf', ({ params }) => demoPdf(`Incident ${byId(w.incidents, params.id).id}`))
on('PUT', '/incidents/:id/status', ({ params, body }) => Object.assign(byId(w.incidents, params.id), { status: body.status, updated_at: iso(now()) }))
on('POST', '/incidents/:id/amendments', ({ params, body }) => {
  if (!String(body.reason ?? '').trim()) throw bad('reason_required', 'a reason is required')
  const a = { id: uid(), incident_id: params.id!, changes: body.changes as Record<string, string>, reason: String(body.reason), amended_by: 'demo/supervisor', created_at: iso(now()) }
  w.amendments.push(a)
  return a
})
on('GET', '/sos/active', () =>
  w.sos
    .filter((s) => !s.acknowledged_at)
    .sort((a, b) => b.fired_at.localeCompare(a.fired_at))
    .map((s) => ({ id: s.id, site_id: s.site_id, site_name: w.sites.find((x) => x.id === s.site_id)?.name ?? '—', guard_id: s.guard_id, guard_name: guardName(s.guard_id), fired_at: s.fired_at, received_at: s.fired_at, gps_lat: s.gps_lat, gps_lng: s.gps_lng })),
)
on('GET', '/sos', ({ query }) => w.sos.filter((s) => s.site_id === query.site_id && (query.unack_only !== 'true' || !s.acknowledged_at)).sort((a, b) => b.fired_at.localeCompare(a.fired_at)))
on('POST', '/sos/:id/acknowledge', ({ params }) => {
  const s = byId(w.sos, params.id)
  s.acknowledged_by ??= 'demo/supervisor'
  s.acknowledged_at ??= iso(now())
  return s
})
on('GET', '/defects', ({ query }) => w.defects.filter((d) => d.site_id === query.site_id && (!query.status || d.status === query.status)).map((d) => ({ flags: [], ...d })))
on('PUT', '/defects/:id/status', ({ params, body }) => Object.assign(byId(w.defects, params.id), { status: body.status, updated_at: iso(now()) }))
on('GET', '/visitors', ({ query }) =>
  w.visitors.filter((v) => v.site_id === query.site_id && inRange(v.entry_at, dateRange(query.from, query.to))).map((v) => ({ flags: [], ...v, id_number: mask(v.id_number) })),
)
on('GET', '/visitors/:id', ({ params }) => {
  requireRoles('admin', 'supervisor')
  const v = byId(w.visitors, params.id)
  w.audit.push(visitorReadAudit(v.id)) // reading the full ID number is audited (D-046)
  persist()
  return { visitor: { flags: [], ...v }, photos: v.photo_object_key ? [{ key: v.photo_object_key, uploaded: true, url: photoUrl(v.photo_object_key) }] : [] }
})
on('GET', '/media/url', ({ query }) => {
  const kind = (query.key ?? '').split('/')[2]
  const allowed: Record<string, string[]> = {
    selfie: ['admin', 'supervisor', 'hr'],
    visitor: ['admin', 'supervisor'],
    defect: ['admin', 'supervisor'],
    incident: ['admin', 'supervisor', 'client-readonly'],
    checkpoint: ['admin', 'supervisor', 'client-readonly'],
    floorplan: ['admin', 'supervisor', 'hr', 'client-readonly'],
  }
  if (allowed[kind] && !hasRole(...allowed[kind])) throw new HttpError(403, 'media_forbidden', 'your role may not view this photo')
  return { key: query.key, uploaded: true, url: photoUrl(query.key ?? '') }
})

// Reports & dashboard
on('GET', '/dashboard/summary', ({ query }) => {
  const site = siteOf(query)
  const rows = dailyRows(site.id, dayRange())
  const weekly = Array.from({ length: 7 }, (_, i) => {
    const date = new Date(wib(i - 6, 0).getTime() + 7 * 3600_000).toISOString().slice(0, 10)
    const r = dailyRows(site.id, dayRange(date))
    return { date, present: r.filter((x) => x.status === 'present').length, late: r.filter((x) => x.status === 'late').length, absent: r.filter((x) => x.status === 'absent').length }
  })
  const open = w.incidents.filter((i) => i.site_id === site.id && i.status !== 'resolved')
  const bySev: Partial<Record<Severity, number>> = {}
  for (const i of open) bySev[i.severity] = (bySev[i.severity] ?? 0) + 1
  return {
    shifts_today: rows.length,
    attended_today: rows.filter((r) => r.status === 'present' || r.status === 'late').length,
    absent_today: rows.filter((r) => r.status === 'absent').length,
    active_patrols: w.sessions.filter((s) => s.site_id === site.id && s.status === 'in_progress').length,
    open_incidents: open.length,
    sos_alerts_unacknowledged: w.sos.filter((s) => s.site_id === site.id && !s.acknowledged_at).length,
    open_defects: w.defects.filter((d) => d.site_id === site.id && d.status !== 'resolved').length,
    attendance_pending_review: w.attendance.filter((a) => a.site_id === site.id && a.needs_review).length,
    weekly_attendance: weekly,
    incidents_by_severity: bySev,
  }
})
on('GET', '/reports/attendance/daily', ({ query }) => ({ site_id: query.site_id, date: query.date, rows: dailyRows(siteOf(query).id, dayRange(query.date)) }))
on('GET', '/reports/attendance/monthly', ({ query }) => ({
  site_id: query.site_id, month: query.month, rows: monthly(siteOf(query).id, monthRange(query.month)),
  patrol_compliance: compliance(siteOf(query).id, monthRange(query.month)).totals,
}))
on('GET', '/reports/patrol-compliance', ({ query }) => ({ site_id: query.site_id, from: query.from, to: query.to, ...compliance(siteOf(query).id, dateRange(query.from, query.to)) }))
on('GET', '/reports/patrol/sessions', ({ query }) => ({
  site_id: query.site_id,
  rows: w.sessions.filter((s) => s.site_id === query.site_id && inRange(s.started_at, dateRange(query.from, query.to))).map(summary),
}))
on('GET', '/reports/incidents', ({ query }) =>
  w.incidents
    .filter((i) => i.site_id === query.site_id && (!query.type || i.type === query.type) && (!query.severity || i.severity === query.severity) && (!query.status || i.status === query.status) && (!query.from || inRange(i.occurred_at, dateRange(query.from, query.to))))
    .sort((a, b) => b.occurred_at.localeCompare(a.occurred_at)),
)
on('GET', '/reports/guards/kpi', ({ query }): { site_id?: string; rows: GuardKpiRow[] } => ({
  site_id: query.site_id,
  rows: monthly(siteOf(query).id, dateRange(query.from, query.to)).map((m) => {
    const decided = m.present + m.late + m.absent
    return {
      guard_id: m.guard_id, guard_name: m.guard_name,
      attendance_rate_pct: decided ? Math.round(((m.present + m.late) / decided) * 1000) / 10 : 0,
      on_time_rate_pct: m.present + m.late ? Math.round((m.present / (m.present + m.late)) * 1000) / 10 : 0,
      patrol_completion_pct: m.patrol_completion_pct,
      incidents_filed: w.incidents.filter((i) => i.guard_id === m.guard_id).length,
      sla_breaches: w.sessions
        .filter((s) => s.guard_id === m.guard_id && s.site_id === query.site_id)
        .reduce((n, s) => n + progress(s).checkpoints.filter((c) => c.status === 'late' || c.status === 'overdue').length, 0),
      scheduled_patrols: 0, schedule_compliance_pct: null, // D-035 (no scheduled patrols in the demo data)
    } as GuardKpiRow
  }),
}))

// Administration
on('GET', '/notifications/event-types', () => ({ event_types: ['sos', 'sla_missed', 'absent', 'late_clock_in', 'incident_high', 'incident_filed', 'defect_opened', 'monthly_recap', 'patrol_not_started'], channels: ['email', 'whatsapp'] }))
on('GET', '/notifications/channels', () => [
  { name: 'email', configured: !!w.channels.smtp?.host },
  { name: 'whatsapp', configured: !!w.channels.whatsapp?.phone_number_id },
])
on('GET', '/notifications/rules', ({ query }) => w.rules.filter((r) => r.site_id === query.site_id))
on('POST', '/notifications/rules', ({ body }) => {
  const r = { id: uid(), enabled: true, created_at: iso(now()), ...body } as World['rules'][0]
  w.rules.push(r)
  return r
})
on('PUT', '/notifications/rules/:id', ({ params, body }) => Object.assign(byId(w.rules, params.id), { enabled: !!body.enabled }))
on('DELETE', '/notifications/rules/:id', ({ params }) => {
  w.rules = w.rules.filter((r) => r.id !== params.id)
  return null
})
on('GET', '/notifications/log', ({ query }) => w.log.filter((l) => (!query.site_id || l.site_id === query.site_id) && (!query.status || l.status === query.status)))
on('POST', '/notifications/log/:id/retry', ({ params }) => {
  Object.assign(byId(w.log, params.id), { status: 'queued', last_error: undefined })
  return { status: 'queued' }
})
on('GET', '/settings/channels', () => (requirePlatform(), w.channels))
on('PUT', '/settings/channels', ({ body }) => {
  requirePlatform()
  const next = body as unknown as World['channels']
  const MASK = '********'
  // D-032: a stored secret may only be kept while the destination is unchanged.
  const smtpMoved = (['host', 'port', 'user'] as const).some((k) => (next.smtp?.[k] ?? '') !== (w.channels.smtp?.[k] ?? ''))
  const waMoved = (['api_url', 'phone_number_id'] as const).some((k) => (next.whatsapp?.[k] ?? '') !== (w.channels.whatsapp?.[k] ?? ''))
  if ((smtpMoved && next.smtp?.password === MASK && w.channels.smtp?.password) || (waMoved && next.whatsapp?.token === MASK && w.channels.whatsapp?.token)) {
    throw new HttpError(400, 'secret_required', 're-enter the secret when changing the destination')
  }
  if (next.whatsapp?.api_url && !next.whatsapp.api_url.startsWith('https://graph.facebook.com/')) {
    throw new HttpError(400, 'whatsapp_url_not_allowed', 'whatsapp api_url not allowed')
  }
  if (next.smtp?.port && !['25', '465', '587', '2525'].includes(String(next.smtp.port))) throw new HttpError(400, 'invalid_smtp_port', 'invalid smtp port')
  const keep = (v: string | undefined, old: string | undefined) => (v === MASK ? old ?? '' : v ?? '')
  w.channels = {
    smtp: { ...next.smtp, password: keep(next.smtp?.password, w.channels.smtp?.password) ? MASK : '' },
    whatsapp: { ...next.whatsapp, token: keep(next.whatsapp?.token, w.channels.whatsapp?.token) ? MASK : '' },
  }
  return w.channels
})
on('POST', '/settings/channels/test', ({ body }) => {
  requirePlatform()
  const configured = body.channel === 'email' ? !!w.channels.smtp?.host : !!w.channels.whatsapp?.token
  const e = new HttpError(502, 'delivery_failed', 'Demo mode: messages are not really sent')
  throw Object.assign(e, { reason: configured ? 'connection_failed' : 'not_configured' })
})

// Install-wide ops alerts (platform-admin, D-027)
on('GET', '/notifications/ops-event-types', () => (requirePlatform(), { event_types: ['ops_backup_failed', 'ops_backup_stale', 'ops_sos_undelivered', 'ops_job_failing'], channels: ['email', 'whatsapp'] }))
on('GET', '/notifications/ops-rules', () => (requirePlatform(), w.opsRules))
on('POST', '/notifications/ops-rules', ({ body }) => {
  requirePlatform()
  if (!String(body.event_type ?? '').startsWith('ops_')) throw bad('invalid_event_type', 'unknown ops event')
  if (w.opsRules.some((r) => r.event_type === body.event_type && r.channel === body.channel && r.recipient === body.recipient)) throw new HttpError(409, 'duplicate', 'rule exists')
  const r = { id: uid(), enabled: true, created_by: 'demo/platform-admin', created_at: iso(now()), ...body } as World['opsRules'][0]
  w.opsRules.push(r)
  return r
})
on('PUT', '/notifications/ops-rules/:id', ({ params, body }) => (requirePlatform(), Object.assign(byId(w.opsRules, params.id), { enabled: !!body.enabled })))
on('DELETE', '/notifications/ops-rules/:id', ({ params }) => {
  requirePlatform()
  w.opsRules = w.opsRules.filter((r) => r.id !== params.id)
  return null
})
on('GET', '/notifications/ops-log', ({ query }) => (requirePlatform(), w.opsLog.filter((l) => !query.status || l.status === query.status)))
on('POST', '/notifications/ops-log/:id/retry', ({ params }) => {
  requirePlatform()
  const l = byId(w.opsLog, params.id)
  if (l.status !== 'failed') throw new HttpError(409, 'not_failed', 'only failed deliveries can be retried')
  Object.assign(l, { status: 'queued', last_error: undefined })
  return { status: 'queued' }
})

// Audit log (admin)
on('GET', '/audit', ({ query }) => {
  requireRoles('admin')
  const from = query.from ? Date.parse(query.from) : -Infinity
  const to = query.to ? Date.parse(query.to) : Infinity
  const limit = Math.min(Math.max(Number(query.limit) || 100, 1), 500)
  const offset = Math.max(Number(query.offset) || 0, 0)
  const actor = (query.actor ?? '').toLowerCase()
  const rt = query.resource_type ?? ''
  const rows = [...w.audit]
    .filter((a) => {
      const at = Date.parse(a.created_at)
      if (at < from || at > to) return false
      if (actor && !(a.actor ?? '').toLowerCase().includes(actor) && a.guard_id !== query.actor) return false
      if (rt && !(a.resource_type === rt || a.resource_type?.startsWith(`${rt}.`))) return false
      return true
    })
    .sort((a, b) => b.created_at.localeCompare(a.created_at))
  return { rows: rows.slice(offset, offset + limit), limit, offset, has_more: rows.length > offset + limit }
})

on('GET', '/backups', () => (requirePlatform(), [...w.backups].sort((a, b) => b.created_at.localeCompare(a.created_at))))
on('POST', '/backups', () => {
  requirePlatform()
  const b = { id: uid(), object_key: `postgres/securepatrol-manual-${Date.now()}.dump`, status: 'pending' as const, trigger: 'manual' as const, requested_by: 'demo/admin', created_at: iso(now()) }
  w.backups.push(b)
  setTimeout(() => Object.assign(b, { status: 'succeeded', size_bytes: 18_600_000, started_at: iso(now()), finished_at: iso(now()) }) && persist(), 2500)
  return b
})

/** A neutral placeholder photo (SVG) so demo photos render offline. */
function photoUrl(key: string): string {
  const kind = key.split('/')[2] ?? 'photo'
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="640" height="420"><defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#1e293b"/><stop offset="1" stop-color="#334155"/></linearGradient></defs><rect width="640" height="420" fill="url(#g)"/><text x="320" y="215" fill="#94a3b8" font-family="sans-serif" font-size="26" text-anchor="middle">Demo ${kind} photo</text></svg>`
  return 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(svg)
}

// ── be-features (F1, F3, F5, F6, F7, WEB-3, BE-S9): demo shapes ───────────────

type DemoUser = { id: string; username: string; email: string; first_name: string; last_name: string; enabled: boolean; roles: string[]; sites: string[]; otp_configured: boolean; required_actions: string[]; created_at: string }
type DemoDeviceAccount = { id: string; username: string; name: string; enabled: boolean; created_at: string; device?: unknown }
type DemoFloorplan = { id: string; site_id: string; name: string; object_key: string; content_type: string; width: number; height: number; size_bytes: number; created_at: string; url?: string }
type DemoOverride = Record<string, unknown> & { attendance_id: string }
const fx = w as World & { attendance_overrides?: DemoOverride[]; users?: DemoUser[]; device_accounts?: DemoDeviceAccount[]; floorplans?: DemoFloorplan[] }

function mask(v?: string | null) {
  if (!v) return v
  return v.length > 8 ? v.slice(0, 4) + '*'.repeat(v.length - 8) + v.slice(-4) : '*'.repeat(Math.max(0, v.length - 2)) + v.slice(-2)
}

/** Incident with the latest amendment per field applied (F7). */
function effective<T extends { id: string; type: string; severity: string; description?: string | null; location?: string | null }>(i: T) {
  const out: Record<string, unknown> = { ...i, amended: false }
  for (const a of w.amendments.filter((x) => x.incident_id === i.id).sort((x, y) => x.created_at.localeCompare(y.created_at))) {
    out.amended = true
    for (const [k, v] of Object.entries(a.changes)) {
      if (out[`original_${k}`] === undefined && (i as Record<string, unknown>)[k] !== v) out[`original_${k}`] = (i as Record<string, unknown>)[k]
      out[k] = v
    }
  }
  return out as T & { amended: boolean }
}

function demoPdf(title: string) {
  return new Blob([`%PDF-1.4\n% SecurePatrol demo: ${title}\n`], { type: 'application/pdf' })
}

const planSvg = (name: string) =>
  'data:image/svg+xml;utf8,' +
  encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="800"><rect width="1200" height="800" fill="#f8fafc"/><rect x="60" y="60" width="1080" height="680" fill="none" stroke="#334155" stroke-width="8"/><line x1="600" y1="60" x2="600" y2="500" stroke="#334155" stroke-width="6"/><line x1="60" y1="420" x2="420" y2="420" stroke="#334155" stroke-width="6"/><text x="600" y="780" font-family="sans-serif" font-size="28" text-anchor="middle" fill="#64748b">${name}</text></svg>`)

on('GET', '/sites/:id/floorplans', ({ params }) => (fx.floorplans ?? []).filter((f) => f.site_id === params.id).map((f) => ({ ...f, url: planSvg(f.name) })))
on('POST', '/sites/:id/floorplans', ({ params, body }) => {
  const form = body as unknown as { get?: (k: string) => unknown }
  const name = String(form.get?.('name') ?? 'Floor plan')
  const id = uid()
  const f: DemoFloorplan = { id, site_id: byId(w.sites, params.id).id, name, object_key: `sites/${params.id}/floorplan/${id}.png`, content_type: 'image/png', width: 1200, height: 800, size_bytes: 48_000, created_at: iso(now()) }
  ;(fx.floorplans ??= []).push(f)
  return { ...f, url: planSvg(name) }
})
on('DELETE', '/sites/:id/floorplans/:floorplanId', ({ params }) => {
  if (w.routes.some((r) => r.checkpoints.some((c) => (c as { floorplan_id?: string }).floorplan_id === params.floorplanId))) {
    throw new HttpError(409, 'floorplan_in_use', 'checkpoints are placed on this floor plan; move them first')
  }
  fx.floorplans = (fx.floorplans ?? []).filter((f) => f.id !== params.floorplanId)
  return null
})

// Users, tablet accounts (F1) — see mockPeople.ts.
registerPeopleMocks({
  on,
  world: w,
  fail: (status, code, msg, field) => {
    throw new HttpError(status, code, msg, field)
  },
  paged: (rows, total) => new Paged(rows, total),
})

// ── Adapter ──────────────────────────────────────────────────────────────────

function toCsv(data: unknown): string {
  const rows = Array.isArray(data) ? data : ((data as { rows?: unknown[] })?.rows ?? [])
  if (rows.length === 0) return 'no data\n'
  const keys = Object.keys(rows[0] as object)
  const esc = (v: unknown) => `"${String(v ?? '').replace(/"/g, '""')}"`
  return [keys.join(','), ...rows.map((r) => keys.map((k) => esc((r as Record<string, unknown>)[k])).join(','))].join('\n')
}

const adapter: AxiosAdapter = async (cfg: InternalAxiosRequestConfig) => {
  await new Promise((r) => setTimeout(r, 120 + Math.random() * 180)) // feel like a network
  const method = (cfg.method ?? 'get').toUpperCase()
  const path = (cfg.url ?? '').split('?')[0]
  const query = Object.fromEntries(Object.entries(cfg.params ?? {}).map(([k, v]) => [k, v == null ? undefined : String(v)])) as Params
  const body = typeof cfg.data === 'string' && cfg.data ? (JSON.parse(cfg.data) as Body) : ((cfg.data as Body) ?? {})
  const respond = (status: number, data: unknown, headers: Record<string, string> = {}): AxiosResponse =>
    ({ data, status, statusText: String(status), headers, config: cfg, request: {} }) as AxiosResponse

  for (const r of routes) {
    if (r.method !== method) continue
    const m = r.re.exec(path)
    if (!m) continue
    const params = Object.fromEntries(r.keys.map((k, i) => [k, decodeURIComponent(m[i + 1])]))
    try {
      const data = r.fn({ params, query, body })
      if (method !== 'GET') {
        recordAudit(method, path, params, body)
        persist()
      }
      if (query.format === 'csv' || query.format === 'xlsx') {
        // Demo: XLSX downloads are served as CSV.
        const blob = new Blob(['﻿' + toCsv(data)], { type: 'text/csv' })
        return respond(200, blob, { 'content-disposition': `attachment; filename="demo${path.replace(/\//g, '-')}.csv"` })
      }
      if (query.format === 'pdf') {
        return respond(200, demoPdf(path), { 'content-disposition': `attachment; filename="demo${path.replace(/\//g, '-')}.pdf"` })
      }
      if (data instanceof Paged) {
        return respond(200, data.rows, { 'x-total-count': String(data.total) })
      }
      if (Array.isArray(data)) {
        return respond(200, data, { 'x-total-count': String(data.length) })
      }
      return respond(method === 'POST' && data !== null ? 201 : data === null ? 204 : 200, data)
    } catch (e) {
      if (e instanceof HttpError) {
        const extra = (e as HttpError & { reason?: string }).reason
        const err = Object.assign(new Error(e.msg), {
          isAxiosError: true,
          config: cfg,
          response: respond(e.status, { error: e.msg, code: e.code, ...(e.field ? { field: e.field } : {}), ...(extra ? { reason: extra } : {}) }),
        })
        throw err
      }
      throw e
    }
  }
  const err = Object.assign(new Error('demo: no handler'), { isAxiosError: true, config: cfg, response: respond(404, { error: `Demo mode has no handler for ${method} ${path}`, code: 'not_found' }) })
  throw err
}

const SECRET_KEYS = ['password', 'token', 'secret', 'pin', 'id_number']

function redact(v: unknown): unknown {
  if (Array.isArray(v)) return v.map(redact)
  if (v && typeof v === 'object') {
    return Object.fromEntries(Object.entries(v as Record<string, unknown>).map(([k, x]) => [k, SECRET_KEYS.some((s) => k.toLowerCase().includes(s)) ? '[redacted]' : redact(x)]))
  }
  return v
}

/** Appends an audit row for a successful demo write (like the API's audit middleware). */
function recordAudit(method: string, path: string, params: Record<string, string>, body: Body) {
  const route = '/api/v1' + path.replace(/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/g, ':id')
  const segs = path.split('/').filter((s) => s && !/^[0-9a-f-]{36}$/.test(s))
  const roles = getUserRoles()
  const row: AuditRow = {
    id: uid(), user_id: `demo/${roles[0] ?? 'user'}`, realm: 'demo', actor: `demo.${roles[0] ?? 'user'}`, roles, action: `${method} ${route}`, method, route,
    resource_type: segs.slice(0, 2).join('.') || undefined, resource_id: Object.values(params)[0], params: Object.keys(params).length ? params : undefined,
    request_id: uid().slice(-12), summary: Object.keys(body).length ? redact(body) : undefined, ip: '127.0.0.1', user_agent: navigator.userAgent,
    status_code: method === 'POST' ? 201 : 200, created_at: iso(now()),
  }
  w.audit.push(row)
}

export function installDemoApi(client: AxiosInstance) {
  client.defaults.adapter = adapter
}
