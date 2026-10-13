import { describe, expect, it } from 'vitest'
import { usersEn } from './users.en'
import { usersId } from './users.id'
import { en } from './en'

/** Strings that may legitimately read the same in both languages. */
const SAME_OK = /^(\{.*\}|Status|OTP|E-mail|Tablet: .*)$/

describe('people & access strings (EN/ID)', () => {
  it('are merged into the main dictionaries', () => {
    for (const k of Object.keys(usersEn)) expect(en[k as keyof typeof en], k).toBe(usersEn[k as keyof typeof usersEn])
  })

  it('are really translated (no English copied into Indonesian)', () => {
    for (const k of Object.keys(usersEn) as (keyof typeof usersEn)[]) {
      if (SAME_OK.test(usersEn[k])) continue
      expect(usersId[k], k).not.toBe(usersEn[k])
    }
  })

  it('explain every role, health issue and required action the UI can show', () => {
    for (const r of ['admin', 'supervisor', 'hr', 'client-readonly', 'platform-admin']) expect(usersEn).toHaveProperty(`acc.role.${r}`)
    for (const i of ['noBeacon', 'stale', 'sosPending', 'outboxPaused', 'outboxFailed', 'outboxStuck', 'clockSkew', 'rosterOld', 'rosterBlocked', 'dbKeyReset', 'errors']) {
      expect(usersEn).toHaveProperty(`acc.health.issue.${i}`)
      expect(usersEn).toHaveProperty(`acc.health.fix.${i}`)
    }
    for (const s of ['pending', 'active', 'revoked']) expect(usersEn).toHaveProperty(`acc.dev.status.${s}`)
  })
})
