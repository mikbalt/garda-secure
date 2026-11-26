import { describe, expect, it } from 'vitest'
import { flagRecordLink } from './flags'

const base = { site_id: 'site-b', event_at: '2026-03-10T23:30:00Z' }
const link = (kind: Parameters<typeof flagRecordLink>[0]['kind'], tz = 'UTC') => flagRecordLink({ ...base, kind }, tz)

describe('flagRecordLink', () => {
  it('opens the attendance day of the record in the site time zone', () => {
    expect(link('attendance', 'Asia/Jakarta')).toBe('/attendance?site=site-b&date=2026-03-11')
    expect(link('attendance')).toBe('/attendance?site=site-b&date=2026-03-10')
  })

  it('opens the tab, the day and the site of scans, sessions and incidents', () => {
    expect(link('incident')).toBe('/incidents?tab=incidents&site=site-b&date=2026-03-10')
    expect(link('scan', 'Asia/Jakarta')).toBe('/patrol?tab=sessions&site=site-b&date=2026-03-11')
    expect(link('session')).toBe('/patrol?tab=sessions&site=site-b&date=2026-03-10')
  })

  it('opens the right tab and site for SOS, defects and visitors', () => {
    expect(link('sos')).toBe('/incidents?tab=sos&site=site-b')
    expect(link('defect')).toBe('/incidents?tab=defects&site=site-b')
    expect(link('visitor')).toBe('/incidents?tab=visitors&site=site-b')
  })
})
