import { CheckCircle2, Clock, AlertTriangle, XCircle, Circle } from 'lucide-react'
import { StatusBadge } from '../../components/ui/Badge'
import { useI18n } from '../../i18n/context'
import { formatTime } from '../../lib/time'
import type { CheckpointProgress, CheckpointStatus } from '../../types'

export function ProgressBar({ pct, label }: { pct: number; label: string }) {
  const v = Math.max(0, Math.min(100, Math.round(pct)))
  return (
    <div
      role="progressbar"
      aria-valuenow={v}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-label={label}
      className="h-2.5 w-full overflow-hidden rounded-full bg-gray-200"
    >
      <div className={`h-full rounded-full ${v >= 100 ? 'bg-green-600' : 'bg-primary-600'}`} style={{ width: `${v}%` }} />
    </div>
  )
}

const icons: Record<CheckpointStatus, { icon: typeof Clock; cls: string }> = {
  on_time: { icon: CheckCircle2, cls: 'text-green-700 bg-green-50 border-green-300' },
  late: { icon: Clock, cls: 'text-yellow-800 bg-yellow-50 border-yellow-300' },
  overdue: { icon: AlertTriangle, cls: 'text-orange-700 bg-orange-50 border-orange-300' },
  missed: { icon: XCircle, cls: 'text-red-700 bg-red-50 border-red-300' },
  pending: { icon: Circle, cls: 'text-gray-500 bg-white border-gray-300' },
}

/** Vertical timeline of a session's checkpoints, coloured by status. */
export function CheckpointTimeline({ checkpoints, tz, compact = false }: { checkpoints: CheckpointProgress[]; tz: string; compact?: boolean }) {
  const { t } = useI18n()
  return (
    <ol className={compact ? 'flex flex-wrap gap-1.5' : 'space-y-2'}>
      {checkpoints.map((cp) => {
        const { icon: Icon, cls } = icons[cp.status] ?? icons.pending
        if (compact) {
          return (
            <li key={cp.id} title={`${cp.name} — ${t(`status.${cp.status}`)}`} className={`inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs ${cls}`}>
              <Icon className="h-3 w-3" aria-hidden="true" />
              <span>{cp.name}</span>
              <span className="sr-only">: {t(`status.${cp.status}`)}</span>
            </li>
          )
        }
        return (
          <li key={cp.id} className={`flex flex-wrap items-center gap-3 rounded-lg border px-3 py-2 text-sm ${cls}`}>
            <Icon className="h-4 w-4 shrink-0" aria-hidden="true" />
            <span className="w-6 text-xs tabular-nums text-gray-600">{cp.order_index + 1}.</span>
            <span className="font-medium text-gray-900">{cp.name}</span>
            <StatusBadge status={cp.status} />
            <span className="ml-auto flex flex-wrap gap-3 text-xs text-gray-700">
              {cp.scanned_at && (
                <span>
                  {t('pat.scannedAt', { time: formatTime(cp.scanned_at, tz) })}
                  {cp.method && ` · ${t(cp.method === 'manual' ? 'pat.method.manual' : 'pat.method.qr')}`}
                </span>
              )}
              {cp.due_at && <span>{t('pat.due', { time: formatTime(cp.due_at, tz) })}</span>}
            </span>
          </li>
        )
      })}
    </ol>
  )
}
