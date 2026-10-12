/**
 * URL guards for anything the API hands us to render or open (WEB-9):
 * photo URLs (pre-signed MinIO links) and downloads. Only `https:`,
 * `blob:` and same-origin URLs pass; `data:image/…` only in demo mode
 * (its sample photos are inline SVGs). `javascript:`, `data:text/html` and
 * friends are dropped.
 */
export function safeMediaUrl(
  raw: string | null | undefined,
  opts: { allowDataImages?: boolean; origin?: string; /** dev server only: MinIO on plain http */ allowHttp?: boolean } = {}
): string | undefined {
  if (!raw) return undefined
  const origin = opts.origin ?? (typeof window !== 'undefined' ? window.location.origin : 'http://localhost')
  let u: URL
  try {
    u = new URL(raw, origin)
  } catch {
    return undefined
  }
  if (u.protocol === 'https:' || u.protocol === 'blob:') return u.href
  if (u.origin === origin && (u.protocol === 'http:' || u.protocol === 'https:')) return u.href
  if (opts.allowHttp && u.protocol === 'http:') return u.href
  if (opts.allowDataImages && u.protocol === 'data:' && /^data:image\/(png|jpe?g|webp|gif|svg\+xml)[;,]/i.test(raw)) return raw
  return undefined
}
