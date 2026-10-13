/**
 * Demo data and logic for floor plans, patrol schedules + compliance,
 * signed QR codes and amended incidents (D-037, D-038, D-043, D-048).
 * Pure functions over the demo World; used by mockApi.ts.
 */
import { clock, dueOffsets, validateSchedule } from '../lib/patrolSchedule'
import { addDays, isoWeekday, localDateOf, todayIn, zonedToUtcIso } from '../lib/time'
import type { Floorplan, Incident, PatrolRoute, RouteSchedule } from '../types'
import { iso, type World } from './world'

/** Ids of seeded demo records (own namespace: a reloaded world must not collide with world.ts ids). */
let pseq = 0
const pid = () => `00000000-0000-4000-9000-${(++pseq).toString(16).padStart(12, '0')}`

/** Random id for records created while using the demo. */
export const newId = () => (typeof crypto !== 'undefined' && 'randomUUID' in crypto ? crypto.randomUUID() : pid())

export type DemoSchedule = RouteSchedule & { id: string; route_id: string }

/** Extra collections kept on the demo world (persisted with it). */
export type PatrolWorld = World & {
  floorplans?: Floorplan[]
  route_schedules?: Record<string, DemoSchedule[]>
  p2_seeded?: boolean
}

// ── floor plan pictures (inline SVG; CSP allows data: images) ────────────────

type Room = [x: number, y: number, w: number, h: number, label: string]

const esc = (v: string) => v.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')

