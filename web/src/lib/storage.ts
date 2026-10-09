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
} as const
