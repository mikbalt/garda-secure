import { describe, expect, it } from 'vitest'
import { en } from './en'
import { id } from './id'
import { translate } from './context'

const placeholders = (s: string) => Array.from(s.matchAll(/\{(\w+)\}/g), (m) => m[1]).sort()

/** Strings that are legitimately the same in both languages (names, codes, loanwords). */
const SAME_OK = /^(GARDA|Demo|SOS|SOS — \{site\}|Supervisor|WhatsApp|E-mail|Email|CSV|PIN|QR|ID|OK|Status|Shift|Log|Backup|Server|Tablet|Patroli|Total|Template|Token|Host|Port|URL|API.*|.*:\s*\{.*\}|\{.*\}|[\d\s.,:/–-]+|0812 3456 7890|.*@.*)$/i

describe('i18n EN/ID parity', () => {
  it('has the same keys in both dictionaries', () => {
    expect(Object.keys(id).sort()).toEqual(Object.keys(en).sort())
  })

  it('has no empty strings', () => {
    for (const [k, v] of Object.entries(en)) expect(v.trim(), `en ${k}`).not.toBe('')
    for (const [k, v] of Object.entries(id)) expect(v.trim(), `id ${k}`).not.toBe('')
  })

  it('uses the same placeholders in both languages', () => {
    for (const k of Object.keys(en) as (keyof typeof en)[]) {
      expect(placeholders(id[k]), k).toEqual(placeholders(en[k]))
    }
  })

  it('translates the new hardening strings (no English copied into ID)', () => {
    const fresh = Object.keys(en).filter((k) => /^(plat|aud|flag|idle|sos\.(notify|sound|more|window))\./.test(k)) as (keyof typeof en)[]
    expect(fresh.length).toBeGreaterThan(50)
    for (const k of fresh) {
      if (SAME_OK.test(en[k])) continue
      expect(id[k], k).not.toBe(en[k])
    }
  })

  it('fills placeholders', () => {
    expect(translate('id', 'idle.countdown', { s: 9 })).toBe('Keluar dalam 9 detik')
    expect(translate('en', 'aud.page', { n: 2 })).toBe('Page 2')
  })
})
