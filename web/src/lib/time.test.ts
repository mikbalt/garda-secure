import { describe, expect, it } from 'vitest'
import {
  addDays,
  daysBetween,
  formatTime,
  isoToLocalInput,
  isoWeekday,
  localDateOf,
  localInputToIso,
  localTimeOf,
  startOfWeek,
  zonedToUtcIso,
} from './time'

describe('site time zone maths', () => {
  it('puts the Jakarta day boundary at 17:00 UTC', () => {
    // 2026-10-11 16:59:59Z = 23:59:59 WIB on the 11th; 17:00Z = 00:00 WIB on the 12th.
    expect(localDateOf('2026-10-11T16:59:59Z', 'Asia/Jakarta')).toBe('2026-10-11')
    expect(localDateOf('2026-10-11T17:00:00Z', 'Asia/Jakarta')).toBe('2026-10-12')
    expect(localTimeOf('2026-10-11T17:00:00Z', 'Asia/Jakarta')).toBe('00:00')
  })

  it('handles other Indonesian zones (WITA, WIT)', () => {
    expect(localDateOf('2026-10-11T15:59:00Z', 'Asia/Makassar')).toBe('2026-10-11')
    expect(localDateOf('2026-10-11T16:00:00Z', 'Asia/Makassar')).toBe('2026-10-12')
    expect(localDateOf('2026-10-11T15:00:00Z', 'Asia/Jayapura')).toBe('2026-10-12')
  })

  it('converts a site-local wall time to UTC', () => {
    expect(zonedToUtcIso('2026-10-12', '00:00', 'Asia/Jakarta')).toBe('2026-10-11T17:00:00Z')
    expect(zonedToUtcIso('2026-10-12', '07:30', 'Asia/Jakarta')).toBe('2026-10-12T00:30:00Z')
    expect(zonedToUtcIso('2026-01-01', '00:00', 'UTC')).toBe('2026-01-01T00:00:00Z')
  })

  it('handles DST transitions', () => {
    // Europe/Berlin switches to CEST on 2026-03-29 (02:00 → 03:00).
    expect(zonedToUtcIso('2026-03-28', '12:00', 'Europe/Berlin')).toBe('2026-03-28T11:00:00Z')
    expect(zonedToUtcIso('2026-03-29', '12:00', 'Europe/Berlin')).toBe('2026-03-29T10:00:00Z')
    expect(localDateOf('2026-03-28T23:30:00Z', 'Europe/Berlin')).toBe('2026-03-29')
  })

  it('falls back to UTC for an unknown zone', () => {
    expect(localDateOf('2026-10-11T23:00:00Z', 'Mars/Olympus')).toBe('2026-10-11')
    expect(zonedToUtcIso('2026-10-12', '08:00', 'Mars/Olympus')).toBe('2026-10-12T08:00:00Z')
  })

  it('round-trips datetime-local inputs in the site zone', () => {
    const iso = '2026-10-11T23:15:00Z'
    const local = isoToLocalInput(iso, 'Asia/Jakarta')
    expect(local).toBe('2026-10-12T06:15')
    expect(localInputToIso(local, 'Asia/Jakarta')).toBe(iso)
    expect(localInputToIso('', 'Asia/Jakarta')).toBeUndefined()
    expect(isoToLocalInput(undefined, 'Asia/Jakarta')).toBe('')
  })

  it('formats empty times as a dash', () => {
    expect(formatTime(undefined, 'Asia/Jakarta')).toBe('—')
    expect(formatTime('2026-10-11T17:05:00Z', 'Asia/Jakarta')).toBe('00:05')
  })
})

describe('calendar dates', () => {
  it('adds days across month and year ends', () => {
    expect(addDays('2026-10-31', 1)).toBe('2026-11-01')
    expect(addDays('2026-12-31', 1)).toBe('2027-01-01')
    expect(addDays('2028-03-01', -1)).toBe('2028-02-29')
  })

  it('knows ISO weekdays and week starts', () => {
    expect(isoWeekday('2026-10-12')).toBe(1) // Monday
    expect(isoWeekday('2026-10-18')).toBe(7) // Sunday
    expect(startOfWeek('2026-10-18')).toBe('2026-10-12')
  })

  it('counts days inclusively', () => {
    expect(daysBetween('2026-10-01', '2026-10-01')).toBe(1)
    expect(daysBetween('2026-10-01', '2026-10-31')).toBe(31)
  })
})
