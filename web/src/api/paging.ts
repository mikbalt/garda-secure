import apiClient from './client'
import type { Page } from '../types'

type Params = Record<string, string | number | boolean | undefined | null>

function clean(params: Params): Record<string, string> {
  const out: Record<string, string> = {}
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== null && v !== '') out[k] = String(v)
  }
  return out
}

/** Reads `X-Total-Count` / `X-Truncated` of a paged list (D-045). */
export function pageOf<T>(data: unknown, headers: Record<string, unknown> | undefined, offset = 0): Page<T> {
  const rows = Array.isArray(data) ? (data as T[]) : []
  const rawTotal = Number(headers?.['x-total-count'])
  const total = Number.isFinite(rawTotal) && rawTotal >= rows.length + offset ? rawTotal : rows.length + offset
  const truncated = String(headers?.['x-truncated'] ?? '').toLowerCase() === 'true' || total > offset + rows.length
  return { rows, total, truncated, offset }
}

/**
 * GET of a paged list (`/incidents`, `/sos`, `/defects`, `/visitors`,
 * `/reports/incidents`): the rows plus the total and whether more exist.
 */
export async function apiGetPage<T>(path: string, params: Params & { limit?: number; offset?: number }): Promise<Page<T>> {
  const res = await apiClient.get<T[]>(path, { params: clean(params) })
  return pageOf<T>(res.data, res.headers as Record<string, unknown>, params.offset ?? 0)
}

/** Rows of all loaded pages (useInfiniteQuery). */
export function flattenPages<T>(pages: Page<T>[] | undefined): { rows: T[]; total: number; truncated: boolean } {
  if (!pages || pages.length === 0) return { rows: [], total: 0, truncated: false }
  const last = pages[pages.length - 1]
  return { rows: pages.flatMap((p) => p.rows), total: last.total, truncated: last.truncated }
}

/** Next offset for useInfiniteQuery, or undefined when everything is loaded. */
export function nextOffset<T>(last: Page<T>): number | undefined {
  const loaded = last.offset + last.rows.length
  return last.truncated && last.rows.length > 0 && loaded < last.total ? loaded : undefined
}
