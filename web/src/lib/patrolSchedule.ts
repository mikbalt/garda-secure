/**
 * Patrol schedule windows (D-038) — pure helpers mirroring the backend
 * (service/schedule.go): validation, patrols per window, a human preview and
 * the next planned patrol times in the site's time zone.
 */
import { addDays, isoWeekday, localDateOf, weekdayName, zonedToUtcIso, type Lang } from './time'
import type { RouteSchedule } from '../types'

export const MAX_SCHEDULES = 20
export const MIN_EVERY = 15
export const MAX_EVERY = 720
export const MAX_GRACE = 180
export const DEFAULT_GRACE = 15

const CLOCK = /^([01]\d|2[0-3]):([0-5]\d)$/

export function parseClock(v: string | null | undefined): number | null {
  const m = CLOCK.exec(v ?? '')
  return m ? Number(m[1]) * 60 + Number(m[2]) : null
}

export function clock(min: number): string {
  const m = ((min % 1440) + 1440) % 1440
  return `${String(Math.floor(m / 60)).padStart(2, '0')}:${String(m % 60).padStart(2, '0')}`
}

export type ScheduleErrorCode =
  | 'invalid_weekdays'
  | 'invalid_start_time'
  | 'invalid_end_time'
  | 'end_time_required'
  | 'invalid_every'
  | 'invalid_grace'
  | 'grace_too_long'
  | 'too_many_schedules'

export interface ScheduleError {
  code: ScheduleErrorCode
  field: 'weekdays' | 'start_time' | 'end_time' | 'every_minutes' | 'grace_minutes' | 'schedules'
}

/** First validation error of a window, like the server (same order of checks), or null. */
export function validateSchedule(s: RouteSchedule): ScheduleError | null {
  if (parseClock(s.start_time) === null) return { code: 'invalid_start_time', field: 'start_time' }
  const days = s.weekdays ?? []
  if (days.length === 0 || days.some((d) => !Number.isInteger(d) || d < 1 || d > 7)) return { code: 'invalid_weekdays', field: 'weekdays' }
  const grace = s.grace_minutes ?? DEFAULT_GRACE
  if (!Number.isInteger(grace) || grace < 0 || grace > MAX_GRACE) return { code: 'invalid_grace', field: 'grace_minutes' }
  if (s.end_time && parseClock(s.end_time) === null) return { code: 'invalid_end_time', field: 'end_time' }
  if (s.every_minutes != null) {
    const every = s.every_minutes
    if (!Number.isInteger(every) || every < MIN_EVERY || every > MAX_EVERY) return { code: 'invalid_every', field: 'every_minutes' }
    if (!s.end_time) return { code: 'end_time_required', field: 'end_time' }
    if (grace * 2 > every) return { code: 'grace_too_long', field: 'grace_minutes' }
  }
  return null
}

/** Window length in minutes (end ≤ start crosses midnight; equal = 24 h). */
export function windowSpan(s: Pick<RouteSchedule, 'start_time' | 'end_time'>): number {
  const start = parseClock(s.start_time)
  const end = parseClock(s.end_time)
  if (start === null || end === null) return 0
  const span = end - start
  return span <= 0 ? span + 1440 : span
}

/** Due times of one window as minutes after midnight of its start day (may exceed 1440). */
export function dueOffsets(s: Pick<RouteSchedule, 'start_time' | 'end_time' | 'every_minutes'>): number[] {
  const start = parseClock(s.start_time)
  if (start === null) return []
  if (!s.every_minutes || s.every_minutes <= 0) return [start]
  const span = windowSpan(s)
  if (span <= 0) return [start]
  const out: number[] = []
  for (let m = 0; m < span; m += s.every_minutes) out.push(start + m)
  return out
}

/** Whether the window runs past midnight. */
export function crossesMidnight(s: Pick<RouteSchedule, 'start_time' | 'end_time' | 'every_minutes'>): boolean {
  const offs = dueOffsets(s)
  return s.every_minutes != null && !!s.end_time && windowSpan(s) + (parseClock(s.start_time) ?? 0) > 1440 && offs.length > 0
}

/** "Mon–Fri", "Sat, Sun", "Mon, Wed–Fri" or the `everyDay` text. */
export function weekdaysLabel(days: number[], lang: Lang, everyDay: string): string {
  const set = Array.from(new Set(days)).filter((d) => d >= 1 && d <= 7).sort((a, b) => a - b)
  if (set.length === 7) return everyDay
  const parts: string[] = []
  for (let i = 0; i < set.length; ) {
    let j = i
    while (j + 1 < set.length && set[j + 1] === set[j] + 1) j++
    parts.push(j - i >= 2 ? `${weekdayName(set[i], lang)}–${weekdayName(set[j], lang)}` : set.slice(i, j + 1).map((d) => weekdayName(d, lang)).join(', '))
    i = j + 1
  }
  return parts.join(', ')
}

export interface PlannedPatrol {
  /** RFC 3339 UTC. */
  due_at: string
  grace_until: string
  /** Site-local date of the window's start day. */
  window_date: string
}

/**
 * Planned patrol times of the active windows within [from, from + horizon),
 * computed in the site's time zone (DST-safe via zonedToUtcIso), earliest first.
 */
export function plannedPatrols(schedules: RouteSchedule[], tz: string, from: number, horizonMs: number, max = 50): PlannedPatrol[] {
  const out: PlannedPatrol[] = []
  // Start from the site-local date of `from`, not of "now" (they differ in tests and when planning ahead).
  const today = localDateOf(new Date(from), tz)
  const to = from + horizonMs
  const days = Math.ceil(horizonMs / 86_400_000) + 1
  for (let d = -1; d <= days; d++) {
    const date = addDays(today, d)
    const wd = isoWeekday(date)
    for (const s of schedules) {
      if (!s.active || !s.weekdays.includes(wd) || validateSchedule(s)) continue
      for (const off of dueOffsets(s)) {
        const at = Date.parse(zonedToUtcIso(addDays(date, Math.floor(off / 1440)), clock(off), tz))
        if (at < from || at >= to) continue
        out.push({ due_at: new Date(at).toISOString(), grace_until: new Date(at + (s.grace_minutes ?? DEFAULT_GRACE) * 60_000).toISOString(), window_date: date })
      }
    }
  }
  out.sort((a, b) => a.due_at.localeCompare(b.due_at))
  return out.filter((p, i) => i === 0 || p.due_at !== out[i - 1].due_at).slice(0, max)
}

/** Patrols a week of the active windows plans (for the summary line). */
export function patrolsPerWeek(schedules: RouteSchedule[]): number {
  return schedules.filter((s) => s.active && !validateSchedule(s)).reduce((n, s) => n + dueOffsets(s).length * new Set(s.weekdays).size, 0)
}
