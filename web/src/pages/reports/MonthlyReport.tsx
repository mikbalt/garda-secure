import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { CalendarCheck2, CalendarClock, CalendarX2, Route as RouteIcon, Timer, UserCheck } from 'lucide-react'
import { Link } from 'react-router-dom'
import { apiGet } from '../../api/client'
import { Table, type Column } from '../../components/ui/Table'
import { useAuth } from '../../hooks/useAuth'
import { useI18n } from '../../i18n/context'
import { recapTotals } from '../../lib/recap'
import { formatNumber, monthIn } from '../../lib/time'
import { formatMonth, ratio } from './format'
import { MonthPicker, PctCell, ReportEmpty, Stat, StatRow } from './ReportKit'
import { ReportShell } from './ReportShell'
import type { MonthlyRecapRow, RowsReport, Site } from '../../types'

/**
 * Monthly recap per guard — the payroll export (CSV) for HR. Attendance is
 * counted per guard-day in the site's time zone (FR-REP-05); shifts are shown
 * for transparency.
 */
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
  const tot = data?.totals ?? recapTotals(rows)
  const attendance = ratio(tot.days_present + tot.days_late, tot.days_present + tot.days_late + tot.days_absent)
  const patrolAvg = tot.patrol_sessions > 0 ? tot.patrol_completion_pct : undefined
  const comp = data?.patrol_compliance

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
    { key: 'days', header: t('adm.rep.mon.days'), numeric: true, cell: (r) => r.scheduled_days },
    { key: 'present', header: t('status.present'), numeric: true, cell: (r) => r.days_present },
    { key: 'late', header: t('status.late'), numeric: true, cell: (r) => (r.days_late > 0 ? <span className="font-medium text-amber-700">{r.days_late}</span> : muted(0)) },
    { key: 'absent', header: t('status.absent'), numeric: true, cell: (r) => muted(r.days_absent, true) },
    { key: 'early', header: t('adm.rep.mon.earlyOutDays'), numeric: true, cell: (r) => muted(r.early_out_days) },
    { key: 'upcoming', header: t('adm.rep.mon.upcoming'), numeric: true, cell: (r) => muted(r.days_upcoming) },
    { key: 'hours', header: t('rep.totalHours'), numeric: true, cell: (r) => <span className="font-medium text-gray-900">{formatNumber(r.total_hours, lang, 1)}</span> },
    { key: 'shifts', header: t('rep.scheduled'), numeric: true, cell: (r) => <span className="text-gray-600">{r.scheduled_shifts}</span> },
    { key: 'patrols', header: t('rep.patrols'), numeric: true, cell: (r) => r.patrol_sessions },
    { key: 'patrolPct', header: t('rep.patrolPct'), numeric: true, cell: (r) => (r.patrol_sessions > 0 ? <PctCell value={r.patrol_completion_pct} /> : <span className="text-gray-500">—</span>) },
  ]

  const footer = {
    guard: t('adm.rep.mon.total', { n: tot.guards }),
    days: tot.scheduled_days,
    present: tot.days_present,
    late: tot.days_late,
    absent: tot.days_absent,
    early: tot.early_out_days,
    upcoming: tot.days_upcoming,
    hours: formatNumber(tot.total_hours, lang, 1),
    shifts: tot.scheduled_shifts,
    patrols: tot.patrol_sessions,
    patrolPct: patrolAvg === undefined ? '—' : `${formatNumber(patrolAvg, lang, 1)}%`,
  }

  return (
    <ReportShell
      site={site}
      title={title}
      period={valid ? formatMonth(month, lang) : month}
      csv={{ path: '/reports/attendance/monthly', params: { site_id: site.id, month }, filename: `monthly-recap-${month}`, disabled: !valid }}
      filters={<MonthPicker value={month} onChange={setMonth} current={current} />}
      stats={
        rows.length > 0 || isLoading ? (
          <StatRow loading={isLoading}>
            <Stat
              icon={CalendarClock}
              tone="blue"
              label={t('adm.rep.mon.stat.days')}
              value={tot.scheduled_days}
              sub={t('adm.rep.mon.stat.daysSub', { shifts: tot.scheduled_shifts, n: tot.guards })}
            />
            <Stat
              icon={UserCheck}
              tone="green"
              label={t('adm.rep.stat.attendanceRate')}
              value={attendance === undefined ? '—' : `${formatNumber(attendance, lang, 1)}%`}
              progress={attendance}
              sub={t('adm.rep.mon.stat.lateAbsent', { late: tot.days_late, absent: tot.days_absent })}
            />
            <Stat
              icon={Timer}
              tone="violet"
              label={t('adm.rep.stat.totalHours')}
              value={formatNumber(tot.total_hours, lang, 1)}
              sub={rows.length ? t('adm.rep.stat.hoursPerGuard', { n: formatNumber(tot.total_hours / rows.length, lang, 1) }) : undefined}
            />
            <Stat
              icon={RouteIcon}
              tone="amber"
              label={t('adm.rep.stat.patrols')}
              value={tot.patrol_sessions}
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
          {comp && comp.expected > 0 && (
            <div className="mb-4 flex flex-wrap items-center gap-3 rounded-2xl border border-gray-200/80 bg-white px-4 py-3 text-sm shadow-card print:shadow-none">
              <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-teal-50 text-teal-600 ring-1 ring-teal-100 print:hidden">
                <CalendarCheck2 className="h-4 w-4" aria-hidden="true" />
              </span>
              <p className="min-w-0 flex-1 text-gray-700">
                <span className="font-semibold text-gray-900">
                  {t('p2.rep.mon.comp', { pct: comp.compliance_pct == null ? '—' : `${formatNumber(comp.compliance_pct, lang, 1)}%` })}
                </span>{' '}
                {t('p2.rep.mon.compSub', { ok: comp.started_on_time, late: comp.late, missed: comp.missed, n: comp.expected })}
              </p>
              <Link to="/reports?tab=compliance" className="no-print text-sm font-medium text-primary-700 hover:text-primary-800 hover:underline">
                {t('p2.rep.mon.compLink')}
              </Link>
            </div>
          )}
          {rows.length > 0 && <p className="mb-3 text-sm text-gray-600">{t('adm.rep.mon.rule')}</p>}
          <Table
            columns={columns}
            rows={rows}
            rowKey={(r) => r.guard_id}
            isLoading={isLoading}
            error={error}
            onRetry={() => refetch()}
            caption={title}
            footer={rows.length > 0 ? footer : undefined}
          />
          {rows.length > 0 && <p className="no-print mt-3 text-xs text-gray-500">{t('adm.rep.monthly.foot')}</p>}
        </>
      )}
    </ReportShell>
  )
}
