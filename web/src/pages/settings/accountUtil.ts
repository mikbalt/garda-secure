/** Helpers for tablet (device) accounts, D-041. */

/**
 * Port of the API's `kcadmin.Slug`: a-z and 0-9 are kept, whitespace and
 * `- _ . /` become one dash, everything else is dropped; at most 40 chars.
 */
export function slug(name: string): string {
  let out = ''
  let dash = false
  for (const ch of name.toLowerCase()) {
    if (/[a-z0-9]/.test(ch)) {
      out += ch
      dash = false
    } else if (/[\s\-_./]/.test(ch)) {
      if (!dash && out.length > 0) {
        out += '-'
        dash = true
      }
    }
    if (out.length >= 40) break
  }
  return out.replace(/^-+|-+$/g, '')
}

/** Username the API derives from a tablet name (`device-<slug>`), for the preview; '' when invalid. */
export function deviceUsername(name: string): string {
  const s = slug(name.trim())
  return s && name.trim().length <= 100 ? `device-${s}` : ''
}
