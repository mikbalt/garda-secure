/**
 * Small building blocks shared by the Patrol and Incidents pages (date range
 * filter, chip/segmented controls, live indicator, checkpoint bar, avatar).
 * Candidates for promotion to components/ui once other pages need them.
 */
import { useId, type ReactNode } from 'react'
import type { LucideIcon } from 'lucide-react'
import { Dot, type Tone } from '../../components/ui/Badge'
import { Input } from '../../components/ui/Form'
import { useI18n } from '../../i18n/context'
import { cpTone, initials, matchPreset, presetRange, relTime, useNow, type RangePreset } from './format'
import type { CheckpointProgress } from '../../types'

const groupLabel = 'mb-1.5 block text-sm font-medium text-gray-700'

// ── Segmented control / chips ────────────────────────────────────────────────

export interface ChipOption<V extends string> {
  value: V
  label: string
  count?: number
  tone?: Tone
}

/** Single-select segmented control (pill group). Buttons use aria-pressed. */
export function Segmented<V extends string>({
  label,
  options,
  value,
  onChange,
  hideLabel = false,
  size = 'md',
  disabled = false,
}: {
  label: string
  options: ChipOption<V>[]
  value: V
  onChange: (v: V) => void
  hideLabel?: boolean
  size?: 'sm' | 'md'
  disabled?: boolean
}) {
  const id = useId()
  return (
    <div>
      <span id={id} className={hideLabel ? 'sr-only' : groupLabel}>
        {label}
      </span>
      <div role="group" aria-labelledby={id} className="inline-flex max-w-full flex-wrap gap-0.5 rounded-lg bg-gray-100 p-0.5 ring-1 ring-inset ring-gray-200/70">
        {options.map((o) => {
          const on = o.value === value
          return (
            <button
              key={o.value}
              type="button"
              aria-pressed={on}
              disabled={disabled}
              onClick={() => onChange(o.value)}
              className={`inline-flex items-center gap-1.5 whitespace-nowrap rounded-md font-medium disabled:cursor-wait disabled:opacity-60 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 ${
                size === 'sm' ? 'h-7 px-2.5 text-xs' : 'h-8 px-3 text-sm'
              } ${on ? 'bg-white text-gray-900 shadow-card' : 'text-gray-600 hover:text-gray-900'}`}
            >
              {o.tone && <Dot tone={o.tone} />}
              {o.label}
              {o.count !== undefined && (
                <span className={`rounded-full px-1.5 text-[11px] tabular-nums ${on ? 'bg-gray-100 text-gray-700' : 'bg-gray-200/70 text-gray-500'}`}>{o.count}</span>
              )}
            </button>
          )
        })}
      </div>
    </div>
  )
}

/** Toggle chips (single select with "all"), e.g. severity filter. */
export function ChipGroup<V extends string>({
  label,
  options,
  value,
  onChange,
}: {
  label: string
  options: ChipOption<V>[]
  value: V
  onChange: (v: V) => void
}) {
  const id = useId()
  return (
    <div>
      <span id={id} className={groupLabel}>
        {label}
      </span>
      <div role="group" aria-labelledby={id} className="flex flex-wrap gap-1.5">
        {options.map((o) => {
          const on = o.value === value
          return (
            <button
              key={o.value}
              type="button"
              aria-pressed={on}
              onClick={() => onChange(o.value)}
              className={`inline-flex h-8 items-center gap-1.5 rounded-full px-3 text-sm font-medium ring-1 ring-inset transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 ${
                on ? 'bg-gray-900 text-white ring-gray-900' : 'bg-white text-gray-700 ring-gray-300 hover:bg-gray-50'
              }`}
            >
              {o.tone && <Dot tone={o.tone} />}
              {o.label}
            </button>
          )
        })}
      </div>
    </div>
  )
}

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
          <span className="text-gray-400" aria-hidden="true">
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

// ── Live indicator ───────────────────────────────────────────────────────────

/** Pulsing dot + "Updated 4 seconds ago" (ticks every second). */
export function LiveIndicator({ updatedAt, fetching }: { updatedAt: number; fetching?: boolean }) {
  const { t, lang } = useI18n()
  const now = useNow(1000)
  return (
    <span className="inline-flex items-center gap-2 rounded-full bg-white px-3 py-1.5 text-xs font-medium text-gray-600 shadow-card ring-1 ring-gray-200/80">
      <Dot tone="green" pulse />
      <span aria-live="off">
        {updatedAt ? t('ops.live.updated', { when: relTime(updatedAt, lang, Math.max(now, updatedAt)) }) : t('ops.live.connecting')}
      </span>
      {fetching && <span className="sr-only">{t('app.loading')}</span>}
    </span>
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

export function Avatar({ name, className = '' }: { name?: string; className?: string }) {
  return (
    <span
      className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-gradient-to-br from-primary-50 to-primary-100 text-xs font-semibold text-primary-700 ring-1 ring-primary-100 ${className}`}
      aria-hidden="true"
    >
      {initials(name)}
    </span>
  )
}

/** Small "meta" line item with an icon. */
export function Meta({ icon: Icon, children }: { icon: LucideIcon; children: ReactNode }) {
  return (
    <span className="inline-flex min-w-0 items-center gap-1">
      <Icon className="h-3.5 w-3.5 shrink-0 text-gray-400" aria-hidden="true" />
      <span className="truncate">{children}</span>
    </span>
  )
}
