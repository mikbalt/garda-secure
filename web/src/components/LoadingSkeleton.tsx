interface TableSkeletonProps {
  rows?: number
  cols?: number
}

/** Animated pulse skeleton for data tables. */
export function TableSkeleton({ rows = 5, cols = 5 }: TableSkeletonProps) {
  return (
    <div className="animate-pulse overflow-hidden rounded-2xl border border-gray-200/80 bg-white shadow-card" aria-hidden="true">
      <div className="flex gap-4 border-b border-gray-100 bg-gray-50/80 px-4 py-3">
        {Array.from({ length: cols }).map((_, i) => (
          <div key={i} className="h-3 flex-1 rounded bg-gray-200" />
        ))}
      </div>
      {Array.from({ length: rows }).map((_, r) => (
        <div key={r} className="flex gap-4 border-b border-gray-50 px-4 py-4 last:border-0">
          {Array.from({ length: cols }).map((_, c) => (
            <div key={c} className="h-3 flex-1 rounded bg-gray-100" />
          ))}
        </div>
      ))}
    </div>
  )
}

/** Animated pulse skeleton for stat/summary cards. */
export function CardSkeleton({ className = '' }: { className?: string }) {
  return (
    <div className={`animate-pulse rounded-2xl border border-gray-200/80 bg-white p-5 shadow-card ${className}`} aria-hidden="true">
      <div className="mb-4 h-8 w-8 rounded-lg bg-gray-100" />
      <div className="mb-2 h-3 w-1/3 rounded bg-gray-200" />
      <div className="h-6 w-1/2 rounded bg-gray-200" />
    </div>
  )
}
