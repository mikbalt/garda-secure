import { describe, expect, it } from 'vitest'
import { flagRecordLink } from './flags'

const base = { record_id: 'r1', event_at: '2026-03-10T23:30:00Z' }

describe('flagRecordLink', () => {
  it('opens the attendance day of the record in the site time zone', () => {
    expect(flagRecordLink({ ...base, kind: 'attendance' }, 'Asia/Jakarta')).toBe('/attendance?date=2026-03-11')
    expect(flagRecordLink({ ...base, kind: 'attendance' }, 'UTC')).toBe('/attendance?date=2026-03-10')
  })

  it('opens the matching tab of the incidents and patrol pages', () => {
    expect(flagRecordLink({ ...base, kind: 'incident' }, 'UTC')).toBe('/incidents?tab=incidents')
    expect(flagRecordLink({ ...base, kind: 'sos' }, 'UTC')).toBe('/incidents?tab=sos')
    expect(flagRecordLink({ ...base, kind: 'defect' }, 'UTC')).toBe('/incidents?tab=defects')
    expect(flagRecordLink({ ...base, kind: 'visitor' }, 'UTC')).toBe('/incidents?tab=visitors')
    expect(flagRecordLink({ ...base, kind: 'scan' }, 'UTC')).toBe('/patrol?tab=sessions')
    expect(flagRecordLink({ ...base, kind: 'session' }, 'UTC')).toBe('/patrol?tab=sessions')
  })
})
