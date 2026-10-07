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

apiClient.interceptors.response.use(
  (response) => response,
  (error) => {
    if (axios.isAxiosError(error) && error.response?.status === 401) {
      login()
    }
    return Promise.reject(error)
  }
)

/** Error body returned by the backend. */
export interface ApiErrorBody {
  error?: string
  code?: string
}

/** Machine code of an API error, if any. */
export function apiErrorCode(err: unknown): string | undefined {
  if (axios.isAxiosError(err)) {
    const body = err.response?.data as ApiErrorBody | undefined
    return body?.code
  }
  return undefined
}

export function apiErrorStatus(err: unknown): number | undefined {
  return axios.isAxiosError(err) ? err.response?.status : undefined
}

/** Raw server message (`error` field) or a transport error message. */
export function apiErrorMessage(err: unknown): string | undefined {
  if (axios.isAxiosError(err)) {
    const body = (err as AxiosError<ApiErrorBody>).response?.data
    if (body && typeof body === 'object' && typeof body.error === 'string') return body.error
    if (!err.response) return err.message
    return undefined
  }
  if (err instanceof Error) return err.message
  return undefined
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
 * `<a href>` cannot send the Authorization header.
 */
export async function downloadFile(path: string, params: Params, fallbackName: string): Promise<void> {
  const res = await apiClient.get<Blob>(path, { params: cleanParams(params), responseType: 'blob' })
  const disposition = String(res.headers['content-disposition'] ?? '')
  const match = /filename="?([^";]+)"?/i.exec(disposition)
  const name = match?.[1] ?? fallbackName
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
export async function blobErrorMessage(err: unknown): Promise<string | undefined> {
  if (axios.isAxiosError(err) && err.response?.data instanceof Blob) {
    try {
      const body = JSON.parse(await err.response.data.text()) as ApiErrorBody
      return body.error
    } catch {
      return undefined
    }
  }
  return apiErrorMessage(err)
}

export default apiClient
