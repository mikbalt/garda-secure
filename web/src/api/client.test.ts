import { AxiosError, AxiosHeaders, type AxiosAdapter, type AxiosResponse, type InternalAxiosRequestConfig } from 'axios'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const login = vi.fn()
const freshToken = vi.fn(async () => 'token-123')
vi.mock('../keycloak', () => ({ login: () => login(), freshToken: () => freshToken() }))

const client = await import('./client')
const apiClient = client.default

function respondWith(status: number, data: unknown, headers: Record<string, string> = {}): AxiosAdapter {
  return async (config: InternalAxiosRequestConfig) => {
    const response = { status, statusText: String(status), data, headers, config } as AxiosResponse
    if (status >= 400) throw new AxiosError(`status ${status}`, 'ERR_BAD_RESPONSE', config, {}, response)
    return response
  }
}

describe('API client', () => {
  let captured: InternalAxiosRequestConfig | undefined
  beforeEach(() => {
    login.mockClear()
    client.resetUnauthorizedForTests()
    captured = undefined
  })
  afterEach(() => {
    apiClient.defaults.adapter = undefined
  })

  it('sends the bearer token and cleans empty params', async () => {
    apiClient.defaults.adapter = async (config) => {
      captured = config
      return { status: 200, statusText: 'OK', data: [1], headers: {}, config } as AxiosResponse
    }
    await expect(client.apiGet('/sites', { a: 'x', b: '', c: undefined, d: null, e: 0 })).resolves.toEqual([1])
    expect(String(captured?.headers.Authorization)).toBe('Bearer token-123')
    expect(captured?.params).toEqual({ a: 'x', e: '0' })
  })

  it('redirects to login once on 401', async () => {
    apiClient.defaults.adapter = respondWith(401, { error: 'unauthorized', code: 'unauthorized' })
    await expect(client.apiGet('/sites')).rejects.toBeInstanceOf(AxiosError)
    await expect(client.apiGet('/guards')).rejects.toBeInstanceOf(AxiosError)
    expect(login).toHaveBeenCalledTimes(1)
  })

  it('does not log out on 403', async () => {
    apiClient.defaults.adapter = respondWith(403, { error: 'forbidden', code: 'forbidden' })
    await expect(client.apiGet('/devices')).rejects.toBeInstanceOf(AxiosError)
    expect(login).not.toHaveBeenCalled()
  })

  it('exposes 429 Retry-After and retries once after it', async () => {
    apiClient.defaults.adapter = respondWith(429, { error: 'too many', code: 'rate_limited' }, { 'retry-after': '7' })
    const err = await client.apiGet('/sites').catch((e: unknown) => e)
    expect(client.apiErrorStatus(err)).toBe(429)
    expect(client.apiErrorCode(err)).toBe('rate_limited')
    expect(client.apiRetryAfter(err)).toBe(7)
    expect(client.shouldRetry(0, err)).toBe(true)
    expect(client.shouldRetry(1, err)).toBe(false)
    expect(client.retryDelay(0, err)).toBe(7000)
  })

  it('does not wait minutes for a long Retry-After', async () => {
    apiClient.defaults.adapter = respondWith(429, { code: 'rate_limited' }, { 'retry-after': '600' })
    const err = await client.apiGet('/sites').catch((e: unknown) => e)
    expect(client.shouldRetry(0, err)).toBe(false)
  })

  it('parses an HTTP-date Retry-After', () => {
    const config = { headers: new AxiosHeaders() } as InternalAxiosRequestConfig
    const now = Date.parse('2026-10-12T10:00:00Z')
    const response = { status: 429, statusText: '429', data: {}, headers: { 'retry-after': 'Mon, 12 Oct 2026 10:00:30 GMT' }, config } as AxiosResponse
    const err = new AxiosError('x', 'ERR_BAD_RESPONSE', config, {}, response)
    expect(client.apiRetryAfter(err, now)).toBe(30)
  })

  it('carries request_id and field of error bodies', async () => {
    apiClient.defaults.adapter = respondWith(500, { error: 'internal error', code: 'internal', request_id: 'rid-9' })
    const err = await client.apiGet('/sites').catch((e: unknown) => e)
    expect(client.apiRequestId(err)).toBe('rid-9')
    expect(client.shouldRetry(0, err)).toBe(true)
    expect(client.shouldRetry(1, err)).toBe(false)

    apiClient.defaults.adapter = respondWith(400, { error: 'name too long', code: 'field_too_long', field: 'name' })
    const err2 = await client.apiGet('/sites').catch((e: unknown) => e)
    expect(client.apiErrorField(err2)).toBe('name')
    expect(client.shouldRetry(0, err2)).toBe(false)
  })

  it('retries network errors once', () => {
    const config = { headers: new AxiosHeaders() } as InternalAxiosRequestConfig
    const err = new AxiosError('Network Error', 'ERR_NETWORK', config)
    expect(client.shouldRetry(0, err)).toBe(true)
    expect(client.retryDelay(1, err)).toBe(2000)
  })
})
