/**
 * Small building blocks shared by the Patrol and Incidents pages (date range
 * filter, chip/segmented controls, live indicator, checkpoint bar, avatar).
 * General-purpose components are promoted to components/ui and re-exported
 * here for backward compatibility.
 */
import { useId, type ReactNode } from 'react'
import type { LucideIcon } from 'lucide-react'
import { Input } from '../../components/ui/Form'
import { useI18n } from '../../i18n/context'
import { cpTone, matchPreset, presetRange, type RangePreset } from './format'
import type { CheckpointProgress } from '../../types'

// Re-exports from components/ui — import directly from there in new code.
export { Segmented, ChipGroup, type ChipOption } from '../../components/ui/FilterChips'
export { LiveIndicator } from '../../components/ui/LiveIndicator'
export { Avatar } from '../../components/ui/Avatar'

const groupLabel = 'mb-1.5 block text-sm font-medium text-gray-700'

// ── Date range ───────────────────────────────────────────────────────────────

/** Period filter: quick presets (today / 7 / 30 days) plus explicit dates. */
export function DateRangeFilter({
  from,
  to,
  tz,
  onChange,
}: {
  from: string
  to: string
  tz: string
  onChange: (r: { from: string; to: string }) => void
}) {
  const { t } = useI18n()
  const id = useId()
  const preset = matchPreset(from, to, tz)
  const presets: { value: RangePreset; label: string }[] = [
    { value: 'today', label: t('ops.range.today') },
    { value: '7d', label: t('ops.range.7d') },
    { value: '30d', label: t('ops.range.30d') },
  ]
  return (
    <div role="group" aria-labelledby={id} className="min-w-0 max-w-full">
      <span id={id} className={groupLabel}>
        {t('ops.range.label')}
      </span>
      <div className="flex flex-wrap items-center gap-2">
        <div className="inline-flex gap-0.5 rounded-lg bg-gray-100 p-0.5 ring-1 ring-inset ring-gray-200/70">
          {presets.map((p) => (
            <button
              key={p.value}
              type="button"
              aria-pressed={preset === p.value}
              onClick={() => onChange(presetRange(p.value, tz))}
              className={`h-8 whitespace-nowrap rounded-md px-3 text-sm font-medium transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 ${
                preset === p.value ? 'bg-white text-gray-900 shadow-card' : 'text-gray-600 hover:text-gray-900'
              }`}
            >
              {p.label}
            </button>
          ))}
        </div>
        <div className="flex w-full min-w-0 items-center gap-1.5 sm:w-auto">
          <Input
            type="date"
            aria-label={t('common.from')}
            value={from}
            max={to || undefined}
            onChange={(e) => onChange({ from: e.target.value, to })}
            className="!h-9 min-w-0 flex-1 !py-1 sm:w-[9.5rem] sm:flex-none"
          />
          <span className="text-gray-500" aria-hidden="true">
            –
          </span>
          <Input
            type="date"
            aria-label={t('common.to')}
            value={to}
            min={from || undefined}
            onChange={(e) => onChange({ from, to: e.target.value })}
            className="!h-9 min-w-0 flex-1 !py-1 sm:w-[9.5rem] sm:flex-none"
          />
        </div>
      </div>
    </div>
  )
}

// ── Checkpoint progress ──────────────────────────────────────────────────────

/** One segment per checkpoint, coloured by status. */
export function SegmentedBar({ checkpoints, label, thick = false }: { checkpoints: CheckpointProgress[]; label: string; thick?: boolean }) {
  const { t } = useI18n()
  return (
    <div role="img" aria-label={label} className="flex items-center gap-1">
      {checkpoints.map((c) => (
        <span
          key={c.id}
          title={`${c.order_index + 1}. ${c.name} — ${t(`status.${c.status}`)}`}
          className={`${thick ? 'h-2.5' : 'h-2'} flex-1 rounded-full ${cpTone[c.status]?.bar ?? cpTone.pending.bar}`}
        />
      ))}
    </div>
  )
}

/** Legend for the checkpoint colours. */
export function CheckpointLegend() {
  const { t } = useI18n()
  const items = [
    ['on_time', 'bg-emerald-500'],
    ['late', 'bg-amber-400'],
    ['overdue', 'bg-orange-500'],
    ['missed', 'bg-red-500'],
    ['pending', 'bg-gray-300'],
  ] as const
  return (
    <ul className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-gray-500" aria-label={t('ops.pat.legend')}>
      {items.map(([s, cls]) => (
        <li key={s} className="inline-flex items-center gap-1.5">
          <span className={`h-2 w-4 rounded-full ${cls}`} aria-hidden="true" />
          {t(`status.${s}`)}
        </li>
      ))}
    </ul>
  )
}

/** Simple bar for a percentage. */
export function MiniBar({ pct, label }: { pct: number; label: string }) {
  const v = Math.max(0, Math.min(100, Math.round(pct)))
  const tone = v >= 100 ? 'bg-emerald-500' : v >= 60 ? 'bg-primary-500' : 'bg-amber-400'
  return (
    <div role="progressbar" aria-valuenow={v} aria-valuemin={0} aria-valuemax={100} aria-label={label} className="h-1.5 w-full overflow-hidden rounded-full bg-gray-100">
      <div className={`h-full rounded-full ${tone}`} style={{ width: `${v}%` }} />
    </div>
  )
}

/** Small "meta" line item with an icon. */
export function Meta({ icon: Icon, children }: { icon: LucideIcon; children: ReactNode }) {
  return (
    <span className="inline-flex min-w-0 items-center gap-1">
      <Icon className="h-3.5 w-3.5 shrink-0 text-gray-500" aria-hidden="true" />
      <span className="truncate">{children}</span>
    </span>
  )
}
