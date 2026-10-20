/**
 * Demo mode data (config.demo). An in-memory world with realistic Indonesian
 * sample data, generated relative to "now" so it always looks live. Used only
 * when the app runs without a backend — never in a normal deployment.
 */
import type {
  AttendanceRecord,
  AuditRow,
  Backup,
  ChannelSettings,
  DefectReport,
  Device,
  Guard,
  Incident,
  IncidentAmendment,
  NotificationLogEntry,
  NotificationRule,
  OpsRule,
  PatrolRoute,
  PatrolSession,
  Shift,
  ShiftTemplate,
  Site,
  SosAlert,
  VisitorLog,
} from '../types'

export interface Scan {
  id: string
  session_id: string
  checkpoint_id: string
  guard_id: string
  scanned_at: string
  method: 'qr' | 'manual'
  flags?: string[]
}

export interface World {
  sites: Site[]
  guards: Guard[]
  devices: Device[]
  templates: ShiftTemplate[]
  shifts: Shift[]
  attendance: AttendanceRecord[]
  routes: PatrolRoute[]
  sessions: PatrolSession[]
  scans: Scan[]
  incidents: Incident[]
  amendments: IncidentAmendment[]
  sos: SosAlert[]
  defects: DefectReport[]
  visitors: VisitorLog[]
  rules: NotificationRule[]
  log: NotificationLogEntry[]
  backups: Backup[]
  channels: ChannelSettings
  /** Install-wide ops alerts (platform-admin). */
  opsRules: OpsRule[]
  opsLog: NotificationLogEntry[]
  /** Audit log of the demo realm, newest last. */
  audit: AuditRow[]
}

// ── helpers ──────────────────────────────────────────────────────────────────

let seq = 1000
export function uid(): string {
  seq += 1
  const hex = seq.toString(16).padStart(12, '0')
  return `00000000-0000-4000-8000-${hex}`
}

/** Deterministic PRNG so the demo looks the same on every load. */
function rng(seed: number) {
  let s = seed >>> 0
  return () => {
    s = (s * 1664525 + 1013904223) >>> 0
    return s / 2 ** 32
  }
}

const WIB = 7 * 60 // minutes offset of Asia/Jakarta (no DST)

/** UTC Date for a WIB wall-clock time `dayOffset` days from today. */
export function wib(dayOffset: number, hh: number, mm = 0): Date {
  const now = new Date()
  const local = new Date(now.getTime() + WIB * 60_000)
  const d = Date.UTC(local.getUTCFullYear(), local.getUTCMonth(), local.getUTCDate() + dayOffset, hh, mm)
  return new Date(d - WIB * 60_000)
}

export const iso = (d: Date) => d.toISOString()
const minutes = (d: Date, m: number) => new Date(d.getTime() + m * 60_000)

/** Start of "today" in WIB, as UTC. */
export function todayStartWib(): Date {
  return wib(0, 0)
}

const emptyChannels: ChannelSettings = {
  smtp: { host: '', port: '587', user: '', password: '', from: '' },
  whatsapp: { api_url: 'https://graph.facebook.com/v18.0', token: '', phone_number_id: '', template_name: '', template_lang: 'id' },
}

// ── world ────────────────────────────────────────────────────────────────────

export function emptyWorld(): World {
  return {
    sites: [], guards: [], devices: [], templates: [], shifts: [], attendance: [], routes: [], sessions: [],
    scans: [], incidents: [], amendments: [], sos: [], defects: [], visitors: [], rules: [], log: [], backups: [],
    channels: structuredClone(emptyChannels), opsRules: [], opsLog: [], audit: [],
  }
}