function planSvg(title: string, rooms: Room[], extra = ''): string {
  const r = rooms
    .map(
      ([x, y, w, h, label]) =>
        `<rect x="${x}" y="${y}" width="${w}" height="${h}" fill="#ffffff" stroke="#475569" stroke-width="4"/><text x="${x + w / 2}" y="${y + h / 2 + 8}" font-family="Arial, sans-serif" font-size="24" fill="#64748b" text-anchor="middle">${esc(label)}</text>`
    )
    .join('')
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="800" viewBox="0 0 1200 800"><rect width="1200" height="800" fill="#f1f5f9"/><g fill="none" stroke="#cbd5e1" stroke-width="1">${Array.from({ length: 23 }, (_, i) => `<line x1="${(i + 1) * 50}" y1="0" x2="${(i + 1) * 50}" y2="800"/>`).join('')}${Array.from({ length: 15 }, (_, i) => `<line x1="0" y1="${(i + 1) * 50}" x2="1200" y2="${(i + 1) * 50}"/>`).join('')}</g>${r}${extra}<text x="40" y="770" font-family="Arial, sans-serif" font-size="26" font-weight="bold" fill="#334155">${esc(title)}</text><g transform="translate(1120,80)"><circle r="34" fill="#ffffff" stroke="#475569" stroke-width="3"/><path d="M0,-26 L9,6 L0,0 L-9,6 Z" fill="#334155"/><text y="26" font-family="Arial, sans-serif" font-size="16" text-anchor="middle" fill="#334155">U</text></g></svg>`
  return 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(svg)
}

const WAREHOUSE: Room[] = [
  [60, 60, 300, 220, 'Gerbang & pos'],
  [380, 60, 500, 420, 'Gudang bahan baku'],
  [900, 60, 240, 200, 'Ruang panel'],
  [60, 300, 300, 400, 'Dock bongkar muat'],
  [380, 500, 500, 200, 'Parkir truk'],
  [900, 280, 240, 420, 'Kantor gudang'],
]
const PERIMETER: Room[] = [
  [150, 150, 900, 500, 'Area gudang'],
  [60, 60, 120, 90, 'Pos timur'],
  [1020, 60, 120, 90, 'Pos barat'],
  [520, 680, 160, 80, 'Gerbang belakang'],
  [1000, 600, 140, 140, 'Tangki air'],
]
const OFFICE: Room[] = [
  [60, 60, 420, 280, 'Lobby (Lt. 1)'],
  [520, 60, 620, 280, 'Ruang server (Lt. 3)'],
  [60, 380, 260, 360, 'Tangga darurat'],
  [360, 380, 780, 360, 'Area kerja Lt. 3'],
]

/** Image URL of a demo floor plan (by its stored kind). */
export function floorplanUrl(f: Floorplan): string {
  const kind = f.object_key.split('#')[1] ?? ''
  if (kind === 'warehouse') return planSvg(f.name, WAREHOUSE)
  if (kind === 'perimeter') return planSvg(f.name, PERIMETER, '<path d="M60,60 H1140 V740 H60 Z" fill="none" stroke="#94a3b8" stroke-width="6" stroke-dasharray="18 12"/>')
  if (kind === 'office') return planSvg(f.name, OFFICE)
  // Uploaded in the demo: a neutral plan with the given name.
  return planSvg(f.name, [[100, 100, 1000, 560, 'Denah']])
}

// ── signed QR codes (D-037) ──────────────────────────────────────────────────

/** Demo stand-in for the server's signed payload SP1:<checkpoint id>:<22-char signature>. */
export const signedQR = (id: string, version: number) =>
  `SP1:${id}:${btoa(`${id}:${version}`).replace(/[^A-Za-z0-9]/g, '').padEnd(22, 'A').slice(-22)}`

// ── seeding (sample mode, once per world) ────────────────────────────────────

/** Deterministic 0..1 hash of a string (stable demo outcomes). */
export function hash01(s: string): number {
  let h = 2166136261
  for (let i = 0; i < s.length; i++) h = Math.imul(h ^ s.charCodeAt(i), 16777619)
  return ((h >>> 0) % 10_000) / 10_000
}

export function seedPatrolDemo(w: PatrolWorld): void {
  if (w.p2_seeded || w.sites.length < 2 || w.routes.length < 3) return
  w.p2_seeded = true
  const [siteA, siteB] = w.sites
  const created = iso(new Date(Date.now() - 20 * 86_400_000))
  const plan = (site: string, name: string, kind: string): Floorplan => ({
    id: pid(), site_id: site, name, object_key: `sites/${site}/floorplan/${pid()}.png#${kind}`, content_type: 'image/png',
    width: 1200, height: 800, size_bytes: 182_000 + Math.round(hash01(name) * 90_000), created_by: 'demo/admin', created_at: created,
  })
  const ground = plan(siteA.id, 'Gudang – lantai dasar', 'warehouse')
  const outer = plan(siteA.id, 'Perimeter luar', 'perimeter')
  const office = plan(siteB.id, 'Gedung kantor', 'office')
  w.floorplans = [...(w.floorplans ?? []), ground, outer, office]

  const [routeA, routeA2, routeB] = w.routes
  const place = (r: PatrolRoute, f: Floorplan, pts: ([number, number] | null)[]) => {
    r.checkpoints = r.checkpoints.map((c, i) => (pts[i] ? { ...c, floorplan_id: f.id, plan_x: pts[i]![0], plan_y: pts[i]![1] } : c))
  }
  place(routeA, ground, [[0.17, 0.2], [0.17, 0.62], [0.52, 0.32], [0.85, 0.2], [0.52, 0.75], [0.88, 0.7]])
  place(routeA2, outer, [[0.1, 0.13], [0.89, 0.83], [0.9, 0.13], [0.5, 0.9]])
  place(routeB, office, [[0.22, 0.25], [0.68, 0.25], [0.16, 0.72], null])

  // Signed codes for two routes; the perimeter route keeps old printed (legacy) codes.
  for (const r of [routeA, routeB]) r.checkpoints = r.checkpoints.map((c) => ({ ...c, qr_code: signedQR(c.id, 1), qr_signed: true, qr_version: 1 }))
  routeA2.checkpoints = routeA2.checkpoints.map((c) => ({ ...c, qr_signed: false, qr_version: 0 }))

  // Schedules: nightly rounds every 2 h + an afternoon round on weekdays; office rounds every 3 h.
  const sched = (route: PatrolRoute, s: Omit<DemoSchedule, 'id' | 'route_id' | 'created_at' | 'updated_at'>): DemoSchedule => ({
    id: pid(), route_id: route.id, created_at: created, updated_at: created, ...s,
  })
  w.route_schedules = {
    ...(w.route_schedules ?? {}),
    [routeA.id]: [
      sched(routeA, { weekdays: [1, 2, 3, 4, 5, 6, 7], start_time: '22:00', end_time: '06:00', every_minutes: 120, grace_minutes: 15, active: true }),
      sched(routeA, { weekdays: [1, 2, 3, 4, 5], start_time: '14:00', end_time: null, every_minutes: null, grace_minutes: 15, active: true }),
    ],
    [routeB.id]: [sched(routeB, { weekdays: [1, 2, 3, 4, 5], start_time: '08:00', end_time: '17:00', every_minutes: 180, grace_minutes: 20, active: true })],
  }

  // Amended incidents (D-043) and incident GPS (D-049).
  const at = (minAgo: number) => iso(new Date(Date.now() - minAgo * 60_000))
  const [i0, i1, , i3] = w.incidents
  if (i0) Object.assign(i0, { gps_lat: -6.29512, gps_lng: 107.15188 })
  if (i1) {
    Object.assign(i1, { gps_lat: -6.29431, gps_lng: 107.15302 })
    w.amendments.push({ id: pid(), incident_id: i1.id, changes: { location: 'Gudang Bahan Baku – Rak C4', severity: 'critical' }, reason: 'Lokasi diperjelas dari rekaman CCTV; nilai kerugian dikonfirmasi gudang', amended_by: 'demo/supervisor', created_at: at(60 * 20) })
    i1.severity = 'high' // filed as high, amended to critical
  }
  if (i3) {
    Object.assign(i3, { gps_lat: -6.22531, gps_lng: 106.80992 })
    w.amendments.push({ id: pid(), incident_id: i3.id, changes: { severity: 'high', description: 'Stop kontak terbakar di pantry lantai 3; kabel instalasi ikut meleleh, teknisi gedung dipanggil.' }, reason: 'Kerusakan instalasi listrik lebih parah dari laporan awal', amended_by: 'demo/supervisor', created_at: at(150) })
  }

  // Older, resolved incidents so the list pages ("showing 50 of …").
  const guardsA = w.guards.filter((g) => g.site_id === siteA.id)
  const kinds: [string, Incident['severity'], string, string][] = [
    ['suspicious', 'low', 'Orang mencurigakan berdiri lama di depan gerbang, pergi setelah ditegur.', 'Gerbang Utama'],
    ['other', 'low', 'Lampu koridor gudang berkedip, sudah dilaporkan ke teknisi.', 'Gudang Bahan Baku'],
    ['vandalism', 'medium', 'Coretan di dinding pagar belakang.', 'Pagar Belakang'],
    ['accident', 'medium', 'Forklift menyenggol rak, tidak ada korban.', 'Dock Bongkar Muat'],
    ['trespassing', 'medium', 'Pemulung masuk area parkir truk, sudah diarahkan keluar.', 'Parkir Truk'],
    ['fire_hazard', 'low', 'Puntung rokok di dekat tumpukan palet, sudah dibersihkan.', 'Dock Bongkar Muat'],
  ]
  for (let n = 0; n < 56; n++) {
    const [type, severity, description, location] = kinds[n % kinds.length]
    const g = guardsA[n % guardsA.length]
    const minAgo = 60 * (80 + Math.round(hash01(`inc${n}`) * 24 * 27))
    const t = at(minAgo)
    w.incidents.push({
      id: pid(), site_id: siteA.id, guard_id: g.id, guard_name: g.name, type, severity, description, location, status: 'resolved',
      photo_object_keys: n % 3 === 0 ? [`sites/${siteA.id}/incident/${pid()}.jpg`] : [], occurred_at: t, created_at: t, updated_at: t, received_at: t, flags: [],
    })
  }
}

