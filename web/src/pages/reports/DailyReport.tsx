import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { CalendarX2, Clock4, UserCheck, UserX, Timer } from 'lucide-react'
import { apiGet } from '../../api/client'
import { Badge, StatusBadge } from '../../components/ui/Badge'
import { Table, type Column } from '../../components/ui/Table'
import { useAuth } from '../../hooks/useAuth'
import { useI18n } from '../../i18n/context'
import { formatDay, formatNumber, formatTime, todayIn } from '../../lib/time'
import { ratio } from './format'
import { DayPicker, ReportEmpty, Stat, StatRow } from './ReportKit'
import { ReportShell } from './ReportShell'
import type { DailyAttendance, DailyAttendanceRow, Site } from '../../types'

export function DailyReport({ site, title }: { site: Site; title: string }) {
  const { t, lang } = useI18n()
  const { can } = useAuth()
  const tz = site.timezone
  const today = todayIn(tz)
  const [date, setDate] = useState(today)
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['report-daily', site.id, date],
    queryFn: () => apiGet<DailyAttendance>('/reports/attendance/daily', { site_id: site.id, date }),
    enabled: !!date,
  })

  const rows = data?.rows ?? []
  const count = (s: DailyAttendanceRow['status']) => rows.filter((r) => r.status === s).length
  const present = count('present')
  const late = count('late')
  const absent = count('absent')
  const pending = count('pending')
  const hours = rows.reduce((n, r) => n + (r.hours_worked || 0), 0)
  // Only shifts that have started can be judged present / late / absent.
  const attendedPct = ratio(present + late, present + late + absent)

  const columns: Column<DailyAttendanceRow>[] = [
    {
      key: 'guard',
      header: t('common.guard'),
      cell: (r) => (
        <div className="min-w-[9rem]">
          <p className="font-medium text-gray-900">{r.guard_name}</p>
          {r.employee_id && <p className="text-xs text-gray-500">{r.employee_id}</p>}
        </div>
      ),
    },
    { key: 'shift', header: t('att.shift'), cell: (r) => <span className="whitespace-nowrap tabular-nums">{`${formatTime(r.shift_start, tz)}–${formatTime(r.shift_end, tz)}`}</span> },
    {
      key: 'status',
      header: t('common.status'),
      cell: (r) => (r.status === 'pending' ? <Badge tone="gray" dot>{t('adm.rep.notIn')}</Badge> : <StatusBadge status={r.status} />),
    },
    { key: 'in', header: t('att.clockIn'), cell: (r) => <span className="tabular-nums">{formatTime(r.clock_in_at, tz)}</span> },
    { key: 'out', header: t('att.clockOut'), cell: (r) => <span className="tabular-nums">{formatTime(r.clock_out_at, tz)}</span> },
    {
      key: 'late',
      header: t('att.lateMin'),
      numeric: true,
      cell: (r) => (r.late_minutes > 0 ? <span className="font-medium text-amber-700">{r.late_minutes}</span> : <span className="text-gray-500">0</span>),
    },
    {
      key: 'early',
      header: t('att.earlyOutMin'),
      numeric: true,
      cell: (r) => (r.early_out_minutes > 0 ? <span className="font-medium text-amber-700">{r.early_out_minutes}</span> : <span className="text-gray-500">0</span>),
    },
    { key: 'hours', header: t('common.hours'), numeric: true, cell: (r) => formatNumber(r.hours_worked, lang, 1) },
    { key: 'reason', header: t('adm.rep.note'), cell: (r) => (r.override_reason ? <span className="text-xs text-gray-600">{r.override_reason}</span> : '') },
  ]

  return (
    <ReportShell
      site={site}
      title={title}
      period={date ? formatDay(date, lang) : ''}
      csv={{ path: '/reports/attendance/daily', params: { site_id: site.id, date }, filename: `attendance-${date}.csv`, disabled: !date }}
      filters={<DayPicker value={date} onChange={setDate} today={today} />}
      stats={
        rows.length > 0 || isLoading ? (
          <StatRow loading={isLoading}>
            <Stat
              icon={UserCheck}
              tone="green"
              label={t('adm.rep.stat.attended')}
              value={attendedPct === undefined ? '—' : `${formatNumber(attendedPct, lang, 0)}%`}
              progress={attendedPct}
              sub={t('adm.rep.stat.attendedSub', { a: present + late, n: present + late + absent })}
            />
            <Stat icon={Clock4} tone="amber" label={t('status.late')} value={late} sub={t('adm.rep.stat.lateSub')} />
            <Stat icon={UserX} tone="red" label={t('status.absent')} value={absent} sub={pending > 0 ? t('adm.rep.stat.pendingSub', { n: pending }) : t('adm.rep.stat.absentSub')} />
            <Stat icon={Timer} tone="blue" label={t('adm.rep.stat.hours')} value={formatNumber(hours, lang, 1)} sub={t('adm.rep.stat.hoursSub')} />
          </StatRow>
        ) : null
      }
    >
      {!isLoading && !error && rows.length === 0 ? (
        <ReportEmpty
          icon={CalendarX2}
          title={t('adm.rep.daily.emptyTitle')}
          message={t('adm.rep.daily.empty')}
          cta={can('viewSchedules') ? { to: '/schedules', label: t('adm.rep.openSchedules') } : undefined}
        />
      ) : (
        <Table columns={columns} rows={rows} rowKey={(r) => r.shift_id} isLoading={isLoading} error={error} onRetry={() => refetch()} caption={title} />
      )}
    </ReportShell>
  )
}
