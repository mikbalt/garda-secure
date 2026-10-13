import { useMemo, useState } from 'react'
import { AlertTriangle, Check, ChevronsRight, X, type LucideIcon } from 'lucide-react'
import { useI18n, type Key } from '../../../i18n/context'
import { formatTime } from '../../../lib/time'
import { Segmented } from '../kit'
import { PlanImage, PlanPath } from './PlanImage'
import { planPos, useFloorplans } from './floorplans'
import { planState, type PlanState } from './planState'
import type { SessionProgress, Site } from '../../../types'

const STATE: Record<PlanState, { cls: string; icon?: LucideIcon; label: Key }> = {
  scanned: { cls: 'bg-emerald-700 text-white ring-white', icon: Check, label: 'p2.live.st.scanned' },
  late: { cls: 'bg-amber-500 text-gray-950 ring-white', icon: Check, label: 'p2.live.st.late' },
  next: { cls: 'bg-primary-600 text-white ring-primary-300 outline outline-2 outline-offset-2 outline-primary-600', icon: ChevronsRight, label: 'p2.live.st.next' },
  overdue: { cls: 'bg-orange-700 text-white ring-white', icon: AlertTriangle, label: 'p2.live.st.overdue' },
  missed: { cls: 'bg-red-600 text-white ring-white', icon: X, label: 'p2.live.st.missed' },
  pending: { cls: 'bg-white text-gray-800 ring-gray-400', label: 'p2.live.st.pending' },
}

function Marker({ state, n, x, y }: { state: PlanState; n: number; x: number; y: number }) {
  const s = STATE[state]
  const Icon = s.icon
  return (
    <span
      aria-hidden="true"
      className={`absolute flex h-7 min-w-[1.75rem] -translate-x-1/2 -translate-y-1/2 items-center justify-center gap-0.5 rounded-full px-1 text-[11px] font-bold tabular-nums shadow-md ring-2 ${s.cls}`}
      style={{ left: `${x * 100}%`, top: `${y * 100}%` }}
    >
      {Icon && <Icon className="h-3 w-3" strokeWidth={3} />}
      {n}
    </span>
  )
}

/** Legend: colour + icon per state (colour is never the only signal). */
export function PlanLegend() {
  const { t } = useI18n()
  return (
    <ul className="flex flex-wrap items-center gap-x-4 gap-y-1.5 text-xs text-gray-600" aria-label={t('ops.pat.legend')}>
      {(Object.keys(STATE) as PlanState[]).map((k) => {
        const s = STATE[k]
        const Icon = s.icon
        return (
          <li key={k} className="inline-flex items-center gap-1.5">
            <span className={`flex h-4 min-w-[1rem] items-center justify-center rounded-full px-0.5 ring-2 ${s.cls}`} aria-hidden="true">
              {Icon ? <Icon className="h-2.5 w-2.5" strokeWidth={3} /> : <span className="text-[9px] font-bold">1</span>}
            </span>
            {t(s.label)}
          </li>
        )
      })}
    </ul>
  )
}

/**
 * Session progress drawn on the site's floor plans: one tab per plan the
 * route's checkpoints are placed on, markers numbered by order and coloured
 * by state, the route path, and a text list mirroring the picture.
 */
