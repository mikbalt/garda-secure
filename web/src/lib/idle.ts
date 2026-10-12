/** Idle-logout state machine (WEB-4), kept pure for unit tests. */

export type IdlePhase = { kind: 'active' } | { kind: 'warning'; secondsLeft: number } | { kind: 'expired' }

/** Warning length: 60 s, or half the timeout when that is shorter. */
export function warningMs(timeoutMs: number): number {
  return Math.min(60_000, Math.floor(timeoutMs / 2))
}

export function idlePhase(now: number, lastActivity: number, timeoutMs: number): IdlePhase {
  const idle = now - lastActivity
  if (idle >= timeoutMs) return { kind: 'expired' }
  const warn = warningMs(timeoutMs)
  if (idle >= timeoutMs - warn) return { kind: 'warning', secondsLeft: Math.ceil((timeoutMs - idle) / 1000) }
  return { kind: 'active' }
}

export function samePhase(a: IdlePhase, b: IdlePhase): boolean {
  if (a.kind !== b.kind) return false
  return a.kind !== 'warning' || (b.kind === 'warning' && a.secondsLeft === b.secondsLeft)
}
