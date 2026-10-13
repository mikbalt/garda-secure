import { useId, useMemo, useRef, useState, type KeyboardEvent, type PointerEvent } from 'react'
import { Crosshair, MapPinOff } from 'lucide-react'
import { Button } from '../../../components/ui/Button'
import { Input, Select } from '../../../components/ui/Form'
import { Notice } from '../../../components/ui/States'
import { useI18n } from '../../../i18n/context'
import { PlanImage, PlanPath } from './PlanImage'
import { clamp01, pctText, round4, useFloorplans, type PlanPos } from './floorplans'
import type { Site } from '../../../types'

export interface PlaceRow {
  key: string
  name: string
  pos: PlanPos | null
}

const NUDGE = 0.01
const NUDGE_FAST = 0.05

/**
 * Route editor: place checkpoints on a site floor plan (D-048). Click the
 * picture to put the selected checkpoint there, drag markers to move them;
 * keyboard: focus a marker and use the arrow keys (Shift = bigger steps),
 * Delete removes it, or type X / Y in percent.
 */
export function PlanPlacement({ site, rows, onPlace }: { site: Site; rows: PlaceRow[]; onPlace: (key: string, pos: PlanPos | null) => void }) {
  const { t } = useI18n()
  const { data: plans = [], isLoading } = useFloorplans(site.id)
  const sorted = useMemo(() => [...plans].sort((a, b) => a.name.localeCompare(b.name)), [plans])
  const usedPlan = rows.find((r) => r.pos && sorted.some((p) => p.id === r.pos!.floorplan_id))?.pos?.floorplan_id
  const [planChoice, setPlanChoice] = useState<string | null>(null)
  const planId = planChoice && sorted.some((p) => p.id === planChoice) ? planChoice : (usedPlan ?? sorted[0]?.id)
  const plan = sorted.find((p) => p.id === planId)
  const [activeChoice, setActiveChoice] = useState<string | null>(null)
  const active = rows.find((r) => r.key === activeChoice) ?? rows.find((r) => !r.pos) ?? rows[0]
  const box = useRef<HTMLDivElement>(null)
  const drag = useRef<string | null>(null)
  const ids = { plan: useId(), cp: useId(), x: useId(), y: useId(), help: useId() }

  if (isLoading) return <div className="h-40 animate-pulse rounded-xl bg-gray-100" aria-hidden="true" />
  if (sorted.length === 0) return <Notice>{t('p2.place.noPlans')}</Notice>
  if (rows.length === 0 || !plan) return <Notice>{t('p2.place.noRows')}</Notice>

  const posFromEvent = (e: { clientX: number; clientY: number }): { x: number; y: number } | null => {
    const r = box.current?.getBoundingClientRect()
    if (!r || r.width === 0 || r.height === 0) return null
    return { x: round4((e.clientX - r.left) / r.width), y: round4((e.clientY - r.top) / r.height) }
  }
  const place = (key: string, x: number, y: number) => onPlace(key, { floorplan_id: plan.id, plan_x: round4(x), plan_y: round4(y) })

  const onImageDown = (e: PointerEvent<HTMLDivElement>) => {
    if (e.button !== 0 || !active) return
    const p = posFromEvent(e)
    if (!p) return
    place(active.key, p.x, p.y)
    // Continue with the next checkpoint that still needs a place.
    const i = rows.findIndex((r) => r.key === active.key)
    const next = [...rows.slice(i + 1), ...rows.slice(0, i)].find((r) => !r.pos)
    if (next) setActiveChoice(next.key)
  }

  const onMarkerDown = (e: PointerEvent<HTMLButtonElement>, key: string) => {
    e.stopPropagation()
    setActiveChoice(key)
    drag.current = key
    e.currentTarget.setPointerCapture?.(e.pointerId)
  }
  const onMarkerMove = (e: PointerEvent<HTMLButtonElement>, key: string) => {
    if (drag.current !== key) return
    const p = posFromEvent(e)
    if (p) place(key, p.x, p.y)
  }
  const onMarkerUp = (e: PointerEvent<HTMLButtonElement>) => {
    drag.current = null
    e.currentTarget.releasePointerCapture?.(e.pointerId)
  }
  const onMarkerKey = (e: KeyboardEvent<HTMLButtonElement>, row: PlaceRow) => {
    if (!row.pos) return
    const step = e.shiftKey ? NUDGE_FAST : NUDGE
    let { plan_x: x, plan_y: y } = row.pos
    if (e.key === 'ArrowLeft') x -= step
    else if (e.key === 'ArrowRight') x += step
    else if (e.key === 'ArrowUp') y -= step
    else if (e.key === 'ArrowDown') y += step
    else if (e.key === 'Delete' || e.key === 'Backspace') {
      e.preventDefault()
      onPlace(row.key, null)
      return
    } else return
    e.preventDefault()
    place(row.key, clamp01(x), clamp01(y))
  }

  const onPlan = rows.map((r, i) => ({ r, i })).filter(({ r }) => r.pos?.floorplan_id === plan.id)
  const elsewhere = active?.pos && active.pos.floorplan_id !== plan.id ? sorted.find((p) => p.id === active.pos!.floorplan_id) : undefined
  const activeIndex = rows.findIndex((r) => r.key === active?.key)
  const setCoord = (axis: 'x' | 'y', raw: string) => {
    if (!active) return
    const v = Number(raw)
    if (raw.trim() === '' || !Number.isFinite(v)) return
    const cur = active.pos && active.pos.floorplan_id === plan.id ? active.pos : { plan_x: 0.5, plan_y: 0.5 }
    const rel = clamp01(v / 100)
    place(active.key, axis === 'x' ? rel : cur.plan_x, axis === 'y' ? rel : cur.plan_y)
  }
  const placedCount = rows.filter((r) => r.pos).length

  return (
    <div className="space-y-3">
      <p id={ids.help} className="text-sm text-gray-600">
        {t('p2.place.help')}
      </p>
      <div className="flex flex-wrap items-end gap-3">
        {sorted.length > 1 && (
          <div>
            <label htmlFor={ids.plan} className="mb-1.5 block text-sm font-medium text-gray-700">
              {t('p2.place.plan')}
            </label>
            <Select id={ids.plan} value={plan.id} onChange={(e) => setPlanChoice(e.target.value)} className="!h-9 !w-52 !py-1">
              {sorted.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </Select>
          </div>
        )}
        <div>
          <label htmlFor={ids.cp} className="mb-1.5 block text-sm font-medium text-gray-700">
            {t('p2.place.placing')}
          </label>
          <Select id={ids.cp} value={active?.key ?? ''} onChange={(e) => setActiveChoice(e.target.value)} className="!h-9 !w-64 max-w-full !py-1">
            {rows.map((r, i) => {
              const where = r.pos ? (sorted.find((p) => p.id === r.pos!.floorplan_id)?.name ?? '?') : t('p2.place.notPlaced')
              return (
                <option key={r.key} value={r.key}>
                  {`${i + 1}. ${r.name || '—'} (${where})`}
                </option>
              )
            })}
          </Select>
        </div>
        <div className="flex items-end gap-2">
          <div>
            <label htmlFor={ids.x} className="mb-1.5 block text-sm font-medium text-gray-700">
              {t('p2.place.x')}
            </label>
            <Input
              id={ids.x}
              type="number"
              inputMode="decimal"
              min={0}
              max={100}
              step={0.1}
              value={active?.pos && !elsewhere ? pctText(active.pos.plan_x) : ''}
              onChange={(e) => setCoord('x', e.target.value)}
              className="!h-9 w-24 !py-1 tabular-nums"
              placeholder="—"
            />
          </div>
          <div>
            <label htmlFor={ids.y} className="mb-1.5 block text-sm font-medium text-gray-700">
              {t('p2.place.y')}
            </label>
            <Input
              id={ids.y}
              type="number"
              inputMode="decimal"
              min={0}
              max={100}
              step={0.1}
              value={active?.pos && !elsewhere ? pctText(active.pos.plan_y) : ''}
              onChange={(e) => setCoord('y', e.target.value)}
              className="!h-9 w-24 !py-1 tabular-nums"
              placeholder="—"
            />
          </div>
          {active?.pos && (
            <Button variant="ghost" size="sm" className="mb-0.5" onClick={() => onPlace(active.key, null)}>
              <MapPinOff className="h-3.5 w-3.5" aria-hidden="true" />
              {t('p2.place.remove')}
            </Button>
          )}
        </div>
      </div>
      {elsewhere && (
        <p className="text-xs text-gray-600">
          {t('p2.place.elsewhere', { plan: elsewhere.name })}{' '}
          <button type="button" className="font-medium text-primary-700 underline underline-offset-2" onClick={() => place(active!.key, 0.5, 0.5)}>
            {t('p2.place.moveHere')}
          </button>
        </p>
      )}

      <PlanImage ref={box} plan={plan} alt={t('p2.place.alt', { plan: plan.name })} interactive onPointerDown={onImageDown}>
        <PlanPath points={onPlan.map(({ r }) => ({ x: r.pos!.plan_x, y: r.pos!.plan_y }))} />
        {onPlan.map(({ r, i }) => {
          const on = r.key === active?.key
          return (
            <button
              key={r.key}
              type="button"
              aria-label={t('p2.place.marker', { n: i + 1, name: r.name || '—', x: pctText(r.pos!.plan_x), y: pctText(r.pos!.plan_y) })}
              aria-describedby={ids.help}
              aria-pressed={on}
              onPointerDown={(e) => onMarkerDown(e, r.key)}
              onPointerMove={(e) => onMarkerMove(e, r.key)}
              onPointerUp={onMarkerUp}
              onPointerCancel={onMarkerUp}
              onFocus={() => setActiveChoice(r.key)}
              onKeyDown={(e) => onMarkerKey(e, r)}
              className={`absolute flex h-7 w-7 -translate-x-1/2 -translate-y-1/2 cursor-grab items-center justify-center rounded-full text-xs font-bold tabular-nums shadow-md ring-2 focus:outline-none focus-visible:ring-4 focus-visible:ring-primary-400 active:cursor-grabbing ${
                on ? 'bg-primary-600 text-white ring-white' : 'bg-white text-gray-900 ring-primary-600'
              }`}
              style={{ left: `${r.pos!.plan_x * 100}%`, top: `${r.pos!.plan_y * 100}%` }}
            >
              {i + 1}
            </button>
          )
        })}
      </PlanImage>
      <p className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-gray-500">
        <span className="inline-flex items-center gap-1">
          <Crosshair className="h-3.5 w-3.5" aria-hidden="true" />
          {active ? t('p2.place.next', { n: activeIndex + 1, name: active.name || '—' }) : null}
        </span>
        <span>{t('p2.place.count', { a: placedCount, n: rows.length })}</span>
      </p>
    </div>
  )
}
