import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { apiGet } from '../../api/client'
import { StatusBadge } from '../../components/ui/Badge'
import { Field, Input } from '../../components/ui/Form'
import { Table, type Column } from '../../components/ui/Table'
import { useI18n } from '../../i18n/context'
import { addDays, daysBetween, formatDateTime, formatDay, formatNumber, todayIn } from '../../lib/time'
import { ReportShell } from './ReportShell'
import type { RowsReport, SessionSummary, Site } from '../../types'

export function PatrolReport({ site }: { site: Site }) {
  const { t, lang } = useI18n()
  const tz = site.timezone
  const [from, setFrom] = useState(() => addDays(todayIn(tz), -6))
  const [to, setTo] = useState(() => todayIn(tz))
  const valid = !!from && !!to && from <= to && daysBetween(from, to) <= 92

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['report-patrol', site.id, from, to],
    queryFn: () => apiGet<RowsReport<SessionSummary>>('/reports/patrol/sessions', { site_id: site.id, from, to }),
    enabled: valid,
  })

  const columns: Column<SessionSummary>[] = [
    { key: 'route', header: t('pat.route'), cell: (r) => <span className="font-medium">{r.route_name}</span> },
    { key: 'guard', header: t('common.guard'), cell: (r) => r.guard_name },
    { key: 'started', header: t('pat.started'), cell: (r) => <span className="whitespace-nowrap">{formatDateTime(r.started_at, tz, lang)}</span> },
    { key: 'ended', header: t('pat.ended'), cell: (r) => <span className="whitespace-nowrap">{formatDateTime(r.ended_at, tz, lang)}</span> },
    { key: 'status', header: t('common.status'), cell: (r) => <StatusBadge status={r.status} /> },
    { key: 'total', header: t('pat.checkpoints'), numeric: true, cell: (r) => r.total_checkpoints },
    { key: 'scanned', header: t('pat.scanned'), numeric: true, cell: (r) => r.scanned_count },
    { key: 'missed', header: t('rep.missed'), numeric: true, cell: (r) => r.total_checkpoints - r.scanned_count },
    { key: 'sla', header: t('pat.slaBreaches'), numeric: true, cell: (r) => r.sla_breaches },
    { key: 'pct', header: t('pat.completion'), numeric: true, cell: (r) => `${formatNumber(r.completion_pct, lang, 1)}%` },
  ]

  return (
    <ReportShell
      site={site}
      title={t('rep.patrol')}
      period={from && to ? `${formatDay(from, lang, false)} – ${formatDay(to, lang, false)}` : ''}
      printable
      csv={{ path: '/reports/patrol/sessions', params: { site_id: site.id, from, to }, filename: 'patrol-sessions.csv', disabled: !valid }}
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
        <Table columns={columns} rows={data?.rows} rowKey={(r) => r.session_id} isLoading={isLoading} error={error} onRetry={() => refetch()} caption={t('rep.patrol')} />
      ) : (
        <p role="alert" className="text-sm text-red-700">
          {t('rep.rangeTooLong', { n: 92 })}
        </p>
      )}
    </ReportShell>
  )
}
