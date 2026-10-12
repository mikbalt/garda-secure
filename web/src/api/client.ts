import axios, { AxiosError } from 'axios'
import { config } from '../config'
import { freshToken, login } from '../keycloak'

/** Axios instance; baseURL already includes /api/v1 — call paths like `/sites`. */
const apiClient = axios.create({
  baseURL: config.apiUrl,
  headers: { 'Content-Type': 'application/json' },
})

apiClient.interceptors.request.use(async (req) => {
  const token = await freshToken()
  if (token) req.headers.Authorization = `Bearer ${token}`
  return req
})

let redirectingToLogin = false

/** Sends the user to the login page once (several queries may fail with 401 at once). */
export function handleUnauthorized(): void {
  if (redirectingToLogin) return
  redirectingToLogin = true
  login()
}

/** Test hook: forget that a login redirect was started. */
export function resetUnauthorizedForTests(): void {
  redirectingToLogin = false
}

apiClient.interceptors.response.use(
  (response) => response,
  (error) => {
    if (axios.isAxiosError(error) && error.response?.status === 401) handleUnauthorized()
    return Promise.reject(error)
  }
)

/** Error body returned by the backend (docs/api.md). */
export interface ApiErrorBody {
  error?: string
  code?: string
  /** Offending input field (e.g. `field_too_long`). */
  field?: string
  /** X-Request-ID of a 5xx answer, for support. */
  request_id?: string
  /** `delivery_failed` of a channel test: why the test message failed. */
  reason?: string
}

function body(err: unknown): ApiErrorBody | undefined {
  if (!axios.isAxiosError(err)) return undefined
  const data = (err as AxiosError<ApiErrorBody>).response?.data
  return data && typeof data === 'object' && !(typeof Blob !== 'undefined' && data instanceof Blob) ? data : undefined
}

/** Machine code of an API error, if any. */
export function apiErrorCode(err: unknown): string | undefined {
  return body(err)?.code
}

export function apiErrorStatus(err: unknown): number | undefined {
  return axios.isAxiosError(err) ? err.response?.status : undefined
}

/** The input field an error is about (`field` in the error body). */
export function apiErrorField(err: unknown): string | undefined {
  return body(err)?.field
}

/** `reason` of a failed channel test (502 `delivery_failed`). */
export function apiErrorReason(err: unknown): string | undefined {
  return body(err)?.reason
}

/** Request id of a server error: body `request_id`, else the X-Request-ID header. */
export function apiRequestId(err: unknown): string | undefined {
  const fromBody = body(err)?.request_id
  if (fromBody) return fromBody
  if (!axios.isAxiosError(err)) return undefined
  const h = err.response?.headers?.['x-request-id']
  return typeof h === 'string' && h ? h : undefined
}

/** Seconds from a 429's Retry-After header (integer seconds or HTTP date). */
export function apiRetryAfter(err: unknown, now = Date.now()): number | undefined {
  if (!axios.isAxiosError(err) || err.response?.status !== 429) return undefined
  const raw = err.response.headers?.['retry-after']
  if (raw === undefined || raw === null || raw === '') return undefined
  const s = String(raw).trim()
  if (/^\d+$/.test(s)) return Number(s)
  const at = Date.parse(s)
  return Number.isNaN(at) ? undefined : Math.max(0, Math.ceil((at - now) / 1000))
}

/** Raw server message (`error` field) or a transport error message. */
export function apiErrorMessage(err: unknown): string | undefined {
  if (axios.isAxiosError(err)) {
    const b = body(err)
    if (b && typeof b.error === 'string') return b.error
    if (!err.response) return err.message
    return undefined
  }
  if (err instanceof Error) return err.message
  return undefined
}

/**
 * TanStack Query retry policy: client errors are final; transient errors
 * (network, 5xx) are retried once, and a 429 once after its Retry-After.
 */
export function shouldRetry(count: number, err: unknown): boolean {
  const status = apiErrorStatus(err)
  if (count >= 1) return false
  if (status === 429) return (apiRetryAfter(err) ?? 5) <= 60
  return status === undefined || status >= 500
}

/** Delay before a retry: Retry-After for 429, otherwise a short backoff. */
export function retryDelay(count: number, err: unknown): number {
  const ra = apiRetryAfter(err)
  if (ra !== undefined) return ra * 1000
  return Math.min(1000 * 2 ** count, 10_000)
}

type Params = Record<string, string | number | boolean | undefined | null>

function cleanParams(params: Params): Record<string, string> {
  const out: Record<string, string> = {}
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== null && v !== '') out[k] = String(v)
  }
  return out
}

/** GET helper returning the response body. */
export async function apiGet<T>(path: string, params: Params = {}): Promise<T> {
  const { data } = await apiClient.get<T>(path, { params: cleanParams(params) })
  return data
}

/**
 * Downloads a file (e.g. a CSV report) with the bearer token — a plain
 * `<a href>` cannot send the Authorization header. The file is handed to the
 * browser as a `blob:` URL (WEB-9).
 */
export async function downloadFile(path: string, params: Params, fallbackName: string): Promise<void> {
  const res = await apiClient.get<Blob>(path, { params: cleanParams(params), responseType: 'blob' })
  const disposition = String(res.headers['content-disposition'] ?? '')
  const match = /filename="?([^";]+)"?/i.exec(disposition)
  const name = (match?.[1] ?? fallbackName).replace(/[\\/]/g, '_')
  const url = URL.createObjectURL(res.data)
  const a = document.createElement('a')
  a.href = url
  a.download = name
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

/** Parses a blob error body (downloads use responseType blob). */
export async function blobErrorBody(err: unknown): Promise<ApiErrorBody | undefined> {
  if (axios.isAxiosError(err) && typeof Blob !== 'undefined' && err.response?.data instanceof Blob) {
    try {
      return JSON.parse(await err.response.data.text()) as ApiErrorBody
    } catch {
      return undefined
    }
  }
  return body(err)
}

export default apiClient
