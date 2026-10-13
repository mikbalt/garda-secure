import { AxiosError, AxiosHeaders, type AxiosResponse, type InternalAxiosRequestConfig } from 'axios'
import { describe, expect, it } from 'vitest'
import { flattenPages, nextOffset, pageOf } from '../api/paging'
import { exportFilename } from '../components/exportFormat'
import { crc32, csvBlob, pdfBlob, xlsxBlob } from '../demo/files'
import { compliance, expectations, floorplanUrl, planPositionError, scheduleError, type PatrolWorld } from '../demo/patrolDemo'
import { emptyWorld } from '../demo/world'
import { ERROR_CODES, errorText } from '../i18n/errors'
import { exportLimit } from '../i18n/errors.patrol'
import { floorplanFileError, planPos, round4 } from '../pages/patrol/plan/floorplans'
import { hasPlanPositions, planState } from '../pages/patrol/plan/planState'
import { amendedFields, gpsText } from './incidents'
import type { CheckpointProgress } from '../types'

/** jsdom's Blob has no arrayBuffer()/text(): read it with a FileReader. */
function bytesOf(b: Blob): Promise<Uint8Array> {
  return new Promise((resolve, reject) => {
    const r = new FileReader()
    r.onload = () => resolve(new Uint8Array(r.result as ArrayBuffer))
    r.onerror = () => reject(r.error)
    r.readAsArrayBuffer(b)
  })
}
const textOf = async (b: Blob) => new TextDecoder('utf-8', { ignoreBOM: true }).decode(await bytesOf(b))

function apiError(status: number, data: unknown): AxiosError {
  const config = { headers: new AxiosHeaders() } as InternalAxiosRequestConfig
  return new AxiosError('x', 'ERR_BAD_REQUEST', config, {}, { status, statusText: '', data, headers: {}, config } as AxiosResponse)
}

describe('paged lists (D-045)', () => {
  it('reads X-Total-Count / X-Truncated', () => {
    expect(pageOf([1, 2], { 'x-total-count': '7', 'x-truncated': 'true' })).toEqual({ rows: [1, 2], total: 7, truncated: true, offset: 0 })
    expect(pageOf([1, 2], {})).toEqual({ rows: [1, 2], total: 2, truncated: false, offset: 0 })
    // A total smaller than what we already have is ignored.
    expect(pageOf([1, 2], { 'x-total-count': '1' }, 4).total).toBe(6)
    expect(pageOf({ not: 'a list' }, {}).rows).toEqual([])
  })
  it('computes the next offset and flattens pages', () => {
    const p1 = pageOf([1, 2], { 'x-total-count': '5', 'x-truncated': 'true' })
    const p2 = pageOf([3, 4], { 'x-total-count': '5', 'x-truncated': 'true' }, 2)
    const p3 = pageOf([5], { 'x-total-count': '5' }, 4)
    expect(nextOffset(p1)).toBe(2)
    expect(nextOffset(p2)).toBe(4)
    expect(nextOffset(p3)).toBeUndefined()
    expect(flattenPages([p1, p2, p3])).toEqual({ rows: [1, 2, 3, 4, 5], total: 5, truncated: false })
    expect(flattenPages(undefined)).toEqual({ rows: [], total: 0, truncated: false })
  })
})

