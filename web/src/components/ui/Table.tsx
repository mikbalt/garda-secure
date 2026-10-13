import type { ReactNode } from 'react'
import { TableSkeleton } from '../LoadingSkeleton'
import { EmptyState, ErrorState } from './States'

export interface Column<T> {
  key: string
  header: ReactNode
  cell: (row: T) => ReactNode
  className?: string
  /** Visually right-aligned numeric column. */
  numeric?: boolean
}

interface TableProps<T> {
  columns: Column<T>[]
  rows: T[] | undefined
  rowKey: (row: T) => string
  isLoading?: boolean
  error?: unknown
  empty?: string
  caption?: string
  onRetry?: () => void
  rowClassName?: (row: T) => string
  /** Optional totals row (<tfoot>), keyed by column key. */
  footer?: Partial<Record<string, ReactNode>>
}

/** Simple responsive data table with loading / error / empty states. */
export function Table<T>({ columns, rows, rowKey, isLoading, error, empty, caption, onRetry, rowClassName, footer }: TableProps<T>) {
  if (isLoading) return <TableSkeleton cols={Math.min(columns.length, 6)} />
  if (error) return <ErrorState error={error} onRetry={onRetry} />
  if (!rows || rows.length === 0) return <EmptyState message={empty} />
  return (
    // Focusable so keyboard users can scroll wide tables (axe scrollable-region-focusable).
    <div
      tabIndex={0}
      role="region"
      aria-label={caption}
      className="scroll-thin relative overflow-x-auto rounded-2xl border border-gray-200/80 bg-white shadow-card print-plain focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
    >
      <table className="min-w-full text-sm">
        {caption && <caption className="sr-only">{caption}</caption>}
        <thead className="border-b border-gray-200 bg-gray-50/80">
          <tr>
            {columns.map((c) => (
              <th
                key={c.key}
                scope="col"
                className={`whitespace-nowrap px-4 py-2.5 text-[11px] font-semibold uppercase tracking-wider text-gray-500 ${
                  c.numeric ? 'text-right' : 'text-left'
                } ${c.className ?? ''}`}
              >
                {c.header}
              </th>
            ))}
          </tr>
        </thead>
        <tbody className="divide-y divide-gray-100">
          {rows.map((row) => (
            <tr key={rowKey(row)} className={`transition-colors hover:bg-gray-50/80 ${rowClassName?.(row) ?? ''}`}>
              {columns.map((c) => (
                <td
                  key={c.key}
                  className={`px-4 py-3 align-middle text-gray-800 ${c.numeric ? 'text-right tabular-nums' : ''} ${c.className ?? ''}`}
                >
                  {c.cell(row)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
        {footer && (
          <tfoot className="border-t-2 border-gray-200 bg-gray-50/80 font-semibold text-gray-900">
            <tr>
              {columns.map((c, i) => {
                const Cell = i === 0 ? 'th' : 'td'
                return (
                  <Cell
                    key={c.key}
                    {...(i === 0 ? { scope: 'row' } : {})}
                    className={`px-4 py-3 align-middle ${c.numeric ? 'text-right tabular-nums' : 'text-left'} ${c.className ?? ''}`}
                  >
                    {footer[c.key]}
                  </Cell>
                )
              })}
            </tr>
          </tfoot>
        )}
      </table>
    </div>
  )
}
