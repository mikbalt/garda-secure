import { useQuery } from '@tanstack/react-query'
import { ArrowRight, History, Quote, UserRound } from 'lucide-react'
import { apiGet } from '../../api/client'
import { TableSkeleton } from '../../components/LoadingSkeleton'
import { Badge, StatusBadge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Modal } from '../../components/ui/Modal'
import { EmptyState, ErrorState } from '../../components/ui/States'
import { useI18n } from '../../i18n/context'
import { formatDateTime, formatTime, localDateOf } from '../../lib/time'
import { formatLongDay } from '../guards/peopleUtil'
import type { AttendanceHistory, AttendanceOverride, DailyAttendanceRow, Site } from '../../types'

/** Clock time in the site's zone, with the date when it differs from the shift day. */
function useClock(tz: string, shiftDay: string) {
  const { lang } = useI18n()
  return (iso?: string | null) => {
    if (!iso) return '—'
    const day = localDateOf(iso, tz)
    return day === shiftDay ? formatTime(iso, tz) : formatDateTime(iso, tz, lang)
  }
}

function Change({ label, before, after }: { label: string; before: string; after: string }) {
  if (before === after) return null
  return (
    <div className="flex flex-wrap items-center gap-2 text-sm">
      <span className="w-24 shrink-0 text-gray-600">{label}</span>
      <span className="tabular-nums text-gray-500 line-through decoration-gray-300">{before}</span>
      <ArrowRight className="h-3.5 w-3.5 text-gray-500" aria-hidden="true" />
      <span className="sr-only">→</span>
      <span className="font-medium tabular-nums text-gray-900">{after}</span>
    </div>
  )
}

function Entry({ o, tz, shiftDay, index }: { o: AttendanceOverride; tz: string; shiftDay: string; index: number }) {
  const { t, lang } = useI18n()
  const clock = useClock(tz, shiftDay)
  return (
    <li className="relative pl-8">
      <span className="absolute left-0 top-0.5 flex h-6 w-6 items-center justify-center rounded-full bg-primary-50 text-xs font-semibold text-primary-700 ring-1 ring-primary-100">
        {index + 1}
      </span>
      <div className="rounded-xl border border-gray-200 bg-white p-3.5 shadow-card">
        <p className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-gray-600">
          <time dateTime={o.created_at} className="font-medium text-gray-800">
            {formatDateTime(o.created_at, tz, lang)}
          </time>
          <span className="inline-flex items-center gap-1">
            <UserRound className="h-3 w-3" aria-hidden="true" />
            {o.actor || '—'}
          </span>
        </p>
        <div className="mt-2 flex flex-wrap items-center gap-2">
          {o.before_status ? <StatusBadge status={o.before_status} /> : <Badge tone="gray">{t('acc.hist.noRecord')}</Badge>}
          <ArrowRight className="h-4 w-4 text-gray-500" aria-hidden="true" />
          <span className="sr-only">{t('acc.hist.becomes')}</span>
          <StatusBadge status={o.after_status} />
        </div>
        <div className="mt-2 space-y-1">
          <Change label={t('att.clockIn')} before={clock(o.before_clock_in_at)} after={clock(o.after_clock_in_at)} />
          <Change label={t('att.clockOut')} before={clock(o.before_clock_out_at)} after={clock(o.after_clock_out_at)} />
        </div>
        <p className="mt-2.5 flex items-start gap-2 rounded-lg bg-gray-50 px-3 py-2 text-sm text-gray-800">
          <Quote className="mt-0.5 h-3.5 w-3.5 shrink-0 text-gray-500" aria-hidden="true" />
          <span>
            <span className="sr-only">{t('common.reason')}: </span>
            {o.reason}
          </span>
        </p>
        {o.before_reason && <p className="mt-1.5 text-xs text-gray-500">{t('acc.hist.previousReason', { reason: o.before_reason })}</p>}
      </div>
    </li>
  )
}

/** Override history of one shift's attendance (F6): every before → after with reason, actor and time. */
export function HistoryDrawer({ site, row, onClose }: { site: Site; row: DailyAttendanceRow; onClose: () => void }) {
  const { t, lang } = useI18n()
  const tz = site.timezone
  const shiftDay = localDateOf(row.shift_start, tz)
  const path = row.attendance_id ? `/attendance/${row.attendance_id}/history` : `/attendance/shifts/${row.shift_id}/history`
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['attendance-history', row.attendance_id ?? row.shift_id],
    queryFn: () => apiGet<AttendanceHistory>(path),
  })
  const overrides = data?.overrides ?? []
  return (
    <Modal
      open
      variant="drawer"
      onClose={onClose}
      title={t('acc.hist.title', { name: row.guard_name })}
      footer={
        <Button variant="secondary" onClick={onClose}>
          {t('common.close')}
        </Button>
      }
    >
      <p className="mb-4 text-sm text-gray-600">
        {formatLongDay(shiftDay, lang)} · {formatTime(row.shift_start, tz)}–{formatTime(row.shift_end, tz)} · {t('ppl.att.tzNote', { tz })}
      </p>
      {isLoading ? (
        <TableSkeleton />
      ) : error ? (
        <ErrorState error={error} onRetry={() => refetch()} />
      ) : overrides.length === 0 ? (
        <EmptyState icon={History} title={t('acc.hist.emptyTitle')} message={t('acc.hist.emptyBody')} />
      ) : (
        <>
          <p className="mb-3 text-xs text-gray-500">{t('acc.hist.order', { n: overrides.length })}</p>
          <ol className="space-y-3">
            {overrides.map((o, i) => (
              <Entry key={o.id} o={o} tz={tz} shiftDay={shiftDay} index={i} />
            ))}
          </ol>
        </>
      )}
    </Modal>
  )
}
