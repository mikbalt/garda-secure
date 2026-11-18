import type { ReactNode } from 'react'
import { ChevronLeft, ChevronRight } from 'lucide-react'

/**
 * ◀ [Today] ▶ stepper for a day or week, with the period label beside it.
 * `children` can add e.g. a date input or supplementary controls.
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