// ── floor plan validation of route bodies (D-048) ────────────────────────────

type CpBody = { name?: string; floorplan_id?: string | null; plan_x?: number | null; plan_y?: number | null }

/** Error code of a route body's plan positions, like the API, or null. */
export function planPositionError(w: PatrolWorld, siteId: string, cps: CpBody[]): { code: string; msg: string } | null {
  for (const c of cps) {
    const set = [c.floorplan_id, c.plan_x, c.plan_y].filter((v) => v !== undefined && v !== null).length
    if (set === 0) continue
    if (set !== 3) return { code: 'invalid_plan_position', msg: `checkpoint "${c.name}": floorplan_id, plan_x and plan_y go together` }
    if (!(c.plan_x! >= 0 && c.plan_x! <= 1 && c.plan_y! >= 0 && c.plan_y! <= 1)) return { code: 'invalid_plan_position', msg: `checkpoint "${c.name}": plan_x/plan_y must be 0..1` }
    if (!(w.floorplans ?? []).some((f) => f.id === c.floorplan_id && f.site_id === siteId)) return { code: 'unknown_floorplan', msg: `floor plan ${c.floorplan_id} does not belong to this site` }
  }
  return null
}

/** Schedule validation like service.ValidateSchedules (message "schedule N: …", field). */
export function scheduleError(list: RouteSchedule[]): { code: string; msg: string; field?: string } | null {
  if (list.length > 20) return { code: 'too_many_schedules', msg: 'a route can have at most 20 schedule windows' }
  for (let i = 0; i < list.length; i++) {
    const e = validateSchedule(list[i])
    if (e) return { code: e.code, msg: `schedule ${i + 1}: ${e.code.replace(/_/g, ' ')}`, field: e.field }
  }
  return null
}

