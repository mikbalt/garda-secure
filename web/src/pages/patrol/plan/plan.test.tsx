import { fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import type { ReactNode } from 'react'
import { I18nProvider } from '../../../i18n/I18nProvider'
import { floorplansKey } from './floorplans'
import { LivePlan } from './LivePlan'
import { PlanPlacement } from './PlanPlacement'
import type { CheckpointProgress, Floorplan, SessionProgress, Site } from '../../../types'

const site: Site = { id: 's1', name: 'Gudang', timezone: 'Asia/Jakarta', created_at: '2026-01-01T00:00:00Z' }
const plan: Floorplan = {
  id: 'f1',
  site_id: 's1',
  name: 'Lantai dasar',
  object_key: 'k',
  content_type: 'image/png',
  width: 1200,
  height: 800,
  size_bytes: 1,
  created_at: '2026-01-01T00:00:00Z',
  url: 'https://example.test/plan.png',
}

function wrap(ui: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  qc.setQueryData(floorplansKey('s1'), [plan])
  return render(
    <QueryClientProvider client={qc}>
      <I18nProvider>{ui}</I18nProvider>
    </QueryClientProvider>
  )
}

describe('PlanPlacement (keyboard alternative)', () => {
  it('nudges a marker with the arrow keys and removes it with Delete', () => {
    const onPlace = vi.fn()
    wrap(
      <PlanPlacement
        site={site}
        onPlace={onPlace}
        rows={[
          { key: 'a', name: 'Gerbang', pos: { floorplan_id: 'f1', plan_x: 0.5, plan_y: 0.5 } },
          { key: 'b', name: 'Dock', pos: null },
        ]}
      />
    )
    const marker = screen.getByRole('button', { name: /Titik 1, Gerbang, di X 50 %, Y 50 %/ })
    fireEvent.keyDown(marker, { key: 'ArrowRight' })
    expect(onPlace).toHaveBeenLastCalledWith('a', { floorplan_id: 'f1', plan_x: 0.51, plan_y: 0.5 })
    fireEvent.keyDown(marker, { key: 'ArrowUp', shiftKey: true })
    expect(onPlace).toHaveBeenLastCalledWith('a', { floorplan_id: 'f1', plan_x: 0.5, plan_y: 0.45 })
    fireEvent.keyDown(marker, { key: 'Delete' })
    expect(onPlace).toHaveBeenLastCalledWith('a', null)
  })

  it('places the selected checkpoint by typing X / Y in percent', () => {
    const onPlace = vi.fn()
    wrap(<PlanPlacement site={site} onPlace={onPlace} rows={[{ key: 'b', name: 'Dock', pos: null }]} />)
    fireEvent.change(screen.getByLabelText('X (%)'), { target: { value: '25' } })
    expect(onPlace).toHaveBeenLastCalledWith('b', { floorplan_id: 'f1', plan_x: 0.25, plan_y: 0.5 })
  })
})

describe('LivePlan', () => {
  const cp = (i: number, status: CheckpointProgress['status'], placed = true): CheckpointProgress => ({
    id: `c${i}`,
    name: `Titik ${i + 1}`,
    qr_code: '',
    order_index: i,
    status,
    ...(placed ? { floorplan_id: 'f1', plan_x: 0.1 * (i + 1), plan_y: 0.5 } : {}),
    ...(status === 'on_time' ? { scanned_at: '2026-10-12T03:05:00Z' } : {}),
  })
  const progress: SessionProgress = {
    session: { id: 's', route_id: 'r', site_id: 's1', guard_id: 'g', started_at: '2026-10-12T03:00:00Z', status: 'in_progress', created_at: '' },
    route_name: 'Keliling',
    checkpoints: [cp(0, 'on_time'), cp(1, 'overdue'), cp(2, 'pending'), cp(3, 'pending', false)],
    total: 4,
    scanned: 1,
    completion_pct: 25,
    next_index: 1,
  }

  it('mirrors the picture as a text list (colour is not the only signal)', () => {
    wrap(<LivePlan site={site} progress={progress} />)
    expect(screen.getByRole('img', { name: /Denah Lantai dasar dengan titik-titik Keliling/ })).toBeInTheDocument()
    const items = screen.getAllByRole('listitem').map((li) => li.textContent ?? '')
    expect(items.some((t) => t.includes('Titik 1') && t.includes('Sudah dipindai · 10:05'))).toBe(true)
    expect(items.some((t) => t.includes('Titik 2') && t.includes('Lewat batas'))).toBe(true)
    expect(items.some((t) => t.includes('Titik 4') && t.includes('Tidak di denah'))).toBe(true)
    expect(screen.getByText('Pindai terakhir 10:05')).toBeInTheDocument()
  })
})
