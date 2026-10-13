import type { Key } from '../../i18n/context'
import { DEFAULT_GRACE, MAX_SCHEDULES, crossesMidnight, dueOffsets, validateSchedule, weekdaysLabel, type ScheduleError } from '../../lib/patrolSchedule'
import type { Lang } from '../../lib/time'
import { fmtDuration } from './format'
import type { RouteSchedule } from '../../types'

/** Editable schedule window (inputs kept as text until saved). */
export interface ScheduleDraft {
  key: string
  weekdays: number[]
  start: string
  end: string
  mode: 'repeat' | 'once'
  every: string
  grace: string
  active: boolean
}

let seq = 0
export function newDraft(): ScheduleDraft {
  return { key: `sch-${seq++}`, weekdays: [1, 2, 3, 4, 5, 6, 7], start: '22:00', end: '06:00', mode: 'repeat', every: '120', grace: String(DEFAULT_GRACE), active: true }
}

export function toDrafts(list: RouteSchedule[]): ScheduleDraft[] {
  return list.map((s) => ({
    key: s.id ?? `sch-${seq++}`,
    weekdays: [...s.weekdays].sort((a, b) => a - b),
    start: s.start_time,
    end: s.end_time ?? '',
    mode: s.every_minutes ? 'repeat' : 'once',
    every: s.every_minutes ? String(s.every_minutes) : '120',
    grace: String(s.grace_minutes ?? DEFAULT_GRACE),
    active: s.active,
  }))
}

const int = (v: string): number => (v.trim() === '' ? NaN : Number(v))

/** Draft → API body of PUT /patrol/routes/:id/schedules. */
export function toSchedule(d: ScheduleDraft): RouteSchedule {
  const repeat = d.mode === 'repeat'
  return {
    weekdays: [...d.weekdays].sort((a, b) => a - b),
    start_time: d.start,
    end_time: repeat ? d.end || null : null,
    every_minutes: repeat ? int(d.every) : null,
    grace_minutes: int(d.grace),
    active: d.active,
  }
}

/** First invalid window (index + error), checked like the server. */
export function firstScheduleError(drafts: ScheduleDraft[]): { index: number; error: ScheduleError } | null {
  if (drafts.length > MAX_SCHEDULES) return { index: -1, error: { code: 'too_many_schedules', field: 'schedules' } }
  for (let i = 0; i < drafts.length; i++) {
    const e = validateSchedule(toSchedule(drafts[i]))
    if (e) return { index: i, error: e }
  }
  return null
}

type T = (key: Key, vars?: Record<string, string | number>) => string

/** "Every 2 h from 22:00 to 06:00, Mon–Fri, 15 min grace → 4 patrols/night". */
export function describeSchedule(s: RouteSchedule, t: T, lang: Lang): string {
  const days = weekdaysLabel(s.weekdays, lang, t('p2.sch.everyDay'))
  const grace = t('p2.sch.graceText', { m: s.grace_minutes })
  if (!s.every_minutes) return t('p2.sch.previewOnce', { start: s.start_time, days, grace })
  const n = dueOffsets(s).length
  return t('p2.sch.previewRepeat', {
    every: fmtDuration(s.every_minutes * 60_000, t),
    start: s.start_time,
    end: s.end_time ?? '—',
    days,
    grace,
    count: n === 1 ? t('p2.sch.countOne') : t('p2.sch.count', { n }),
    per: crossesMidnight(s) ? t('p2.sch.perNight') : t('p2.sch.perDay'),
  })
}