describe('exports (D-045)', () => {
  it('names files per format', () => {
    expect(exportFilename('guard-kpi-2026-10-01-2026-10-12.csv', 'xlsx')).toBe('guard-kpi-2026-10-01-2026-10-12.xlsx')
    expect(exportFilename('incidents', 'pdf')).toBe('incidents.pdf')
  })
  it('explains export_too_large with the limit', () => {
    const msg = 'the export has 61234 rows; the limit for csv is 50000 — narrow the date range or filters'
    expect(exportLimit(msg)).toEqual({ rows: 61234, format: 'csv', limit: 50000 })
    const err = apiError(400, { code: 'export_too_large', error: msg })
    expect(errorText(err, 'en')).toBe('The export has 61,234 rows, more than the CSV limit of 50,000. Narrow the date range or the filters and try again.')
    expect(errorText(err, 'id')).toContain('melebihi batas CSV sebesar 50.000')
    expect(errorText(apiError(400, { code: 'export_too_large' }), 'en')).toBe(ERROR_CODES.export_too_large.en)
    expect(errorText(apiError(400, { code: 'invalid_format' }), 'id')).toBe(ERROR_CODES.invalid_format.id)
  })
  it('translates the floor plan and schedule codes', () => {
    for (const c of ['floorplan_in_use', 'too_many_floorplans', 'invalid_plan_position', 'unknown_floorplan', 'end_time_required', 'invalid_every', 'invalid_grace', 'grace_too_long', 'too_many_schedules', 'invalid_limit', 'invalid_gps']) {
      expect(ERROR_CODES[c], c).toBeDefined()
      expect(ERROR_CODES[c].id).not.toBe(ERROR_CODES[c].en)
    }
  })
})

describe('demo export files', () => {
  it('crc32', () => {
    expect(crc32(new TextEncoder().encode('123456789'))).toBe(0xcbf43926)
  })
  it('builds an XLSX zip, a CSV with BOM and a PDF', async () => {
    const x = xlsxBlob('Report', ['Name', 'N'], [['=cmd', 2]])
    expect(x.type).toBe('application/vnd.openxmlformats-officedocument.spreadsheetml.sheet')
    const bytes = await bytesOf(x)
    expect(String.fromCharCode(bytes[0], bytes[1])).toBe('PK')
    const text = new TextDecoder().decode(bytes)
    expect(text).toContain('<t xml:space="preserve">&apos;=cmd</t>'.replace('&apos;', "'"))
    const c = await textOf(csvBlob(['a'], [['+1'], ['x"y']]))
    expect(c.charCodeAt(0)).toBe(0xfeff)
    expect(c).toContain(`"'+1"`)
    expect(c).toContain('"x""y"')
    const p = await textOf(pdfBlob('Insiden – Gudang', ['Baris (1)']))
    expect(p.startsWith('%PDF-1.4')).toBe(true)
    expect(p).toContain('(Insiden  Gudang) Tj')
    expect(p).toContain('(Baris \\(1\\)) ')
    expect(p.trimEnd().endsWith('%%EOF')).toBe(true)
  })
})

describe('floor plans (D-048)', () => {
  it('checks files before upload', () => {
    expect(floorplanFileError(null)).toBe('file_required')
    expect(floorplanFileError(new File(['x'], 'a.gif', { type: 'image/gif' }))).toBe('invalid_file_type')
    const big = new File([new Uint8Array(10 * 1024 * 1024 + 1)], 'a.png', { type: 'image/png' })
    expect(floorplanFileError(big)).toBe('file_too_large')
    expect(floorplanFileError(new File(['x'], 'a.webp', { type: 'image/webp' }))).toBeNull()
  })
  it('reads complete positions only and rounds to the image', () => {
    expect(planPos({ floorplan_id: 'f', plan_x: 0.2, plan_y: 0 })).toEqual({ floorplan_id: 'f', plan_x: 0.2, plan_y: 0 })
    expect(planPos({ floorplan_id: 'f', plan_x: 0.2 })).toBeNull()
    expect(round4(1.2)).toBe(1)
    expect(round4(-0.1)).toBe(0)
    expect(round4(0.123456)).toBe(0.1235)
  })
  it('maps checkpoint status to plan markers', () => {
    const cp = (status: CheckpointProgress['status']) => ({ status }) as CheckpointProgress
    expect(planState(cp('on_time'), 0, 2)).toBe('scanned')
    expect(planState(cp('late'), 1, 2)).toBe('late')
    expect(planState(cp('pending'), 2, 2)).toBe('next')
    expect(planState(cp('pending'), 3, 2)).toBe('pending')
    expect(planState(cp('overdue'), 2, 2)).toBe('overdue')
    expect(planState(cp('missed'), 0, undefined)).toBe('missed')
    expect(hasPlanPositions({ checkpoints: [{ floorplan_id: 'f', plan_x: 0.5, plan_y: 0.5 } as CheckpointProgress] })).toBe(true)
    expect(hasPlanPositions({ checkpoints: [] })).toBe(false)
  })
})