// ── scheduled patrols ("expectations") and compliance (D-038) ────────────────

export interface Expectation {
  id: string
  route_id: string
  site_id: string
  due: number
  grace_until: number
  status: 'pending' | 'started' | 'missed'
  late_start: boolean
  session_id?: string
  guard_id?: string
}

const HORIZON = 48 * 3600_000

/** Scheduled patrols of a site due in [from, to) (materialised 48 h ahead), with their outcome. */
export function expectations(w: PatrolWorld, siteId: string, from: number, to: number): Expectation[] {
  const site = w.sites.find((s) => s.id === siteId)
  if (!site) return []
  const tz = site.timezone || 'Asia/Jakarta'
  const now = Date.now()
  const end = Math.min(to, now + HORIZON)
  const out: Expectation[] = []
  const first = addDays(localDateOf(new Date(from), tz), -1)
  const days = Math.ceil((end - from) / 86_400_000) + 2
  for (const r of w.routes.filter((x) => x.site_id === siteId && x.active)) {
    const list = (w.route_schedules ?? {})[r.id] ?? []
    const dues: { due: number; grace: number }[] = []
    for (let d = 0; d <= days; d++) {
      const date = addDays(first, d)
      const wd = isoWeekday(date)
      for (const s of list) {
        if (!s.active || !s.weekdays.includes(wd) || validateSchedule(s)) continue
        for (const off of dueOffsets(s)) {
          const due = Date.parse(zonedToUtcIso(addDays(date, Math.floor(off / 1440)), clock(off), tz))
          if (due >= from && due < end) dues.push({ due, grace: s.grace_minutes * 60_000 })
        }
      }
    }
    dues.sort((a, b) => a.due - b.due)
    const used = new Set<string>()
    const sessions = w.sessions.filter((s) => s.route_id === r.id).map((s) => ({ s, at: Date.parse(s.started_at) }))
    const today = todayIn(tz)
    const pastToday = dues.filter((x) => x.due + x.grace < now && localDateOf(new Date(x.due), tz) === today)
    const lastPastToday = pastToday[pastToday.length - 1]?.due
    dues.forEach(({ due, grace }, k) => {
      const e: Expectation = { id: `exp-${r.id.slice(-6)}-${due}`, route_id: r.id, site_id: siteId, due, grace_until: due + grace, status: 'pending', late_start: false }
      const nextOpen = dues[k + 1] ? dues[k + 1].due - dues[k + 1].grace : due + 12 * 3600_000
      const onTime = sessions.find(({ s, at }) => !used.has(s.id) && at >= due - grace && at <= due + grace)
      const late = onTime ? undefined : sessions.find(({ s, at }) => !used.has(s.id) && at > due + grace && at < Math.min(nextOpen, due + 12 * 3600_000))
      const hit = onTime ?? late
      if (hit) {
        used.add(hit.s.id)
        Object.assign(e, { status: 'started', late_start: !onTime, session_id: hit.s.id, guard_id: hit.s.guard_id })
      } else if (now <= due + grace) {
        e.status = 'pending'
      } else if (due === lastPastToday) {
        e.status = 'missed' // keeps "missed today" visible on the dashboard
      } else {
        const h = hash01(e.id)
        const guards = w.guards.filter((g) => g.site_id === siteId && g.status === 'active')
        const guard = guards.length ? guards[Math.floor(hash01(`${e.id}g`) * guards.length)].id : undefined
        if (h < 0.74) Object.assign(e, { status: 'started', guard_id: guard })
        else if (h < 0.85) Object.assign(e, { status: 'started', late_start: true, guard_id: guard })
        else e.status = 'missed'
      }
      out.push(e)
    })
  }
  return out.sort((a, b) => a.due - b.due)
}

