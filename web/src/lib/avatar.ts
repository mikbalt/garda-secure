/** Avatar utilities shared by Avatar component and people pages. */

/** Up to two initials from a person's name ("Budi Santoso" → "BS"). */
export function initials(name: string | undefined | null): string {
  const parts = (name ?? '').trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) return '?'
  return parts
    .slice(0, 2)
    .map((p) => p[0]!.toUpperCase())
    .join('')
}

const AVATAR_TONES = [
  'bg-primary-50 text-primary-700 ring-primary-100',
  'bg-emerald-50 text-emerald-700 ring-emerald-100',
  'bg-amber-50 text-amber-800 ring-amber-100',
  'bg-violet-50 text-violet-700 ring-violet-100',
  'bg-rose-50 text-rose-700 ring-rose-100',
  'bg-cyan-50 text-cyan-700 ring-cyan-100',
  'bg-orange-50 text-orange-700 ring-orange-100',
]

/** Stable avatar colour for a name (same person → same colour everywhere). */
export function avatarTone(name: string | undefined | null): string {
  let h = 0
  for (const ch of name ?? '') h = (h * 31 + ch.charCodeAt(0)) >>> 0
  return AVATAR_TONES[h % AVATAR_TONES.length]!
}