export function LivePlan({ site, progress, lastScanAt }: { site: Site; progress: SessionProgress; lastScanAt?: string | null }) {
  const { t } = useI18n()
  const tz = site.timezone
  const { data: plans = [], isLoading } = useFloorplans(site.id)
  const items = useMemo(
    () => progress.checkpoints.map((cp, i) => ({ cp, i, pos: planPos(cp), state: planState(cp, i, progress.next_index) })),
    [progress]
  )
  const planIds = useMemo(() => Array.from(new Set(items.map((x) => x.pos?.floorplan_id).filter((v): v is string => !!v))), [items])
  const shown = plans.filter((p) => planIds.includes(p.id))
  const nextPlan = items.find((x) => x.state === 'next' || x.state === 'overdue')?.pos?.floorplan_id
  const [choice, setChoice] = useState<string | null>(null)
  const planId = choice && shown.some((p) => p.id === choice) ? choice : (shown.find((p) => p.id === nextPlan)?.id ?? shown[0]?.id)
  const plan = shown.find((p) => p.id === planId)
  const last = lastScanAt ?? progress.checkpoints.map((c) => c.scanned_at).filter((v): v is string => !!v).sort().pop()

  const onPlan = items.filter((x) => x.pos?.floorplan_id === planId)
  let walked = -1
  onPlan.forEach((x, k) => {
    if (x.state === 'scanned' || x.state === 'late') walked = k
  })
  const unplaced = items.filter((x) => !x.pos).length
  const stateText = (x: (typeof items)[number]) => {
    const base = t(STATE[x.state].label)
    return x.cp.scanned_at ? `${base} · ${formatTime(x.cp.scanned_at, tz)}` : x.cp.due_at && x.state !== 'missed' ? `${base} · ${t('pat.due', { time: formatTime(x.cp.due_at, tz) })}` : base
  }

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2 text-sm">
        <p className="text-gray-600">
          {last ? t('p2.live.lastScan', { time: formatTime(last, tz) }) : t('p2.live.noScanYet')}
          <span className="text-gray-500"> · {t('ops.pat.cpOf', { a: progress.scanned, n: progress.total })}</span>
        </p>
        {shown.length > 1 && (
          <Segmented
            label={t('p2.live.plans')}
            hideLabel
            size="sm"
            value={planId ?? ''}
            onChange={setChoice}
            options={shown.map((p) => ({ value: p.id, label: p.name, count: items.filter((x) => x.pos?.floorplan_id === p.id).length }))}
          />
        )}
      </div>
      {isLoading ? (
        <div className="aspect-[3/2] animate-pulse rounded-xl bg-gray-100" aria-hidden="true" />
      ) : plan ? (
        <PlanImage plan={plan} alt={t('p2.live.alt', { plan: plan.name, route: progress.route_name })}>
          <PlanPath points={onPlan.map((x) => ({ x: x.pos!.plan_x, y: x.pos!.plan_y }))} dashedFrom={Math.max(0, walked)} />
          {onPlan.map((x) => (
            <Marker key={x.cp.id} state={x.state} n={x.i + 1} x={x.pos!.plan_x} y={x.pos!.plan_y} />
          ))}
        </PlanImage>
      ) : (
        <p className="rounded-xl border border-dashed border-gray-300 px-4 py-6 text-center text-sm text-gray-600">{t('p2.live.planMissing')}</p>
      )}
      <PlanLegend />
      <div>
        <h4 className="mb-1.5 text-xs font-semibold uppercase tracking-wider text-gray-500">{t('p2.live.listTitle')}</h4>
        <ol className="divide-y divide-gray-100 rounded-xl border border-gray-200/80 bg-white text-sm">
          {items.map((x) => {
            const planName = x.pos ? plans.find((p) => p.id === x.pos!.floorplan_id)?.name : undefined
            return (
              <li key={x.cp.id} className="flex flex-wrap items-center gap-x-3 gap-y-0.5 px-3 py-2">
                <span className="w-6 text-xs font-semibold tabular-nums text-gray-500">{x.i + 1}.</span>
                <span className="min-w-0 flex-1 font-medium text-gray-900">{x.cp.name}</span>
                <span className={`text-xs font-medium ${x.state === 'overdue' ? 'text-orange-700' : x.state === 'missed' ? 'text-red-700' : x.state === 'next' ? 'text-primary-700' : 'text-gray-600'}`}>
                  {stateText(x)}
                </span>
                <span className="w-full pl-9 text-xs text-gray-500 sm:w-auto sm:pl-0">{planName ?? t('p2.live.notOnPlan')}</span>
              </li>
            )
          })}
        </ol>
        {unplaced > 0 && <p className="mt-1.5 text-xs text-gray-500">{t('p2.live.unplaced', { n: unplaced })}</p>}
      </div>
    </div>
  )
}
