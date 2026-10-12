import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { CalendarClock, CalendarX2, Route as RouteIcon, Timer, UserCheck } from 'lucide-react'
import { apiGet } from '../../api/client'
import { Table, type Column } from '../../components/ui/Table'
import { useAuth } from '../../hooks/useAuth'
import { useI18n } from '../../i18n/context'
import { formatNumber, monthIn } from '../../lib/time'
import { average, formatMonth, ratio } from './format'
import { MonthPicker, PctCell, ReportEmpty, Stat, StatRow } from './ReportKit'
import { ReportShell } from './ReportShell'
import type { MonthlyRecapRow, RowsReport, Site } from '../../types'

/** Monthly recap per guard — the payroll export (CSV) for HR. */
export function MonthlyReport({ site, title }: { site: Site; title: string }) {
  const { t, lang } = useI18n()
  const { can } = useAuth()
  const current = monthIn(site.timezone)
  const [month, setMonth] = useState(current)
  const valid = /^\d{4}-\d{2}$/.test(month)
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['report-monthly', site.id, month],
    queryFn: () => apiGet<RowsReport<MonthlyRecapRow>>('/reports/attendance/monthly', { site_id: site.id, month }),
    enabled: valid,
  })

  const rows = data?.rows ?? []
  const sum = (f: (r: MonthlyRecapRow) => number) => rows.reduce((n, r) => n + f(r), 0)
  const shifts = sum((r) => r.scheduled_shifts)
  const present = sum((r) => r.present)
  const late = sum((r) => r.late)
  const absent = sum((r) => r.absent)
  const hours = sum((r) => r.total_hours)
  const sessions = sum((r) => r.patrol_sessions)
  const attendance = ratio(present + late, present + late + absent)
  const patrolAvg = average(rows.filter((r) => r.patrol_sessions > 0).map((r) => r.patrol_completion_pct))

  const muted = (n: number, warn = false) => (n > 0 ? <span className={warn ? 'font-medium text-red-700' : ''}>{n}</span> : <span className="text-gray-500">0</span>)

  const columns: Column<MonthlyRecapRow>[] = [
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
    { key: 'shifts', header: t('rep.scheduled'), numeric: true, cell: (r) => r.scheduled_shifts },
    { key: 'present', header: t('adm.rep.onTime'), numeric: true, cell: (r) => r.present },
    { key: 'late', header: t('status.late'), numeric: true, cell: (r) => (r.late > 0 ? <span className="font-medium text-amber-700">{r.late}</span> : muted(0)) },
    { key: 'absent', header: t('status.absent'), numeric: true, cell: (r) => muted(r.absent, true) },
    { key: 'early', header: t('rep.earlyOut'), numeric: true, cell: (r) => muted(r.early_out) },
    { key: 'pending', header: t('adm.rep.upcoming'), numeric: true, cell: (r) => muted(r.pending) },
    { key: 'hours', header: t('rep.totalHours'), numeric: true, cell: (r) => <span className="font-medium text-gray-900">{formatNumber(r.total_hours, lang, 1)}</span> },
    { key: 'patrols', header: t('rep.patrols'), numeric: true, cell: (r) => r.patrol_sessions },
    { key: 'patrolPct', header: t('rep.patrolPct'), numeric: true, cell: (r) => (r.patrol_sessions > 0 ? <PctCell value={r.patrol_completion_pct} /> : <span className="text-gray-500">—</span>) },
  ]

  return (
    <ReportShell
      site={site}
      title={title}
      period={valid ? formatMonth(month, lang) : month}
      csv={{ path: '/reports/attendance/monthly', params: { site_id: site.id, month }, filename: `monthly-recap-${month}.csv`, disabled: !valid }}
      filters={<MonthPicker value={month} onChange={setMonth} current={current} />}
      stats={
        rows.length > 0 || isLoading ? (
          <StatRow loading={isLoading}>
            <Stat icon={CalendarClock} tone="blue" label={t('adm.rep.stat.shifts')} value={shifts} sub={t('adm.rep.stat.guards', { n: rows.length })} />
            <Stat
              icon={UserCheck}
              tone="green"
              label={t('adm.rep.stat.attendanceRate')}
              value={attendance === undefined ? '—' : `${formatNumber(attendance, lang, 1)}%`}
              progress={attendance}
              sub={t('adm.rep.stat.lateAbsent', { late, absent })}
            />
            <Stat
              icon={Timer}
              tone="violet"
              label={t('adm.rep.stat.totalHours')}
              value={formatNumber(hours, lang, 1)}
              sub={rows.length ? t('adm.rep.stat.hoursPerGuard', { n: formatNumber(hours / rows.length, lang, 1) }) : undefined}
            />
            <Stat
              icon={RouteIcon}
              tone="amber"
              label={t('adm.rep.stat.patrols')}
              value={sessions}
              sub={patrolAvg === undefined ? t('adm.rep.stat.noPatrols') : t('adm.rep.stat.avgCompletion', { pct: formatNumber(patrolAvg, lang, 1) })}
            />
          </StatRow>
        ) : null
      }
    >
      {!isLoading && !error && rows.length === 0 ? (
        <ReportEmpty
          icon={CalendarX2}
          title={t('adm.rep.monthly.emptyTitle')}
          message={t('adm.rep.monthly.empty')}
          cta={can('viewSchedules') ? { to: '/schedules', label: t('adm.rep.openSchedules') } : undefined}
        />
      ) : (
        <>
          <Table columns={columns} rows={rows} rowKey={(r) => r.guard_id} isLoading={isLoading} error={error} onRetry={() => refetch()} caption={title} />
          {rows.length > 0 && <p className="no-print mt-3 text-xs text-gray-500">{t('adm.rep.monthly.foot')}</p>}
        </>
      )}
    </ReportShell>
  )
}
