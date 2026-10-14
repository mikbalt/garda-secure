import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { I18nProvider } from '../../i18n/I18nProvider'
import type { PushPreferences } from '../../types/push'

const apiGet = vi.fn()
const put = vi.fn()
vi.mock('../../api/client', async (orig) => {
  const real = await orig<typeof import('../../api/client')>()
  return { ...real, apiGet: (...a: unknown[]) => apiGet(...a), default: { ...real.default, put: (...a: unknown[]) => put(...a) } }
})

const { Preferences } = await import('./Preferences')
const { groupAvailable } = await import('./events')

function renderPrefs(hasDevice = true) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <I18nProvider>
        <Preferences hasDevice={hasDevice} />
      </I18nProvider>
    </QueryClientProvider>
  )
}

const hrPrefs: PushPreferences = {
  available: ['absent', 'late_clock_in', 'attendance_review'],
  events: { absent: true, late_clock_in: false, attendance_review: true },
}

describe('push preferences', () => {
  beforeEach(() => {
    apiGet.mockReset()
    put.mockReset()
  })

  it('shows only the available events, in their group', async () => {
    apiGet.mockResolvedValue(hrPrefs)
    renderPrefs()
    const group = await screen.findByRole('region', { name: 'Absensi' })
    expect(within(group).getAllByRole('switch')).toHaveLength(3)
    expect(screen.queryByRole('region', { name: 'SOS & insiden' })).toBeNull()
    expect(screen.queryByRole('switch', { name: 'Tombol SOS ditekan' })).toBeNull()
    expect(screen.getByRole('switch', { name: 'Petugas tidak hadir' })).toHaveAttribute('aria-checked', 'true')
    expect(screen.getByRole('switch', { name: 'Petugas terlambat' })).toHaveAttribute('aria-checked', 'false')
    expect(apiGet).toHaveBeenCalledWith('/push/preferences')
  })

  it('saves a toggle with PUT (optimistic) and keeps the server answer', async () => {
    apiGet.mockResolvedValue(hrPrefs)
    put.mockResolvedValue({ data: { ...hrPrefs, events: { ...hrPrefs.events, late_clock_in: true } } })
    renderPrefs()
    const late = await screen.findByRole('switch', { name: 'Petugas terlambat' })
    fireEvent.click(late)
    await waitFor(() => expect(late).toHaveAttribute('aria-checked', 'true'))
    await waitFor(() => expect(put).toHaveBeenCalledWith('/push/preferences', { events: { late_clock_in: true } }))
    await waitFor(() => expect(late).toHaveAttribute('aria-checked', 'true'))
  })

  it('rolls back when saving fails', async () => {
    apiGet.mockResolvedValue(hrPrefs)
    put.mockRejectedValue(new Error('offline'))
    renderPrefs()
    const absent = await screen.findByRole('switch', { name: 'Petugas tidak hadir' })
    fireEvent.click(absent)
    await waitFor(() => expect(put).toHaveBeenCalled())
    await waitFor(() => expect(absent).toHaveAttribute('aria-checked', 'true'))
  })

  it('explains that no device receives them yet', async () => {
    apiGet.mockResolvedValue(hrPrefs)
    renderPrefs(false)
    expect(await screen.findByText(/Belum ada perangkat yang menerima notifikasi/)).toBeInTheDocument()
  })

  it('says so when the role has no push alerts', async () => {
    apiGet.mockResolvedValue({ available: [], events: {} })
    renderPrefs()
    expect(await screen.findByText('Tidak ada peringatan push untuk peran Anda.')).toBeInTheDocument()
    expect(screen.queryAllByRole('switch')).toHaveLength(0)
  })
})

describe('groupAvailable', () => {
  it('keeps group order, drops empty groups and collects unknown events under Other', () => {
    const g = groupAvailable(['device_pending', 'sos', 'brand_new_event', 'sla_missed'])
    expect(g.map((x) => x.key)).toEqual(['sos', 'patrol', 'ops', 'other'])
    expect(g.find((x) => x.key === 'other')?.events).toEqual(['brand_new_event'])
  })
})