export type Counts = { expected: number; started_on_time: number; late: number; missed: number; pending: number; compliance_pct: number | null }

const pct = (c: Counts) => {
  const decided = c.started_on_time + c.late + c.missed
  return decided ? Math.round((c.started_on_time / decided) * 1000) / 10 : null
}

function add(c: Counts, e: Expectation) {
  c.expected++
  if (e.status === 'pending') c.pending++
  else if (e.status === 'missed') c.missed++
  else if (e.late_start) c.late++
  else c.started_on_time++
}

const zero = (): Counts => ({ expected: 0, started_on_time: 0, late: 0, missed: 0, pending: 0, compliance_pct: null })

/** GET /reports/patrol-compliance rows + totals. */
export function compliance(w: PatrolWorld, siteId: string, from: number, to: number) {
  const tz = w.sites.find((s) => s.id === siteId)?.timezone || 'Asia/Jakarta'
  const rows = new Map<string, Counts & { date: string; route_id: string; route_name: string }>()
  const totals = zero()
  for (const e of expectations(w, siteId, from, to)) {
    const date = localDateOf(new Date(e.due), tz)
    const key = `${date}|${e.route_id}`
    const row = rows.get(key) ?? { date, route_id: e.route_id, route_name: w.routes.find((r) => r.id === e.route_id)?.name ?? '', ...zero() }
    add(row, e)
    rows.set(key, row)
    add(totals, e)
  }
  const list = [...rows.values()].sort((a, b) => a.date.localeCompare(b.date) || a.route_name.localeCompare(b.route_name))
  for (const r of list) r.compliance_pct = pct(r)
  totals.compliance_pct = pct(totals)
  return { rows: list, totals }
}

/** The scheduled patrol a session satisfied (summary fields). */
export function sessionSchedule(w: PatrolWorld, s: { id: string; site_id: string; started_at: string }) {
  const at = Date.parse(s.started_at)
  const e = expectations(w, s.site_id, at - 14 * 3600_000, at + 3600_000).find((x) => x.session_id === s.id)
  return e ? { expectation_id: e.id, due_at: new Date(e.due).toISOString(), late_start: e.late_start } : { expectation_id: null, due_at: null, late_start: false }
}

/** KPI columns per guard: scheduled patrols satisfied and the on-time share. */
export function guardSchedule(w: PatrolWorld, siteId: string, from: number, to: number, guardId: string) {
  const mine = expectations(w, siteId, from, to).filter((e) => e.status === 'started' && e.guard_id === guardId)
  const onTime = mine.filter((e) => !e.late_start).length
  return { scheduled_patrols: mine.length, schedule_compliance_pct: mine.length ? Math.round((onTime / mine.length) * 1000) / 10 : null }
}
