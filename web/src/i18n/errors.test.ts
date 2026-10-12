import { AxiosError, AxiosHeaders, type AxiosResponse, type InternalAxiosRequestConfig } from 'axios'
import { describe, expect, it } from 'vitest'
import { DELIVERY_REASONS, ERROR_CODES, errorText, fieldLabel } from './errors'

function apiError(status: number, data: unknown, headers: Record<string, string> = {}): AxiosError {
  const config = { headers: new AxiosHeaders() } as InternalAxiosRequestConfig
  const response = { status, statusText: String(status), data, headers, config } as AxiosResponse
  return new AxiosError('Request failed', 'ERR_BAD_RESPONSE', config, {}, response)
}

/** Every code documented in docs/api.md (WEB-7). */
const DOCUMENTED = [
  'id_conflict',
  'field_too_long',
  'body_too_large',
  'rate_limited',
  'platform_admin_required',
  'secret_required',
  'invalid_smtp_port',
  'invalid_smtp_host',
  'smtp_host_not_allowed',
  'smtp_host_unresolvable',
  'whatsapp_url_not_allowed',
  'invalid_phone_number_id',
  'media_exists',
  'media_forbidden',
  'invalid_image',
  'employee_id_taken',
  'internal',
  'duplicate',
  'delivery_failed',
  'no_shift',
  'already_clocked_in',
  'not_clocked_in',
  'clock_out_window_closed',
  'device_unregistered',
  'device_pending',
  'device_revoked',
  'guard_unknown',
  'guard_not_allowed',
  'guard_required',
  'not_failed',
  'shift_has_attendance',
  'range_too_long',
  'invalid_range',
  'reason_required',
]

describe('error code catalogue', () => {
  it('translates every documented code', () => {
    for (const code of DOCUMENTED) expect(ERROR_CODES[code], code).toBeDefined()
  })

  it('has non-empty English and Indonesian text for every code', () => {
    for (const [code, pair] of Object.entries({ ...ERROR_CODES, ...DELIVERY_REASONS })) {
      expect(pair.en.trim(), `${code} en`).not.toBe('')
      expect(pair.id.trim(), `${code} id`).not.toBe('')
      expect(pair.id, `${code}: Indonesian must not just copy the English`).not.toBe(pair.en)
    }
  })
})

describe('errorText', () => {
  it('uses the translated code', () => {
    const err = apiError(409, { error: 'employee_id already used', code: 'employee_id_taken' })
    expect(errorText(err, 'id')).toMatch(/sudah dipakai/)
    expect(errorText(err, 'en')).toMatch(/already used/)
  })

  it('names the field and limit of field_too_long', () => {
    const err = apiError(400, { error: 'description is too long (max 4000 characters)', code: 'field_too_long', field: 'description' })
    expect(errorText(err, 'id')).toBe('Deskripsi terlalu panjang (maks. 4000 karakter).')
    expect(errorText(err, 'en')).toBe('Description is too long (max. 4000 characters).')
  })

  it('humanises unknown field names', () => {
    expect(fieldLabel('host_name', 'id')).toBe('Yang dikunjungi')
    expect(fieldLabel('route_note', 'en')).toBe('Route note')
  })

  it('shows Retry-After seconds for rate_limited', () => {
    const err = apiError(429, { error: 'too many requests', code: 'rate_limited' }, { 'retry-after': '17' })
    expect(errorText(err, 'id')).toContain('17 detik')
    expect(errorText(err, 'en')).toContain('17 seconds')
  })

  it('shows the request id for server errors and never the raw text', () => {
    const err = apiError(500, { error: 'internal error', code: 'internal', request_id: 'req-abc123' })
    const id = errorText(err, 'id')
    expect(id).toContain('req-abc123')
    expect(id).toContain('Terjadi kesalahan di server')
    expect(errorText(apiError(503, { error: 'pq: connection refused' }), 'en')).not.toContain('pq:')
  })

  it('reads the request id from the X-Request-ID header too', () => {
    const err = apiError(502, 'Bad Gateway', { 'x-request-id': 'hdr-42' })
    expect(errorText(err, 'en')).toContain('hdr-42')
  })

  it('explains failed channel tests by reason', () => {
    const err = apiError(502, { error: 'delivery failed', code: 'delivery_failed', reason: 'auth_failed' })
    expect(errorText(err, 'id')).toContain('Login ditolak')
    expect(errorText(err, 'en')).toContain('Login was rejected')
  })

  it('falls back by status for unknown codes (no English in the Indonesian UI)', () => {
    const err = apiError(400, { error: 'weird thing happened', code: 'brand_new_code' })
    expect(errorText(err, 'id')).toBe('Data tidak valid.')
    expect(errorText(err, 'en')).toBe('weird thing happened')
    expect(errorText(apiError(403, {}), 'id')).toBe('Anda tidak diizinkan melakukan ini.')
  })

  it('reports network failures', () => {
    const config = { headers: new AxiosHeaders() } as InternalAxiosRequestConfig
    const err = new AxiosError('Network Error', 'ERR_NETWORK', config)
    expect(errorText(err, 'id')).toMatch(/koneksi/)
  })

  it('never shows raw text of non-HTTP errors', () => {
    expect(errorText(new TypeError("Cannot read properties of undefined (reading 'x')"), 'en')).toBe('Something went wrong.')
  })

  it('prefers a body parsed from a blob download', () => {
    const err = apiError(400, new Blob(['{}']))
    expect(errorText(err, 'en', { body: { code: 'range_too_long', error: 'range too long' } })).toBe('The date range is too long.')
  })
})
