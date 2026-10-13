import { describe, expect, it } from 'vitest'
import { aggregateRecap, emptyCounts, recapTotals, type RecapShift } from './recap'
import type { MonthlyRecapRow } from '../types'

const shift = (start: string, status: RecapShift['status'], early_out = false, hours = 0): RecapShift => ({ guard_id: 'g', start, status, early_out, hours })

describe('aggregateRecap (per guard-day)', () => {
  it('derives the day status from all shifts of the day', () => {
    const c = aggregateRecap(
      [
        shift('2026-10-01T00:00:00Z', 'late', false, 7.5), // 07:00 WIB
        shift('2026-10-01T12:00:00Z', 'present', false, 8), // 19:00 WIB
        shift('2026-10-02T00:00:00Z', 'present', true, 6),
        shift('2026-10-02T12:00:00Z', 'absent'),
        shift('2026-10-03T00:00:00Z', 'present', false, 8),
        shift('2026-10-03T12:00:00Z', 'pending'),
        shift('2026-10-04T00:00:00Z', 'pending'),
      ],
      'Asia/Jakarta',
    ).get('g')!
    expect(c).toMatchObject({
      scheduled_days: 4, days_late: 1, days_absent: 1, days_present: 1, days_upcoming: 1, early_out_days: 1,
      scheduled_shifts: 7, present: 3, late: 1, absent: 1, pending: 2, early_out: 1, total_hours: 29.5,
    })
  })

  it('keys days by the shift start in the site time zone', () => {
    const shifts = [shift('2026-10-10T15:30:00Z', 'present'), shift('2026-10-10T16:30:00Z', 'absent')]
    expect(aggregateRecap(shifts, 'Asia/Jakarta').get('g')!.scheduled_days).toBe(1) // 22:30, 23:30
    expect(aggregateRecap(shifts, 'Asia/Makassar').get('g')!.scheduled_days).toBe(2) // 23:30, 00:30
    expect(aggregateRecap(shifts, 'Asia/Jayapura').get('g')!.scheduled_days).toBe(1) // 00:30, 01:30
  })
})

describe('recapTotals', () => {
  it('sums rows and weights patrol completion by sessions', () => {
    const row = (id: string, p: Partial<MonthlyRecapRow>): MonthlyRecapRow => ({ guard_id: id, guard_name: id, ...emptyCounts(), ...p })
    const t = recapTotals([
      row('a', { scheduled_days: 20, days_present: 19, days_absent: 1, total_hours: 160.4, patrol_sessions: 3, patrol_completion_pct: 100 }),
      row('b', { scheduled_days: 5, days_upcoming: 5, total_hours: 64.3, patrol_sessions: 1, patrol_completion_pct: 60 }),
    ])
    expect(t).toMatchObject({ guards: 2, scheduled_days: 25, days_present: 19, days_absent: 1, days_upcoming: 5, total_hours: 224.7, patrol_sessions: 4, patrol_completion_pct: 90 })
  })
})
