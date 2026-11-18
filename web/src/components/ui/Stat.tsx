import type { ReactNode } from 'react'
import type { LucideIcon } from 'lucide-react'

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

export interface StatProps {
  icon: LucideIcon
  label: string
  value: ReactNode
  sub?: ReactNode
  tone?: StatTone
  /** 0–100: renders a thin progress bar. */
  progress?: number
}

/** Summary stat tile with icon, label, value, optional progress bar and sub-line. */
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

/** Grid row of `Stat` tiles; shows a skeleton while loading. */
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
