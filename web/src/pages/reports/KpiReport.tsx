import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Clock4, Route as RouteIcon, UserCheck, Users } from 'lucide-react'
import { apiGet } from '../../api/client'
import { Table, type Column } from '../../components/ui/Table'
import { useAuth } from '../../hooks/useAuth'
import { useI18n } from '../../i18n/context'
import { addDays, daysBetween, formatDay, formatNumber, todayIn } from '../../lib/time'
import { average } from './format'
import { PctCell, RangePicker, ReportEmpty, Stat, StatRow, type Range } from './ReportKit'
import { ReportShell } from './ReportShell'
import type { GuardKpiRow, RowsReport, Site } from '../../types'

const MAX_DAYS = 366

/** Per-guard discipline KPIs (attendance, punctuality, patrol completion). */
export function KpiReport({ site, title }: { site: Site; title: string }) {
  const { t, lang } = useI18n()
  const { can } = useAuth()
  const tz = site.timezone
  const today = todayIn(tz)
  const [range, setRange] = useState<Range>({ from: addDays(today, -29), to: today })
  const { from, to } = range
  const valid = !!from && !!to && from <= to && daysBetween(from, to) <= MAX_DAYS

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['report-kpi', site.id, from, to],
    queryFn: () => apiGet<RowsReport<GuardKpiRow>>('/reports/guards/kpi', { site_id: site.id, from, to }),
    enabled: valid,
  })

  const rows = useMemo(() => data?.rows ?? [], [data])
  const avg = (f: (r: GuardKpiRow) => number) => average(rows.map(f))
  const att = avg((r) => r.attendance_rate_pct)
  const onTime = avg((r) => r.on_time_rate_pct)
  const patrol = avg((r) => r.patrol_completion_pct)
  const needsCoaching = rows.filter((r) => r.attendance_rate_pct < 75 || r.on_time_rate_pct < 75).length
  const fmt = (v: number | undefined) => (v === undefined ? '—' : `${formatNumber(v, lang, 1)}%`)

  const columns: Column<GuardKpiRow>[] = [
    { key: 'guard', header: t('common.guard'), cell: (r) => <span className="whitespace-nowrap font-medium text-gray-900">{r.guard_name}</span> },
    { key: 'att', header: t('rep.attendanceRate'), numeric: true, cell: (r) => <PctCell value={r.attendance_rate_pct} /> },
    { key: 'ontime', header: t('rep.onTimeRate'), numeric: true, cell: (r) => <PctCell value={r.on_time_rate_pct} /> },
    { key: 'patrol', header: t('rep.patrolPct'), numeric: true, cell: (r) => <PctCell value={r.patrol_completion_pct} /> },
    { key: 'inc', header: t('rep.incidentsFiled'), numeric: true, cell: (r) => r.incidents_filed },
    {
      key: 'sla',
      header: t('pat.slaBreaches'),
      numeric: true,
      cell: (r) => (r.sla_breaches > 0 ? <span className="font-medium text-orange-700">{r.sla_breaches}</span> : <span className="text-gray-500">0</span>),
    },
  ]

  return (
    <ReportShell
      site={site}
      title={title}
      period={from && to ? `${formatDay(from, lang, false)} – ${formatDay(to, lang, false)}` : ''}
      csv={{ path: '/reports/guards/kpi', params: { site_id: site.id, from, to }, filename: `guard-kpi-${from}-${to}.csv`, disabled: !valid }}
      filters={<RangePicker value={range} onChange={setRange} today={today} />}
      invalid={valid ? undefined : t('adm.rep.badRange', { n: MAX_DAYS })}
      stats={
        rows.length > 0 || isLoading ? (
          <StatRow loading={isLoading}>
            <Stat
              icon={Users}
              tone="blue"
              label={t('adm.rep.stat.guardsLabel')}
              value={rows.length}
              sub={needsCoaching > 0 ? t('adm.rep.stat.coaching', { n: needsCoaching }) : t('adm.rep.stat.allGood')}
            />
            <Stat icon={UserCheck} tone="green" label={t('adm.rep.stat.avgAttendance')} value={fmt(att)} progress={att} />
            <Stat icon={Clock4} tone="amber" label={t('adm.rep.stat.avgOnTime')} value={fmt(onTime)} progress={onTime} />
            <Stat icon={RouteIcon} tone="violet" label={t('adm.rep.stat.avgPatrol')} value={fmt(patrol)} progress={patrol} />
          </StatRow>
        ) : null
      }
    >
      {!isLoading && !error && rows.length === 0 ? (
        <ReportEmpty
          icon={Users}
          title={t('adm.rep.kpi.emptyTitle')}
          message={t('adm.rep.kpi.empty')}
          cta={can('viewGuards') ? { to: '/guards', label: t('adm.rep.openGuards') } : undefined}
        />
      ) : (
        <>
          <Table columns={columns} rows={rows} rowKey={(r) => r.guard_id} isLoading={isLoading} error={error} onRetry={() => refetch()} caption={title} />
          {rows.length > 0 && (
            <p className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-gray-500">
              <span>{t('adm.rep.kpi.legend')}</span>
              <span className="inline-flex items-center gap-1.5"><span className="h-2 w-2 rounded-full bg-emerald-500" />≥ 90%</span>
              <span className="inline-flex items-center gap-1.5"><span className="h-2 w-2 rounded-full bg-amber-400" />75–90%</span>
              <span className="inline-flex items-center gap-1.5"><span className="h-2 w-2 rounded-full bg-red-500" />&lt; 75%</span>
            </p>
          )}
        </>
      )}
    </ReportShell>
  )
}
