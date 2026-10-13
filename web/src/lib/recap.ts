import { localDateOf } from './time'
import type { MonthlyRecapRow, MonthlyRecapTotals, RecapCounts } from '../types'

/**
 * Monthly recap per guard-day (FR-REP-05) — port of backend
 * repository/recap.go, used by the demo API. A guard-day is a calendar day in
 * the site's time zone keyed by the day a shift starts; its status is absent
 * if any shift is absent, else late if any is late, else present if any is
 * present, else upcoming (all shifts still pending).
 */
export interface RecapShift {
  guard_id: string
  start: string
  status: 'present' | 'late' | 'absent' | 'pending'
  early_out: boolean
  hours: number
}

const RANK = { pending: 0, present: 1, late: 2, absent: 3 } as const
const round1 = (n: number) => Math.round(n * 10) / 10

export const emptyCounts = (): RecapCounts => ({
  scheduled_days: 0, days_present: 0, days_late: 0, days_absent: 0, days_upcoming: 0, early_out_days: 0,
  scheduled_shifts: 0, present: 0, late: 0, absent: 0, early_out: 0, pending: 0,
  total_hours: 0, patrol_sessions: 0, patrol_completion_pct: 0,
})

/** Shift and guard-day counts per guard id. */
export function aggregateRecap(shifts: RecapShift[], tz: string): Map<string, RecapCounts> {
  const out = new Map<string, RecapCounts>()
  const days = new Map<string, { guard: string; rank: number; earlyOut: boolean }>()
  for (const s of shifts) {
    const c = out.get(s.guard_id) ?? emptyCounts()
    out.set(s.guard_id, c)
    c.scheduled_shifts++
    c[s.status]++
    if (s.early_out) c.early_out++
    c.total_hours += s.hours
    const key = `${s.guard_id}|${localDateOf(s.start, tz)}`
    const d = days.get(key) ?? { guard: s.guard_id, rank: 0, earlyOut: false }
    d.rank = Math.max(d.rank, RANK[s.status])
    d.earlyOut ||= s.early_out
    days.set(key, d)
  }
  for (const d of days.values()) {
    const c = out.get(d.guard)!
    c.scheduled_days++
    if (d.rank === 3) c.days_absent++
    else if (d.rank === 2) c.days_late++
    else if (d.rank === 1) c.days_present++
    else c.days_upcoming++
    if (d.earlyOut) c.early_out_days++
  }
  for (const c of out.values()) c.total_hours = round1(c.total_hours)
  return out
}

const SUMMED = [
  'scheduled_days', 'days_present', 'days_late', 'days_absent', 'days_upcoming', 'early_out_days',
  'scheduled_shifts', 'present', 'late', 'absent', 'early_out', 'pending', 'total_hours', 'patrol_sessions',
] as const

/** Site totals; patrol completion is the session-weighted average. */
export function recapTotals(rows: MonthlyRecapRow[]): MonthlyRecapTotals {
  const t: MonthlyRecapTotals = { guards: rows.length, ...emptyCounts() }
  let pct = 0
  for (const r of rows) {
    for (const k of SUMMED) t[k] += r[k]
    pct += r.patrol_completion_pct * r.patrol_sessions
  }
  t.total_hours = round1(t.total_hours)
  t.patrol_completion_pct = t.patrol_sessions ? round1(pct / t.patrol_sessions) : 0
  return t
}