describe('incidents (D-043, D-049)', () => {
  it('lists amended fields and formats GPS as text', () => {
    expect(amendedFields({ original_severity: 'high', original_location: undefined })).toEqual(['severity'])
    expect(amendedFields({})).toEqual([])
    expect(gpsText(-6.2944, 107.1531)).toBe('-6.294400, 107.153100')
    expect(gpsText(null, 1)).toBeNull()
  })
})

describe('demo mock: schedules and plans', () => {
  const world = (): PatrolWorld => {
    const w = emptyWorld() as PatrolWorld
    w.sites.push({ id: 's1', name: 'S', timezone: 'Asia/Jakarta', created_at: '2026-01-01T00:00:00Z' })
    w.routes.push({ id: 'route-000001', site_id: 's1', name: 'R', active: true, created_at: '2026-01-01T00:00:00Z', checkpoints: [] })
    w.floorplans = [{ id: 'f1', site_id: 's1', name: 'P', object_key: 'k', content_type: 'image/png', width: 10, height: 10, size_bytes: 1, created_at: '' }]
    return w
  }
  it('draws well-formed SVG floor plans (names are escaped)', () => {
    for (const kind of ['warehouse', 'perimeter', 'office', 'x']) {
      const url = floorplanUrl({ id: 'f', site_id: 's', name: 'A & <B>', object_key: `k#${kind}`, content_type: 'image/png', width: 1, height: 1, size_bytes: 1, created_at: '' })
      const doc = new DOMParser().parseFromString(decodeURIComponent(url.split(',')[1]), 'image/svg+xml')
      expect(doc.getElementsByTagName('parsererror')).toHaveLength(0)
    }
  })
  it('validates plan positions and schedules like the API', () => {
    const w = world()
    expect(planPositionError(w, 's1', [{ name: 'a', floorplan_id: 'f1', plan_x: 0.5, plan_y: 0.5 }])).toBeNull()
    expect(planPositionError(w, 's1', [{ name: 'a', floorplan_id: 'f1', plan_x: 0.5 }])?.code).toBe('invalid_plan_position')
    expect(planPositionError(w, 's1', [{ name: 'a', floorplan_id: 'f1', plan_x: 1.5, plan_y: 0 }])?.code).toBe('invalid_plan_position')
    expect(planPositionError(w, 's2', [{ name: 'a', floorplan_id: 'f1', plan_x: 0.5, plan_y: 0.5 }])?.code).toBe('unknown_floorplan')
    expect(scheduleError([{ weekdays: [1], start_time: '08:00', end_time: null, every_minutes: 60, grace_minutes: 15, active: true }])).toEqual({
      code: 'end_time_required',
      msg: 'schedule 1: end time required',
      field: 'end_time',
    })
  })
  it('counts scheduled patrols and decides them', () => {
    const w = world()
    w.route_schedules = { 'route-000001': [{ id: 'x', route_id: 'route-000001', weekdays: [1, 2, 3, 4, 5, 6, 7], start_time: '08:00', end_time: '17:00', every_minutes: 180, grace_minutes: 15, active: true }] }
    const now = Date.now()
    const from = now - 3 * 86_400_000
    const list = expectations(w, 's1', from, now)
    expect(list.length).toBeGreaterThanOrEqual(6)
    expect(list.every((e) => e.due >= from && e.due < now)).toBe(true)
    const { totals, rows } = compliance(w, 's1', from, now)
    expect(totals.expected).toBe(list.length)
    expect(totals.started_on_time + totals.late + totals.missed + totals.pending).toBe(totals.expected)
    expect(rows.reduce((n, r) => n + r.expected, 0)).toBe(totals.expected)
  })
})
