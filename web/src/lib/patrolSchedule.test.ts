import { describe, expect, it } from 'vitest'
import { translate } from '../i18n/context'
import { describeSchedule, firstScheduleError, toDrafts, toSchedule } from '../pages/patrol/scheduleDrafts'
import { crossesMidnight, dueOffsets, patrolsPerWeek, plannedPatrols, validateSchedule, weekdaysLabel, windowSpan } from './patrolSchedule'
import type { RouteSchedule } from '../types'

const win = (s: Partial<RouteSchedule>): RouteSchedule => ({ weekdays: [1, 2, 3, 4, 5], start_time: '22:00', end_time: '06:00', every_minutes: 120, grace_minutes: 15, active: true, ...s })

describe('patrol schedule windows (D-038)', () => {
  it('validates like the server, in the same order', () => {
    expect(validateSchedule(win({}))).toBeNull()
    expect(validateSchedule(win({ start_time: '25:00' }))?.code).toBe('invalid_start_time')
    expect(validateSchedule(win({ weekdays: [] }))?.code).toBe('invalid_weekdays')
    expect(validateSchedule(win({ weekdays: [0, 8] }))?.code).toBe('invalid_weekdays')
    expect(validateSchedule(win({ grace_minutes: 181 }))?.code).toBe('invalid_grace')
    expect(validateSchedule(win({ end_time: '6:00' }))?.code).toBe('invalid_end_time')
    expect(validateSchedule(win({ every_minutes: 10 }))?.code).toBe('invalid_every')
    expect(validateSchedule(win({ every_minutes: 721 }))?.code).toBe('invalid_every')
    expect(validateSchedule(win({ end_time: null }))).toEqual({ code: 'end_time_required', field: 'end_time' })
    expect(validateSchedule(win({ every_minutes: 20, grace_minutes: 15 }))?.code).toBe('grace_too_long')
    // Once: no end time needed, grace not limited by an interval.
    expect(validateSchedule(win({ every_minutes: null, end_time: null, grace_minutes: 90 }))).toBeNull()
  })

  it('plans patrols strictly before the end, crossing midnight', () => {
    expect(windowSpan(win({}))).toBe(480)
    expect(dueOffsets(win({})).map((m) => m / 60)).toEqual([22, 24, 26, 28])
    expect(crossesMidnight(win({}))).toBe(true)
    expect(dueOffsets(win({ start_time: '08:00', end_time: '17:00', every_minutes: 180 }))).toEqual([480, 660, 840])
    expect(crossesMidnight(win({ start_time: '08:00', end_time: '17:00', every_minutes: 180 }))).toBe(false)
    // equal start and end = 24 h
    expect(dueOffsets(win({ start_time: '06:00', end_time: '06:00', every_minutes: 360 }))).toHaveLength(4)
    expect(dueOffsets(win({ every_minutes: null }))).toEqual([22 * 60])
    expect(patrolsPerWeek([win({}), win({ active: false })])).toBe(20)
  })

  it('formats weekday ranges', () => {
    expect(weekdaysLabel([1, 2, 3, 4, 5], 'en', 'every day')).toBe('Mon–Fri')
    expect(weekdaysLabel([6, 7], 'en', 'every day')).toBe('Sat, Sun')
    expect(weekdaysLabel([1, 3, 4, 5], 'en', 'every day')).toBe('Mon, Wed–Fri')
    expect(weekdaysLabel([7, 1, 2, 3, 4, 5, 6], 'en', 'every day')).toBe('every day')
  })

  it('describes a window in plain words', () => {
    const t = (k: Parameters<typeof translate>[1], v?: Record<string, string | number>) => translate('en', k, v)
    expect(describeSchedule(win({}), t, 'en')).toBe('Every 2 h from 22:00 to 06:00, Mon–Fri, 15 min grace → 4 patrols/night')
    expect(describeSchedule(win({ every_minutes: null, start_time: '14:00', weekdays: [1, 2, 3, 4, 5, 6, 7] }), t, 'en')).toBe('Once at 14:00, every day, 15 min grace')
    const ti = (k: Parameters<typeof translate>[1], v?: Record<string, string | number>) => translate('id', k, v)
    expect(describeSchedule(win({ start_time: '08:00', end_time: '17:00', every_minutes: 180 }), ti, 'id')).toBe('Setiap 3 jam dari 08:00 sampai 17:00, Sen–Jum, toleransi 15 mnt → 3 patroli/hari')
  })

  it('lists the next planned patrols in the site time zone', () => {
    // Monday 2026-10-12 20:00 WIB = 13:00 UTC.
    const from = Date.parse('2026-10-12T13:00:00Z')
    const out = plannedPatrols([win({ weekdays: [1] })], 'Asia/Jakarta', from, 24 * 3600_000)
    expect(out.map((p) => p.due_at)).toEqual(['2026-10-12T15:00:00.000Z', '2026-10-12T17:00:00.000Z', '2026-10-12T19:00:00.000Z', '2026-10-12T21:00:00.000Z'])
    expect(out[0].grace_until).toBe('2026-10-12T15:15:00.000Z')
    expect(out[0].window_date).toBe('2026-10-12')
  })

  it('round-trips drafts and finds the first invalid window', () => {
    const drafts = toDrafts([win({ id: 'a' }), win({ id: 'b', every_minutes: null, end_time: null })])
    expect(drafts.map((d) => d.mode)).toEqual(['repeat', 'once'])
    expect(toSchedule(drafts[1])).toMatchObject({ every_minutes: null, end_time: null })
    expect(firstScheduleError(drafts)).toBeNull()
    expect(firstScheduleError([drafts[0], { ...drafts[1], weekdays: [] }])).toEqual({ index: 1, error: { code: 'invalid_weekdays', field: 'weekdays' } })
    expect(firstScheduleError([{ ...drafts[0], every: '' }])?.error.code).toBe('invalid_every')
  })
})
