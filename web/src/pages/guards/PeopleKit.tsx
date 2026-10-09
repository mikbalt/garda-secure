/**
 * Small building blocks shared by the people pages (Guards, Schedules,
 * Attendance). Candidates for promotion into components/ui.
 */
import type { ReactNode } from 'react'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { Button } from '../../components/ui/Button'
import { Modal } from '../../components/ui/Modal'
import { useT } from '../../i18n/context'
import { avatarTone, initials } from './peopleUtil'

const avatarSizes = { sm: 'h-7 w-7 text-[11px]', md: 'h-9 w-9 text-xs', lg: 'h-11 w-11 text-sm' }

/** Round initials avatar with a stable colour per name. */
export function Avatar({ name, size = 'md', muted = false }: { name?: string | null; size?: keyof typeof avatarSizes; muted?: boolean }) {
  return (
    <span
      aria-hidden="true"
      className={`inline-flex shrink-0 select-none items-center justify-center rounded-full font-semibold ring-1 ${avatarSizes[size]} ${
        muted ? 'bg-gray-100 text-gray-500 ring-gray-200' : avatarTone(name)
      }`}
    >
      {initials(name)}
    </span>
  )
}

/** Avatar + name + secondary line (employee id). */
export function PersonCell({
  name,
  sub,
  size = 'md',
  muted,
  extra,
}: {
  name: string
  sub?: ReactNode
  size?: keyof typeof avatarSizes
  muted?: boolean
  extra?: ReactNode
}) {
  return (
    <div className="flex min-w-0 items-center gap-3">
      <Avatar name={name} size={size} muted={muted} />
      <div className="min-w-0">
        <div className="flex min-w-0 items-center gap-2">
          <span className={`truncate font-medium ${muted ? 'text-gray-500' : 'text-gray-900'}`}>{name}</span>
          {extra}
        </div>
        {sub && <div className="truncate text-xs text-gray-500">{sub}</div>}
      </div>
    </div>
  )
}

export interface ChipItem<K extends string> {
  key: K
  label: string
  count?: number
  /** Dot colour class (e.g. `bg-emerald-500`). */
  dot?: string
}

/**
 * Toggle chips that filter a list ("Semua 12 · Hadir 8 · …"). Exactly one is
 * active; each button reports its pressed state to screen readers.
 */
export function FilterChips<K extends string>({
  items,
  active,
  onChange,
  label,
}: {
  items: ChipItem<K>[]
  active: K
  onChange: (k: K) => void
  label: string
}) {
  return (
    <div role="group" aria-label={label} className="flex flex-wrap gap-2">
      {items.map((it) => {
        const on = it.key === active
        return (
          <button
            key={it.key}
            type="button"
            aria-pressed={on}
            onClick={() => onChange(it.key)}
            className={`inline-flex h-8 items-center gap-2 rounded-full px-3 text-sm font-medium transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-1 ${
              on
                ? 'bg-ink-900 text-white shadow-sm'
                : 'bg-white text-gray-700 ring-1 ring-inset ring-gray-200 hover:bg-gray-50 hover:ring-gray-300'
            }`}
          >
            {it.dot && <span className={`h-2 w-2 rounded-full ${it.dot}`} aria-hidden="true" />}
            {it.label}
            {it.count !== undefined && (
              <span
                className={`min-w-[1.25rem] rounded-full px-1.5 text-center text-xs tabular-nums ${
                  on ? 'bg-white/15 text-white' : 'bg-gray-100 text-gray-600'
                }`}
              >
                {it.count}
              </span>
            )}
          </button>
        )
      })}
    </div>
  )
}

/**
 * ◀ [Today] ▶ stepper for a day or week, with the period label beside it.
 * `children` can add e.g. a date input.
 */
export function PeriodStepper({
  label,
  sublabel,
  onPrev,
  onNext,
  onToday,
  isCurrent,
  todayLabel,
  prevLabel,
  nextLabel,
  children,
}: {
  label: string
  sublabel?: ReactNode
  onPrev: () => void
  onNext: () => void
  onToday: () => void
  isCurrent: boolean
  todayLabel: string
  prevLabel: string
  nextLabel: string
  children?: ReactNode
}) {
  return (
    <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
      <div className="inline-flex items-center rounded-lg bg-white shadow-sm ring-1 ring-inset ring-gray-300">
        <button
          type="button"
          onClick={onPrev}
          aria-label={prevLabel}
          title={prevLabel}
          className="inline-flex h-9 w-9 items-center justify-center rounded-l-lg text-gray-600 hover:bg-gray-50 hover:text-gray-900 focus:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary-500"
        >
          <ChevronLeft className="h-4 w-4" aria-hidden="true" />
        </button>
        <button
          type="button"
          onClick={onToday}
          disabled={isCurrent}
          className="h-9 border-x border-gray-200 px-3 text-sm font-medium text-gray-800 hover:bg-gray-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary-500 disabled:cursor-default disabled:text-gray-400 disabled:hover:bg-transparent"
        >
          {todayLabel}
        </button>
        <button
          type="button"
          onClick={onNext}
          aria-label={nextLabel}
          title={nextLabel}
          className="inline-flex h-9 w-9 items-center justify-center rounded-r-lg text-gray-600 hover:bg-gray-50 hover:text-gray-900 focus:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary-500"
        >
          <ChevronRight className="h-4 w-4" aria-hidden="true" />
        </button>
      </div>
      <div className="min-w-0" aria-live="polite">
        <p className="text-[15px] font-semibold leading-tight text-gray-900">{label}</p>
        {sublabel && <p className="text-xs text-gray-500">{sublabel}</p>}
      </div>
      {children}
    </div>
  )
}

/** Confirmation dialog for destructive actions (replaces window.confirm). */
export function ConfirmDialog({
  title,
  message,
  confirmLabel,
  onConfirm,
  onClose,
  loading,
  error,
}: {
  title: string
  message: ReactNode
  confirmLabel: string
  onConfirm: () => void
  onClose: () => void
  loading?: boolean
  error?: ReactNode
}) {
  const t = useT()
  return (
    <Modal
      open
      onClose={onClose}
      title={title}
      footer={
        <>
          <Button variant="secondary" onClick={onClose} data-autofocus>
            {t('common.cancel')}
          </Button>
          <Button variant="danger" loading={loading} onClick={onConfirm}>
            {confirmLabel}
          </Button>
        </>
      }
    >
      <div className="space-y-3 text-sm text-gray-600">
        {error}
        <div>{message}</div>
      </div>
    </Modal>
  )
}
