/** Before/after comparison for the audit log (flattened dotted paths). */

export interface DiffRow {
  path: string
  before: unknown
  after: unknown
  changed: boolean
}

function isPlainObject(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v)
}

/** Flattens nested objects to `a.b.c` paths; arrays and scalars are leaves. */
export function flatten(v: unknown, prefix = '', out: Record<string, unknown> = {}): Record<string, unknown> {
  if (isPlainObject(v)) {
    const keys = Object.keys(v)
    if (keys.length === 0 && prefix) out[prefix] = {}
    for (const k of keys) flatten(v[k], prefix ? `${prefix}.${k}` : k, out)
  } else if (prefix) {
    out[prefix] = v
  } else if (v !== undefined && v !== null) {
    out['(value)'] = v
  }
  return out
}

const same = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b)

/** Rows for every path in either side, changed ones first, then alphabetical. */
export function diffJson(before: unknown, after: unknown): DiffRow[] {
  const a = flatten(before)
  const b = flatten(after)
  const paths = Array.from(new Set([...Object.keys(a), ...Object.keys(b)]))
  return paths
    .map((path) => ({ path, before: a[path], after: b[path], changed: !same(a[path], b[path]) }))
    .sort((x, y) => Number(y.changed) - Number(x.changed) || x.path.localeCompare(y.path))
}

/** Compact display of a JSON leaf value. */
export function showValue(v: unknown): string {
  if (v === undefined) return '—'
  if (v === null) return 'null'
  if (typeof v === 'string') return v
  return JSON.stringify(v)
}
