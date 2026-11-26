import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { I18nProvider } from '../i18n/I18nProvider'
import type { FlagRow } from '../types'

const get = vi.fn()
const post = vi.fn()
vi.mock('../api/client', () => ({
  apiGet: (...a: unknown[]) => get(...a),
  default: { post: (...a: unknown[]) => post(...a) },
}))
vi.mock('../config', () => ({ config: { apiUrl: '/api/v1', demo: false, realms: [] } }))
vi.mock('../context/site', () => ({
  useSite: () => ({
    sites: [
      { id: 's1', name: 'Gate A', timezone: 'Asia/Jakarta' },
      { id: 's2', name: 'Depot B', timezone: 'Asia/Jakarta' },
    ],
    site: { id: 's1', name: 'Gate A', timezone: 'Asia/Jakarta' },
    siteId: 's1',
  }),
}))
vi.mock('../hooks/useGuards', () => ({
  useGuards: () => ({ guards: [{ id: 'g1', name: 'Budi' }], guardName: () => 'Budi' }),
}))

const row = (over: Partial<FlagRow>): FlagRow => ({
  kind: 'attendance',
  record_id: 'a1',
  flag: 'late_sync',
  site_id: 's1',
  site_name: 'Gate A',
  guard_id: 'g1',
  guard_name: 'Budi',
  event_at: '2026-03-10T01:00:00Z',
  ...over,
})

const { default: Flags } = await import('./Flags')

function mount() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <I18nProvider>
        <MemoryRouter>
          <Flags />
        </MemoryRouter>
      </I18nProvider>
    </QueryClientProvider>
  )
}

beforeEach(() => {
  get.mockReset().mockResolvedValue({ rows: [row({}), row({ kind: 'sos', record_id: 'x1', flag: 'guard_unverified' })], limit: 50, offset: 0, has_more: false })
  post.mockReset().mockResolvedValue({ data: { reviewed_at: '2026-03-11T00:00:00Z', reviewed_by: 'demo.supervisor' } })
})

describe('Flags page', () => {
  it('lists flags and links each one to its record', async () => {
    mount()
    const links = await screen.findAllByRole('link', { name: /record|catatan/i })
    expect(links.map((l) => l.getAttribute('href'))).toEqual(['/attendance?date=2026-03-10', '/incidents?tab=sos'])
  })

  it('sends the guard, site and date filters to the API', async () => {
    mount()
    await screen.findAllByRole('link', { name: /record|catatan/i })
    fireEvent.change(screen.getByLabelText(/^(Site|Lokasi)/), { target: { value: 's2' } })
    fireEvent.change(screen.getByLabelText(/^(Guard|Satpam)/), { target: { value: 'g1' } })
    fireEvent.change(screen.getByLabelText(/^(From|Dari)/), { target: { value: '2026-03-01' } })
    fireEvent.click(screen.getByRole('button', { name: /apply|terapkan/i }))
    await waitFor(() => {
      const last = get.mock.calls.at(-1)!
      expect(last[0]).toBe('/flags')
      expect(last[1]).toMatchObject({ site_id: 's2', guard_id: 'g1' })
      expect(String(last[1].from)).toContain('2026-02-28T17:00:00') // midnight Asia/Jakarta
    })
  })

  it('marks a flag reviewed with a POST and shows who reviewed it', async () => {
    mount()
    const first = (await screen.findAllByRole('row'))[1]
    // The list is refetched after the POST and now carries the review mark.
    get.mockResolvedValue({
      rows: [row({ reviewed_by: 'demo.supervisor', reviewed_at: '2026-03-11T00:00:00Z' })],
      limit: 50,
      offset: 0,
      has_more: false,
    })
    fireEvent.click(within(first).getByRole('button', { name: /mark reviewed|tandai/i }))
    await waitFor(() => expect(post).toHaveBeenCalledWith('/flags/review', { kind: 'attendance', record_id: 'a1', flag: 'late_sync' }))
    expect(await screen.findByText(/demo\.supervisor/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /mark reviewed|tandai/i })).toBeNull()
  })
})
