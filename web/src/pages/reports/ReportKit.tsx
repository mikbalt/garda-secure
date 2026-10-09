import { useId, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { ArrowRight, ChevronLeft, ChevronRight, type LucideIcon } from 'lucide-react'
import { Input } from '../../components/ui/Form'
import { EmptyState } from '../../components/ui/States'
import { buttonClass } from '../../components/ui/styles'
import { useI18n } from '../../i18n/context'
import { addDays, daysBetween } from '../../lib/time'
import { addMonths, monthEnd, pct, pctTone } from './format'

// ── summary tiles ────────────────────────────────────────────────────────────

type StatTone = 'gray' | 'green' | 'blue' | 'amber' | 'red' | 'violet' | 'orange'

const statTone: Record<StatTone, string> = {
  gray: 'bg-gray-100 text-gray-600 ring-gray-200',
  green: 'bg-emerald-50 text-emerald-600 ring-emerald-100',
  blue: 'bg-primary-50 text-primary-600 ring-primary-100',
  amber: 'bg-amber-50 text-amber-600 ring-amber-100',
  red: 'bg-red-50 text-red-600 ring-red-100',
  violet: 'bg-violet-50 text-violet-600 ring-violet-100',
  orange: 'bg-orange-50 text-orange-600 ring-orange-100',
}

const barTone: Record<StatTone, string> = {
  gray: 'bg-gray-400',
  green: 'bg-emerald-500',
  blue: 'bg-primary-500',
  amber: 'bg-amber-500',
  red: 'bg-red-500',
  violet: 'bg-violet-500',
  orange: 'bg-orange-500',
}

interface StatProps {
  icon: LucideIcon
  label: string
  value: ReactNode
  sub?: ReactNode
  tone?: StatTone
  /** 0–100: renders a thin progress bar. */
  progress?: number
}

export function Stat({ icon: Icon, label, value, sub, tone = 'gray', progress }: StatProps) {
  return (
    <div className="rounded-2xl border border-gray-200/80 bg-white p-4 shadow-card print:break-inside-avoid print:p-2 print:shadow-none">
      <div className="flex items-center gap-2.5">
        <span className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-lg ring-1 print:hidden ${statTone[tone]}`}>
          <Icon className="h-4 w-4" aria-hidden="true" />
        </span>
        <p className="min-w-0 text-sm font-medium leading-tight text-gray-500">{label}</p>
      </div>
      <p className="mt-3 text-2xl font-semibold tabular-nums tracking-tight text-gray-900 print:mt-1 print:text-lg">{value}</p>
      {progress !== undefined && (
        <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-gray-100 print:hidden">
          <div className={`h-full rounded-full ${barTone[tone]}`} style={{ width: `${Math.max(0, Math.min(100, progress))}%` }} />
        </div>
      )}
      {sub && <p className="mt-1.5 text-xs text-gray-500">{sub}</p>}
    </div>
  )
}

export function StatRow({ children, loading }: { children: ReactNode; loading?: boolean }) {
  if (loading) {
    return (
      <div className="mb-4 grid grid-cols-2 gap-3 lg:grid-cols-4" aria-hidden="true">
        {Array.from({ length: 4 }).map((_, i) => (
          <div key={i} className="h-[118px] animate-pulse rounded-2xl border border-gray-200/80 bg-white shadow-card" />
        ))}
      </div>
    )
  }
  return <div className="mb-4 grid grid-cols-2 gap-3 lg:grid-cols-4 print:grid-cols-4 print:gap-2">{children}</div>
}

const pctText = { green: 'text-emerald-700', yellow: 'text-amber-700', red: 'text-red-700' }
const pctBar = { green: 'bg-emerald-500', yellow: 'bg-amber-400', red: 'bg-red-500' }

/** Percentage with a small traffic-light bar (for compliance columns). */
export function PctCell({ value }: { value: number }) {
  const { lang } = useI18n()
  const tone = pctTone(value)
  return (
    <span className="inline-flex items-center justify-end gap-2">
      <span className="hidden h-1.5 w-14 overflow-hidden rounded-full bg-gray-100 sm:inline-block print:hidden" aria-hidden="true">
        <span className={`block h-full rounded-full ${pctBar[tone]}`} style={{ width: `${Math.max(0, Math.min(100, value))}%` }} />
      </span>
      <span className={`font-medium ${pctText[tone]}`}>{pct(value, lang)}</span>
    </span>
  )
}

/** Teaching empty state for a report with an optional call to action. */
export function ReportEmpty({ icon, title, message, cta }: { icon?: LucideIcon; title: string; message: string; cta?: { to: string; label: string } }) {
  return (
    <EmptyState icon={icon} title={title} message={message}>
      {cta && (
        <Link to={cta.to} className={buttonClass('secondary', 'md')}>
          {cta.label}
          <ArrowRight className="h-4 w-4" aria-hidden="true" />
        </Link>
      )}
    </EmptyState>
  )
}

// ── period pickers ───────────────────────────────────────────────────────────

const stepBtn = buttonClass('secondary', 'md', 'w-9 !px-0')

function Stepper({
  label,
  prevLabel,
  nextLabel,
  onPrev,
  onNext,
  nextDisabled,
  children,
  extra,
}: {
  label: string
  prevLabel: string
  nextLabel: string
  onPrev: () => void
  onNext: () => void
  nextDisabled?: boolean
  children: (id: string) => ReactNode
  extra?: ReactNode
}) {
  const id = useId()
  return (
    <div>
      <label htmlFor={id} className="mb-1.5 block text-sm font-medium text-gray-700">
        {label}
      </label>
      <div className="flex items-center gap-1.5">
        <button type="button" className={stepBtn} onClick={onPrev} aria-label={prevLabel} title={prevLabel}>
          <ChevronLeft className="h-4 w-4" aria-hidden="true" />
        </button>
        {children(id)}
        <button type="button" className={stepBtn} onClick={onNext} disabled={nextDisabled} aria-label={nextLabel} title={nextLabel}>
          <ChevronRight className="h-4 w-4" aria-hidden="true" />
        </button>
        {extra}
      </div>
    </div>
  )
}

/** ◀ date ▶ [Today] */
export function DayPicker({ value, onChange, today }: { value: string; onChange: (d: string) => void; today: string }) {
  const { t } = useI18n()
  return (
    <Stepper
      label={t('common.date')}
      prevLabel={t('adm.rep.prevDay')}
      nextLabel={t('adm.rep.nextDay')}
      onPrev={() => value && onChange(addDays(value, -1))}
      onNext={() => value && onChange(addDays(value, 1))}
      nextDisabled={!value || value >= today}
      extra={
        value !== today && (
          <button type="button" className={buttonClass('ghost', 'md')} onClick={() => onChange(today)}>
            {t('adm.rep.today')}
          </button>
        )
      }
    >
      {(id) => <Input id={id} type="date" value={value} max={today} onChange={(e) => onChange(e.target.value)} className="w-[10.5rem]" />}
    </Stepper>
  )
}

/** ◀ month ▶ [This month] */
export function MonthPicker({ value, onChange, current }: { value: string; onChange: (m: string) => void; current: string }) {
  const { t } = useI18n()
  const ok = /^\d{4}-\d{2}$/.test(value)
  return (
    <Stepper
      label={t('common.month')}
      prevLabel={t('adm.rep.prevMonth')}
      nextLabel={t('adm.rep.nextMonth')}
      onPrev={() => ok && onChange(addMonths(value, -1))}
      onNext={() => ok && onChange(addMonths(value, 1))}
      nextDisabled={!ok || value >= current}
      extra={
        value !== current && (
          <button type="button" className={buttonClass('ghost', 'md')} onClick={() => onChange(current)}>
            {t('adm.rep.thisMonth')}
          </button>
        )
      }
    >
      {(id) => <Input id={id} type="month" value={value} max={current} onChange={(e) => onChange(e.target.value)} className="w-[10.5rem]" />}
    </Stepper>
  )
}

export interface Range {
  from: string
  to: string
}

/** From / to dates plus quick presets (7 days, 30 days, this month, last month). */
export function RangePicker({ value, onChange, today }: { value: Range; onChange: (r: Range) => void; today: string }) {
  const { t } = useI18n()
  const fromId = useId()
  const toId = useId()
  const month = today.slice(0, 7)
  const prev = addMonths(month, -1)
  const presets: { key: string; label: string; range: Range }[] = [
    { key: '7', label: t('adm.rep.last7'), range: { from: addDays(today, -6), to: today } },
    { key: '30', label: t('adm.rep.last30'), range: { from: addDays(today, -29), to: today } },
    { key: 'month', label: t('adm.rep.thisMonth'), range: { from: `${month}-01`, to: today } },
    { key: 'prev', label: t('adm.rep.lastMonth'), range: { from: `${prev}-01`, to: monthEnd(prev) } },
  ]
  const days = value.from && value.to && value.from <= value.to ? daysBetween(value.from, value.to) : 0
  return (
    <>
      <div>
        <label htmlFor={fromId} className="mb-1.5 block text-sm font-medium text-gray-700">
          {t('common.from')}
        </label>
        <Input id={fromId} type="date" value={value.from} max={value.to || today} onChange={(e) => onChange({ ...value, from: e.target.value })} className="w-[10.5rem]" />
      </div>
      <div>
        <label htmlFor={toId} className="mb-1.5 block text-sm font-medium text-gray-700">
          {t('common.to')}
        </label>
        <Input id={toId} type="date" value={value.to} min={value.from} max={today} onChange={(e) => onChange({ ...value, to: e.target.value })} className="w-[10.5rem]" />
      </div>
      <div role="group" aria-label={t('adm.rep.quickRange')}>
        <p className="mb-1.5 text-sm font-medium text-gray-700" aria-hidden="true">
          {t('adm.rep.quickRange')}
          {days > 0 && <span className="ml-1.5 font-normal text-gray-400">· {t('adm.rep.nDays', { n: days })}</span>}
        </p>
        <div className="flex flex-wrap gap-1 rounded-lg bg-gray-100 p-0.5">
          {presets.map((p) => {
            const active = p.range.from === value.from && p.range.to === value.to
            return (
              <button
                key={p.key}
                type="button"
                aria-pressed={active}
                onClick={() => onChange(p.range)}
                className={`h-8 rounded-md px-2.5 text-xs font-medium transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 ${
                  active ? 'bg-white text-gray-900 shadow-card' : 'text-gray-600 hover:text-gray-900'
                }`}
              >
                {p.label}
              </button>
            )
          })}
        </div>
      </div>
    </>
  )
}
