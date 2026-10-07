import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { apiGet } from '../../api/client'
import { StatusBadge } from '../../components/ui/Badge'
import { Field, Input } from '../../components/ui/Form'
import { Table, type Column } from '../../components/ui/Table'
import { useI18n } from '../../i18n/context'
import { formatDay, formatNumber, formatTime, todayIn } from '../../lib/time'
import { ReportShell } from './ReportShell'
import type { DailyAttendance, DailyAttendanceRow, Site } from '../../types'

export function DailyReport({ site }: { site: Site }) {
  const { t, lang } = useI18n()
  const tz = site.timezone
  const [date, setDate] = useState(() => todayIn(tz))
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['report-daily', site.id, date],
    queryFn: () => apiGet<DailyAttendance>('/reports/attendance/daily', { site_id: site.id, date }),
    enabled: !!date,
  })

  const columns: Column<DailyAttendanceRow>[] = [
    { key: 'guard', header: t('common.guard'), cell: (r) => <span className="font-medium">{r.guard_name}</span> },
    { key: 'emp', header: t('att.employeeId'), cell: (r) => r.employee_id ?? '—' },
    { key: 'shift', header: t('att.shift'), cell: (r) => `${formatTime(r.shift_start, tz)}–${formatTime(r.shift_end, tz)}` },
    { key: 'status', header: t('common.status'), cell: (r) => <StatusBadge status={r.status} /> },
    { key: 'in', header: t('att.clockIn'), cell: (r) => formatTime(r.clock_in_at, tz) },
    { key: 'out', header: t('att.clockOut'), cell: (r) => formatTime(r.clock_out_at, tz) },
    { key: 'late', header: t('att.lateMin'), numeric: true, cell: (r) => r.late_minutes },
    { key: 'early', header: t('att.earlyOutMin'), numeric: true, cell: (r) => r.early_out_minutes },
    { key: 'hours', header: t('common.hours'), numeric: true, cell: (r) => formatNumber(r.hours_worked, lang, 1) },
    { key: 'reason', header: t('common.reason'), cell: (r) => r.override_reason ?? '' },
  ]

  return (
    <ReportShell
      site={site}
      title={t('rep.daily')}
      period={date ? formatDay(date, lang) : ''}
      printable
      csv={{ path: '/reports/attendance/daily', params: { site_id: site.id, date }, filename: `attendance-${date}.csv`, disabled: !date }}
      filters={
        <Field label={t('common.date')}>
          <Input type="date" value={date} onChange={(e) => setDate(e.target.value)} />
        </Field>
      }
    >
      <Table columns={columns} rows={data?.rows} rowKey={(r) => r.shift_id} isLoading={isLoading} error={error} onRetry={() => refetch()} caption={t('rep.daily')} />
    </ReportShell>
  )
}
