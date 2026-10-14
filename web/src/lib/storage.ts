/** localStorage wrappers that never throw (private mode, blocked storage). */

export function readStored(key: string): string | null {
  try {
    return window.localStorage.getItem(key)
  } catch {
    return null
  }
}

export function writeStored(key: string, value: string | null): void {
  try {
    if (value === null) window.localStorage.removeItem(key)
    else window.localStorage.setItem(key, value)
  } catch {
    // ignore — preference is simply not remembered
  }
}

export const STORAGE_KEYS = {
  realm: 'sp.realm',
  lang: 'sp.lang',
  site: 'sp.site',
  demoRole: 'sp.demo.role',
  /** "1" = SOS alarm sound muted in this browser. */
  sosMuted: 'sp.sos.muted',
  /** Epoch ms of the last user input in any tab (idle logout). */
  lastActivity: 'sp.lastActivity',
  /** "1" = the dashboard hint to turn on push notifications was dismissed. */
  pushNudgeDismissed: 'sp.push.nudgeDismissed',
} as const
