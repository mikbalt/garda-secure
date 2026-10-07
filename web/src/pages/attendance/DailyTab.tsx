import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Flag, Pencil, Info } from 'lucide-react'
import { apiGet } from '../../api/client'
import { DownloadCsvButton } from '../../components/DownloadButton'
import { StatusBadge, Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { MediaPhotoButton } from '../../components/MediaPhoto'
import { Field, Input } from '../../components/ui/Form'
import { Toolbar } from '../../components/ui/States'
import { Table, type Column } from '../../components/ui/Table'
import { useAuth } from '../../hooks/useAuth'
import { useI18n } from '../../i18n/context'
import { formatNumber, formatTime, todayIn } from '../../lib/time'
import { OverrideModal } from './OverrideModal'
import type { DailyAttendance, DailyAttendanceRow, Site } from '../../types'

export function DailyTab({ site }: { site: Site }) {
  const { t, lang } = useI18n()
  const { can } = useAuth()
  const tz = site.timezone
  const [date, setDate] = useState(() => todayIn(tz))
  const [editing, setEditing] = useState<DailyAttendanceRow | null>(null)

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['attendance-daily', site.id, date],
    queryFn: () => apiGet<DailyAttendance>('/attendance/daily', { site_id: site.id, date }),
    enabled: !!date,
  })

  const columns: Column<DailyAttendanceRow>[] = [
    {
      key: 'guard',
      header: t('common.guard'),
      cell: (r) => (
        <div>
          <div className="font-medium">{r.guard_name}</div>
          {r.employee_id && <div className="text-xs text-gray-600">{r.employee_id}</div>}
        </div>
      ),
    },
    {
      key: 'shift',
      header: t('att.shift'),
      cell: (r) => (
        <span className="whitespace-nowrap tabular-nums">
          {formatTime(r.shift_start, tz)}–{formatTime(r.shift_end, tz)}
        </span>
      ),
    },
    {
      key: 'status',
      header: t('common.status'),
      cell: (r) => (
        <div className="flex flex-wrap items-center gap-1">
          <StatusBadge status={r.status} />
          {r.override_reason && (
            <span title={t('att.overridden', { reason: r.override_reason })}>
              <Info className="h-4 w-4 text-primary-700" aria-hidden="true" />
              <span className="sr-only">{t('att.overridden', { reason: r.override_reason })}</span>
            </span>
          )}
        </div>
      ),
    },
    { key: 'in', header: t('att.clockIn'), cell: (r) => <span className="tabular-nums">{formatTime(r.clock_in_at, tz)}</span> },
    { key: 'out', header: t('att.clockOut'), cell: (r) => <span className="tabular-nums">{formatTime(r.clock_out_at, tz)}</span> },
    { key: 'late', header: t('att.lateMin'), numeric: true, cell: (r) => (r.late_minutes ? r.late_minutes : '—') },
    { key: 'early', header: t('att.earlyOutMin'), numeric: true, cell: (r) => (r.early_out_minutes ? r.early_out_minutes : '—') },
    { key: 'hours', header: t('common.hours'), numeric: true, cell: (r) => (r.hours_worked ? formatNumber(r.hours_worked, lang, 1) : '—') },
    {
      key: 'selfie',
      header: t('att.selfie'),
      cell: (r) => <MediaPhotoButton objectKey={r.selfie_object_key} title={`${t('att.selfie')}: ${r.guard_name}`} />,
    },
    {
      key: 'review',
      header: t('att.review'),
      cell: (r) =>
        r.needs_review ? (
          <Badge tone="purple">
            <Flag className="h-3 w-3" aria-hidden="true" />
            {t('att.needsReview')}
          </Badge>
        ) : null,
    },
  ]
  if (can('overrideAttendance')) {
    columns.push({
      key: 'actions',
      header: <span className="sr-only">{t('common.actions')}</span>,
      cell: (r) => (
        <Button size="sm" variant="secondary" onClick={() => setEditing(r)} aria-label={`${t('att.override')} — ${r.guard_name}`}>
          <Pencil className="h-3.5 w-3.5" aria-hidden="true" />
          {t('att.override')}
        </Button>
      ),
    })
  }

  return (
    <>
      <Toolbar>
        <Field label={t('common.date')}>
          <Input type="date" value={date} onChange={(e) => setDate(e.target.value)} />
        </Field>
        <div className="ml-auto">
          <DownloadCsvButton path="/reports/attendance/daily" params={{ site_id: site.id, date }} filename={`attendance-${date}.csv`} />
        </div>
      </Toolbar>
      <Table
        columns={columns}
        rows={data?.rows}
        rowKey={(r) => r.shift_id}
        isLoading={isLoading}
        error={error}
        onRetry={() => refetch()}
        caption={t('att.title')}
      />
      {editing && <OverrideModal site={site} row={editing} onClose={() => setEditing(null)} />}
    </>
  )
}