export function seedWorld(): World {
  const w = emptyWorld()
  const rand = rng(42)
  const now = new Date()
  const created = iso(wib(-40, 9))

  const siteA: Site = {
    id: uid(), name: 'Gudang Cikarang', client_id: 'PT Logistik Nusantara', keycloak_realm: 'demo',
    address: 'Kawasan Industri Jababeka Blok C-12, Cikarang', timezone: 'Asia/Jakarta',
    config_json: { clock_window_minutes: 15, late_grace_minutes: 5, early_out_grace_minutes: 5, absent_after_minutes: 30, idle_lock_minutes: 5, language: 'id' },
    created_at: created,
  }
  const siteB: Site = {
    id: uid(), name: 'Kantor Pusat Sudirman', client_id: 'PT Logistik Nusantara', keycloak_realm: 'demo',
    address: 'Jl. Jend. Sudirman Kav. 52, Jakarta Selatan', timezone: 'Asia/Jakarta',
    config_json: { clock_window_minutes: 15, late_grace_minutes: 5, early_out_grace_minutes: 5, absent_after_minutes: 30, idle_lock_minutes: 5, language: 'id' },
    created_at: created,
  }
  w.sites.push(siteA, siteB)

  const mkGuard = (name: string, site: Site, emp: string, role: 'guard' | 'supervisor' = 'guard', pin = true): Guard => {
    const g: Guard = {
      id: uid(), name, employee_id: emp, site_id: site.id, role, has_pin: pin, status: 'active',
      created_at: created, updated_at: created,
    }
    w.guards.push(g)
    return g
  }
  const A = [
    mkGuard('Budi Santoso', siteA, 'GS-0101', 'supervisor'),
    mkGuard('Agus Pratama', siteA, 'GS-0102'),
    mkGuard('Dedi Kurniawan', siteA, 'GS-0103'),
    mkGuard('Rina Wulandari', siteA, 'GS-0104'),
    mkGuard('Joko Susilo', siteA, 'GS-0105'),
    mkGuard('Siti Rahmawati', siteA, 'GS-0106'),
    mkGuard('Fajar Nugroho', siteA, 'GS-0107'),
    mkGuard('Hendra Saputra', siteA, 'GS-0108', 'guard', false),
  ]
  const B = [
    mkGuard('Andi Firmansyah', siteB, 'GS-0201', 'supervisor'),
    mkGuard('Wahyu Hidayat', siteB, 'GS-0202'),
    mkGuard('Yusuf Maulana', siteB, 'GS-0203'),
    mkGuard('Dewi Lestari', siteB, 'GS-0204'),
  ]

  // Devices: one active per site, one waiting for approval.
  w.devices.push(
    { id: uid(), keycloak_realm: 'demo', keycloak_user_id: 'tablet-ckr-1', name: 'Tablet Pos Utama Cikarang', site_id: siteA.id, status: 'active', approved_by: 'demo/admin', approved_at: created, last_seen_at: iso(minutes(now, -2)), created_at: created, updated_at: created },
    { id: uid(), keycloak_realm: 'demo', keycloak_user_id: 'tablet-sdm-1', name: 'Tablet Lobby Sudirman', site_id: siteB.id, status: 'active', approved_by: 'demo/admin', approved_at: created, last_seen_at: iso(minutes(now, -6)), created_at: created, updated_at: created },
    { id: uid(), keycloak_realm: 'demo', keycloak_user_id: 'tablet-ckr-2', name: 'Tablet Pos Belakang', status: 'pending', created_at: iso(minutes(now, -50)), updated_at: iso(minutes(now, -50)) },
  )

  // Shift templates: morning / afternoon / night rotations.
  const shiftsDef: [Guard, number, number][] = [
    [A[0], 7, 15], [A[1], 7, 15], [A[2], 15, 23], [A[3], 15, 23], [A[4], 23, 7], [A[5], 23, 7], [A[6], 7, 15],
    [B[0], 7, 15], [B[1], 7, 15], [B[2], 15, 23], [B[3], 23, 7],
  ]
  for (const [g, start, end] of shiftsDef) {
    w.templates.push({
      id: uid(), site_id: g.site_id!, guard_id: g.id,
      start_time: `${String(start).padStart(2, '0')}:00`, end_time: `${String(end).padStart(2, '0')}:00`,
      weekdays: g.name.startsWith('Fajar') ? [6, 7] : [1, 2, 3, 4, 5, 6, 7], valid_from: iso(wib(-40, 0)).slice(0, 10),
      active: true, created_at: created, updated_at: created,
    })
  }

  // Concrete shifts from 7 days ago to 7 days ahead, attendance for the past.
  for (const t of w.templates) {
    const [sh] = t.start_time.split(':').map(Number)
    const [eh] = t.end_time.split(':').map(Number)
    for (let day = -7; day <= 7; day++) {
      const start = wib(day, sh)
      const wd = ((start.getUTCDay() + 6) % 7) + 1
      if (!t.weekdays.includes(wd)) continue
      const end = eh <= sh ? wib(day + 1, eh) : wib(day, eh)
      const shift: Shift = { id: uid(), guard_id: t.guard_id, site_id: t.site_id, template_id: t.id, start_time: iso(start), end_time: iso(end), created_at: created }
      w.shifts.push(shift)
      if (start.getTime() > now.getTime()) continue

      const r = rand()
      const absentDue = minutes(start, 30).getTime() < now.getTime()
      if (r < 0.07 && absentDue) {
        w.attendance.push(record(shift, 'absent'))
        continue
      }
      if (day === 0 && start.getTime() > now.getTime() - 20 * 60_000) continue // just started: pending
      const late = r > 0.8 ? Math.round(6 + rand() * 25) : 0
      const inAt = minutes(start, late ? late : -Math.round(rand() * 12))
      const ended = end.getTime() < now.getTime()
      const early = ended && rand() > 0.9 ? Math.round(10 + rand() * 40) : 0
      const outAt = ended ? minutes(end, early ? -early : Math.round(rand() * 8)) : undefined
      const rec = record(shift, late ? 'late' : 'present', inAt, outAt)
      rec.late_minutes = late
      rec.early_out_minutes = early
      w.attendance.push(rec)
    }
  }
  // One attendance conflict waiting for supervisor review.
  const review = w.attendance.find((a) => a.status === 'late')
  if (review) {
    review.needs_review = true
    review.review_note = 'Clock-in synced after the shift was auto-flagged absent'
    review.flags = ['late_sync']
  }
  // Plausibility flags (docs/api.md): a back-dated "on time" clock-in goes to review too.
  const present = w.attendance.filter((a) => a.status === 'present' && a.clock_in_at && a.site_id === siteA.id)
  const backdated = present[present.length - 3]
  if (backdated) {
    backdated.flags = ['backdated_on_time', 'late_sync']
    backdated.needs_review = true
    backdated.received_at = iso(minutes(new Date(backdated.clock_in_at!), 47))
  }
  const ahead = present[present.length - 6]
  if (ahead) ahead.flags = ['clock_ahead']

  function record(shift: Shift, status: AttendanceRecord['status'], inAt?: Date, outAt?: Date): AttendanceRecord {
    return {
      id: uid(), shift_id: shift.id, guard_id: shift.guard_id, site_id: shift.site_id, status,
      clock_in_at: inAt && iso(inAt), clock_out_at: outAt && iso(outAt), late_minutes: 0, early_out_minutes: 0,
      selfie_object_key: inAt ? `sites/${shift.site_id}/selfie/${uid()}.jpg` : undefined,
      needs_review: false, created_at: iso(inAt ?? new Date(shift.start_time)), received_at: inAt && iso(inAt), flags: [],
    }
  }

  // Patrol routes.
  // Demo: the first checkpoints already have a GPS position learned from scans (D-059).
  const cp = (name: string, i: number, sla: number | null) => ({
    id: uid(), name, qr_code: `SP-${name.toUpperCase().replace(/[^A-Z]/g, '').slice(0, 6)}-${i + 1}`, sla_minutes: sla, order_index: i,
    ...(i < 3 ? { lat: -6.2944 + i * 0.0004, lng: 107.1531 + i * 0.0003 } : {}),
  })
  const routeA: PatrolRoute = {
    id: uid(), site_id: siteA.id, name: 'Patroli Keliling Gudang', active: true, created_at: created,
    checkpoints: [cp('Gerbang Utama', 0, 10), cp('Dock Bongkar Muat', 1, 15), cp('Gudang Bahan Baku', 2, 15), cp('Ruang Panel Listrik', 3, 10), cp('Parkir Truk', 4, 15), cp('Pagar Belakang', 5, 20)],
  }
  const routeA2: PatrolRoute = {
    id: uid(), site_id: siteA.id, name: 'Patroli Malam Perimeter', active: true, created_at: created,
    checkpoints: [cp('Pos Jaga Timur', 0, 15), cp('Tangki Air', 1, 20), cp('Pos Jaga Barat', 2, 20), cp('Gerbang Belakang', 3, 15)],
  }
  const routeB: PatrolRoute = {
    id: uid(), site_id: siteB.id, name: 'Patroli Lantai Kantor', active: true, created_at: created,
    checkpoints: [cp('Lobby', 0, 10), cp('Ruang Server Lt. 3', 1, 15), cp('Tangga Darurat', 2, 10), cp('Basement Parkir', 3, 15)],
  }
  w.routes.push(routeA, routeA2, routeB)

  const addSession = (route: PatrolRoute, guard: Guard, startedMinAgo: number, scanned: number[], ended: boolean, flags: Record<number, string[]> = {}) => {
    const start = minutes(now, -startedMinAgo)
    const s: PatrolSession = {
      id: uid(), route_id: route.id, site_id: route.site_id, guard_id: guard.id, started_at: iso(start),
      status: ended ? 'completed' : 'in_progress', ended_at: ended ? iso(minutes(start, scanned[scanned.length - 1] + 3)) : undefined,
      created_at: iso(start),
    }
    w.sessions.push(s)
    scanned.forEach((m, i) => {
      w.scans.push({ id: uid(), session_id: s.id, checkpoint_id: route.checkpoints[i].id, guard_id: guard.id, scanned_at: iso(minutes(start, m)), method: i === 2 ? 'manual' : 'qr', flags: flags[i] ?? [] })
    })
  }
  addSession(routeA, A[1], 34, [6, 19, 31], false) // running, on track
  addSession(routeB, B[1], 41, [8, 30], false, { 1: ['gps_far'] }) // running, one overdue
  addSession(routeA, A[0], 300, [7, 20, 33, 41, 55, 70], true, { 4: ['too_fast'] })
  addSession(routeA2, A[4], 600, [10, 28, 45], true)
  addSession(routeB, B[0], 1500, [9, 22, 30, 44], true)

  // Incidents.
  const inc = (site: Site, g: Guard, type: string, severity: Incident['severity'], hoursAgo: number, status: Incident['status'], description: string, location: string, photos = 1): Incident => {
    const at = iso(minutes(now, -hoursAgo * 60))
    const i: Incident = {
      id: uid(), site_id: site.id, guard_id: g.id, guard_name: g.name, type, severity, description, location, status,
      photo_object_keys: Array.from({ length: photos }, () => `sites/${site.id}/incident/${uid()}.jpg`),
      occurred_at: at, created_at: at, updated_at: at, received_at: at, flags: [],
    }
    w.incidents.push(i)
    return i
  }
  inc(siteA, A[2], 'trespassing', 'high', 1.5, 'open', 'Dua orang tidak dikenal terlihat di area pagar belakang, kabur saat didekati.', 'Pagar Belakang', 2)
  inc(siteA, A[1], 'theft', 'critical', 26, 'in_progress', 'Satu palet kabel tembaga hilang dari Gudang Bahan Baku. Segel pintu rusak.', 'Gudang Bahan Baku', 3).flags = ['late_sync']
  inc(siteA, A[5], 'medical', 'medium', 50, 'resolved', 'Operator forklift pingsan karena kelelahan, sudah ditangani klinik.', 'Dock Bongkar Muat')
  inc(siteB, B[2], 'fire_hazard', 'medium', 5, 'open', 'Stop kontak terbakar di pantry lantai 3, sudah dicabut.', 'Pantry Lt. 3')
  inc(siteB, B[1], 'other', 'low', 72, 'resolved', 'Lampu tangga darurat mati.', 'Tangga Darurat', 0)

  // SOS: one live, one acknowledged.
  w.sos.push(
    { id: uid(), guard_id: A[3].id, guard_name: A[3].name, site_id: siteA.id, fired_at: iso(minutes(now, -3)), received_at: iso(minutes(now, -3)), gps_lat: -6.2944, gps_lng: 107.1531, flags: [] },
    { id: uid(), guard_id: B[2].id, guard_name: B[2].name, site_id: siteB.id, fired_at: iso(minutes(now, -12)), received_at: iso(minutes(now, -1)), flags: ['late_sync'] },
    { id: uid(), guard_id: B[3].id, guard_name: B[3].name, site_id: siteB.id, fired_at: iso(minutes(now, -60 * 30)), received_at: iso(minutes(now, -60 * 30)), gps_lat: -6.2253, gps_lng: 106.8099, acknowledged_by: 'demo/supervisor', acknowledged_at: iso(minutes(now, -60 * 30 + 2)), flags: [] },
  )

  // Defects and visitors.
  w.defects.push(
    { id: uid(), site_id: siteA.id, guard_id: A[4].id, guard_name: A[4].name, defect_type: 'CCTV', location: 'Parkir Truk', description: 'Kamera CCTV #7 tidak menampilkan gambar.', status: 'open', photo_object_key: `sites/${siteA.id}/defect/${uid()}.jpg`, created_at: iso(minutes(now, -300)), updated_at: iso(minutes(now, -300)) },
    { id: uid(), site_id: siteA.id, guard_id: A[0].id, guard_name: A[0].name, defect_type: 'Lampu', location: 'Pagar Belakang', description: 'Lampu sorot padam di sisi timur.', status: 'in_progress', created_at: iso(minutes(now, -1600)), updated_at: iso(minutes(now, -900)) },
    { id: uid(), site_id: siteB.id, guard_id: B[1].id, guard_name: B[1].name, defect_type: 'Pintu', location: 'Basement Parkir', description: 'Pintu akses kartu tidak mengunci.', status: 'open', created_at: iso(minutes(now, -200)), updated_at: iso(minutes(now, -200)) },
  )
  const vis = (site: Site, g: Guard, name: string, purpose: string, host: string, plate: string | undefined, inMin: number, outMin?: number) =>
    w.visitors.push({ id: uid(), site_id: site.id, guard_id: g.id, guard_name: g.name, visitor_name: name, id_type: 'KTP', id_number: `32${Math.floor(rand() * 1e14)}`, plate, host_name: host, purpose, entry_at: iso(minutes(now, -inMin)), exit_at: outMin !== undefined ? iso(minutes(now, -outMin)) : undefined, photo_object_key: `sites/${site.id}/visitor/${uid()}.jpg`, created_at: iso(minutes(now, -inMin)) })
  vis(siteA, A[1], 'Hasan Basri', 'Pengiriman bahan baku', 'Pak Rudi (Gudang)', 'B 9123 KXA', 95)
  vis(siteA, A[1], 'Lina Marlina', 'Audit stok bulanan', 'Bu Sari (Finance)', undefined, 240, 60)
  vis(siteA, A[0], 'Teknisi PLN', 'Pemeriksaan gardu', 'Pak Rudi (Gudang)', 'B 1772 PLN', 30)
  vis(siteB, B[1], 'Michael Tan', 'Rapat vendor', 'Bu Nadia (Procurement)', 'B 1 MTN', 70)

  // Notification rules + delivery log.
  const rule = (site: Site, event_type: string, channel: 'email' | 'whatsapp', recipient: string) =>
    w.rules.push({ id: uid(), site_id: site.id, event_type, channel, recipient, enabled: true, created_at: created })
  rule(siteA, 'sos', 'whatsapp', '6281234567890')
  rule(siteA, 'sos', 'email', 'ops@logistiknusantara.co.id')
  rule(siteA, 'absent', 'email', 'hr@logistiknusantara.co.id')
  rule(siteA, 'incident_high', 'whatsapp', '6281234567890')
  rule(siteA, 'sla_missed', 'email', 'ops@logistiknusantara.co.id')
  rule(siteB, 'sos', 'whatsapp', '6289876543210')

  const logEntry = (event_type: string, channel: 'email' | 'whatsapp', recipient: string, subject: string, status: NotificationLogEntry['status'], minAgo: number, err?: string) =>
    w.log.push({ id: uid(), site_id: siteA.id, event_type, channel, recipient, subject, body: subject, status, attempts: status === 'failed' ? 3 : 1, last_error: err, created_at: iso(minutes(now, -minAgo)), sent_at: status === 'sent' ? iso(minutes(now, -minAgo + 1)) : undefined })
  logEntry('sos', 'whatsapp', '6281234567890', '[SOS] Rina Wulandari di Gudang Cikarang', 'sent', 3)
  logEntry('sos', 'email', 'ops@logistiknusantara.co.id', '[SOS] Rina Wulandari di Gudang Cikarang', 'queued', 3)
  logEntry('incident_high', 'whatsapp', '6281234567890', '[Insiden HIGH] trespassing di Gudang Cikarang', 'sent', 90)
  logEntry('absent', 'email', 'hr@logistiknusantara.co.id', '[Absen] Dedi Kurniawan tidak hadir di Gudang Cikarang', 'failed', 400, 'SMTP is not configured')

  w.backups.push(
    { id: uid(), object_key: 'postgres/securepatrol-daily.dump', size_bytes: 18_400_000, status: 'succeeded', trigger: 'scheduled', created_at: iso(wib(0, 2)), started_at: iso(wib(0, 2)), finished_at: iso(minutes(wib(0, 2), 1)), keycloak_object_key: 'postgres/keycloak-daily.dump', keycloak_size_bytes: 2_300_000 },
    { id: uid(), object_key: 'postgres/securepatrol-prev.dump', size_bytes: 18_100_000, status: 'succeeded', trigger: 'scheduled', created_at: iso(wib(-1, 2)), started_at: iso(wib(-1, 2)), finished_at: iso(minutes(wib(-1, 2), 1)) },
  )
  w.channels.whatsapp = { api_url: 'https://graph.facebook.com/v18.0', token: '********', phone_number_id: '109876543210', template_name: 'garda_alert', template_lang: 'id' }

  // Install-wide ops alerts (platform-admin).
  w.opsRules.push(
    { id: uid(), event_type: 'ops_backup_failed', channel: 'email', recipient: 'noc@operator.example', enabled: true, created_by: 'demo/platform-admin', created_at: created },
    { id: uid(), event_type: 'ops_sos_undelivered', channel: 'whatsapp', recipient: '6281100002222', enabled: true, created_by: 'demo/platform-admin', created_at: created },
    { id: uid(), event_type: 'ops_backup_stale', channel: 'email', recipient: 'noc@operator.example', enabled: false, created_by: 'demo/platform-admin', created_at: created },
  )
  w.opsLog.push(
    { id: uid(), event_type: 'ops_backup_failed', channel: 'email', recipient: 'noc@operator.example', subject: '[Ops] Backup failed', body: 'pg_dump exited with status 1', status: 'failed', attempts: 5, last_error: 'timeout', created_at: iso(minutes(now, -60 * 26)) },
    { id: uid(), event_type: 'ops_sos_undelivered', channel: 'whatsapp', recipient: '6281100002222', subject: '[Ops] SOS not delivered', body: 'Site has no SOS rule', status: 'sent', attempts: 1, created_at: iso(minutes(now, -60 * 50)), sent_at: iso(minutes(now, -60 * 50 + 1)) },
  )

  // Audit log (GET /audit).
  const audit = (minAgo: number, actor: string, roles: string[], method: string, route: string, resourceType: string, resourceId: string | undefined, extra: Partial<AuditRow> = {}) =>
    w.audit.push({
      id: uid(), user_id: `demo/${actor}`, realm: 'demo', actor, roles, action: `${method} ${route}`, method, route, resource_type: resourceType, resource_id: resourceId,
      ip: '10.0.0.24', user_agent: 'Mozilla/5.0 (Windows NT 10.0) Chrome/141', status_code: method === 'POST' ? 201 : 200, created_at: iso(minutes(now, -minAgo)), request_id: uid().slice(-12), ...extra,
    })
  const g = A[7]
  audit(60 * 30, 'admin.rina', ['admin'], 'PUT', '/api/v1/guards/:id', 'guards', g.id, {
    params: { id: g.id },
    before: { name: g.name, employee_id: 'GS-0180', status: 'inactive', role: 'guard', site_id: siteA.id },
    after: { name: g.name, employee_id: g.employee_id, status: 'active', role: 'guard', site_id: siteA.id },
    summary: { name: g.name, employee_id: g.employee_id, status: 'active', role: 'guard', site_id: siteA.id },
  })
  audit(60 * 20, 'admin.rina', ['admin'], 'PUT', '/api/v1/guards/:id/pin', 'guards', g.id, { params: { id: g.id }, summary: { pin: '[redacted]' } })
  const absent = w.attendance.find((a) => a.status === 'absent')
  if (absent) {
    audit(60 * 6, 'admin.rina', ['admin'], 'PUT', '/api/v1/attendance/shifts/:shiftId/override', 'attendance.shifts', absent.shift_id, {
      params: { shiftId: absent.shift_id },
      before: { status: 'absent', clock_in_at: null, override_reason: null },
      after: { status: 'present', clock_in_at: absent.created_at, override_reason: 'Tablet rusak, absen manual di buku' },
      summary: { status: 'present', reason: 'Tablet rusak, absen manual di buku' },
    })
  }
  audit(180, 'spv.budi', ['supervisor'], 'POST', '/api/v1/incidents/:id/amendments', 'incidents', w.incidents[1]?.id, {
    params: { id: w.incidents[1]?.id },
    summary: { changes: { severity: 'critical' }, reason: 'Nilai kerugian dikonfirmasi gudang' },
  })
  audit(45, 'admin.rina', ['admin'], 'POST', '/api/v1/notifications/rules', 'notifications.rules', undefined, { summary: { site_id: siteA.id, event_type: 'sla_missed', channel: 'email', recipient: 'ops@logistiknusantara.co.id' } })
  audit(20, 'admin.rina', ['admin'], 'PUT', '/api/v1/devices/:id', 'devices', w.devices[0].id, {
    params: { id: w.devices[0].id },
    before: { status: 'pending', site_id: null },
    after: { status: 'active', site_id: siteA.id },
    summary: { status: 'active', site_id: siteA.id },
  })
  audit(8, 'spv.budi', ['supervisor'], 'POST', '/api/v1/sos/:id/acknowledge', 'sos', w.sos[2]?.id, { params: { id: w.sos[2]?.id } })
  audit(4, 'admin.rina', ['admin'], 'PUT', '/api/v1/guards/:id', 'guards', A[2].id, { params: { id: A[2].id }, status_code: 409, summary: { name: A[2].name, employee_id: 'GS-0101' } })

  return w
}
