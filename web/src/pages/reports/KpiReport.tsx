import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { apiGet } from '../../api/client'
import { Field, Input } from '../../components/ui/Form'
import { Table, type Column } from '../../components/ui/Table'
import { useI18n } from '../../i18n/context'
import { addDays, daysBetween, formatDay, formatNumber, todayIn } from '../../lib/time'
import { ReportShell } from './ReportShell'
import type { GuardKpiRow, RowsReport, Site } from '../../types'

export function KpiReport({ site }: { site: Site }) {
  const { t, lang } = useI18n()
  const tz = site.timezone
  const [from, setFrom] = useState(() => addDays(todayIn(tz), -29))
  const [to, setTo] = useState(() => todayIn(tz))
  const valid = !!from && !!to && from <= to && daysBetween(from, to) <= 366

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['report-kpi', site.id, from, to],
    queryFn: () => apiGet<RowsReport<GuardKpiRow>>('/reports/guards/kpi', { site_id: site.id, from, to }),
    enabled: valid,
  })

  const pct = (v: number) => `${formatNumber(v, lang, 1)}%`
  const columns: Column<GuardKpiRow>[] = [
    { key: 'guard', header: t('common.guard'), cell: (r) => <span className="font-medium">{r.guard_name}</span> },
    { key: 'att', header: t('rep.attendanceRate'), numeric: true, cell: (r) => pct(r.attendance_rate_pct) },
    { key: 'ontime', header: t('rep.onTimeRate'), numeric: true, cell: (r) => pct(r.on_time_rate_pct) },
    { key: 'patrol', header: t('rep.patrolPct'), numeric: true, cell: (r) => pct(r.patrol_completion_pct) },
    { key: 'inc', header: t('rep.incidentsFiled'), numeric: true, cell: (r) => r.incidents_filed },
    { key: 'sla', header: t('pat.slaBreaches'), numeric: true, cell: (r) => r.sla_breaches },
  ]

  return (
    <ReportShell
      site={site}
      title={t('rep.kpi')}
      period={from && to ? `${formatDay(from, lang, false)} – ${formatDay(to, lang, false)}` : ''}
      printable
      csv={{ path: '/reports/guards/kpi', params: { site_id: site.id, from, to }, filename: 'guard-kpi.csv', disabled: !valid }}
      filters={
        <>
          <Field label={t('common.from')}>
            <Input type="date" value={from} onChange={(e) => setFrom(e.target.value)} />
          </Field>
          <Field label={t('common.to')}>
            <Input type="date" value={to} onChange={(e) => setTo(e.target.value)} />
          </Field>
        </>
      }
    >
      {valid ? (
        <Table columns={columns} rows={data?.rows} rowKey={(r) => r.guard_id} isLoading={isLoading} error={error} onRetry={() => refetch()} caption={t('rep.kpi')} />
      ) : (
        <p role="alert" className="text-sm text-red-700">
          {t('rep.rangeTooLong', { n: 366 })}
        </p>
      )}
    </ReportShell>
  )
}
