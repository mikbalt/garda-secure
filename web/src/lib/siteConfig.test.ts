import { describe, expect, it } from 'vitest'
import { siteConfigForSave } from './siteConfig'
import type { SiteConfig } from '../types'

const numbers = {
  clock_window_minutes: '20',
  late_grace_minutes: '5',
  early_out_grace_minutes: '5',
  absent_after_minutes: '30',
  idle_lock_minutes: '5',
}

describe('siteConfigForSave', () => {
  it('keeps a stored setting the form does not edit', () => {
    const stored = { ...numbersAsConfig(), language: 'id', scan_radius_m: 0 } as SiteConfig
    const saved = siteConfigForSave(stored, 'en', numbers)
    expect(saved.scan_radius_m).toBe(0)
    expect(saved.language).toBe('en')
    expect(saved.clock_window_minutes).toBe(20)
  })

  it('sends only the form fields for a new site', () => {
    expect(siteConfigForSave(null, 'id', numbers)).toEqual({ ...numbersAsConfig(), clock_window_minutes: 20, language: 'id' })
  })
})

function numbersAsConfig() {
  return { clock_window_minutes: 15, late_grace_minutes: 5, early_out_grace_minutes: 5, absent_after_minutes: 30, idle_lock_minutes: 5 }
}