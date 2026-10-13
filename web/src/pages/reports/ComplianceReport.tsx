import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { CalendarClock, CalendarX2, CheckCircle2, Clock4, Hourglass } from 'lucide-react'
import { apiGet } from '../../api/client'
import { Table, type Column } from '../../components/ui/Table'
import { useAuth } from '../../hooks/useAuth'
import { useI18n } from '../../i18n/context'
import { addDays, daysBetween, formatDay, formatNumber, todayIn } from '../../lib/time'
import { PctCell, RangePicker, ReportEmpty, Stat, StatRow, type Range } from './ReportKit'
import { ReportShell } from './ReportShell'
import type { ComplianceReport as Report, ComplianceRow, Site } from '../../types'

const MAX_DAYS = 92

const muted = (n: number, cls = '') => (n > 0 ? <span className={cls}>{n}</span> : <span className="text-gray-500">0</span>)

/** Patrol schedule compliance (D-038): scheduled patrols started on time, late or missed, per day and route. */
export function ComplianceReport({ site, title }: { site: Site; title: string }) {
  const { t, lang } = useI18n()
  const { can } = useAuth()
  const tz = site.timezone
  const today = todayIn(tz)
  const [range, setRange] = useState<Range>({ from: addDays(today, -6), to: today })
  const { from, to } = range
  const valid = !!from && !!to && from <= to && daysBetween(from, to) <= MAX_DAYS

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['report-compliance', site.id, from, to],
    queryFn: () => apiGet<Report>('/reports/patrol-compliance', { site_id: site.id, from, to }),
    enabled: valid,
  })

  const rows = data?.rows ?? []
  const tot = data?.totals
  const decided = tot ? tot.started_on_time + tot.late + tot.missed : 0

  const columns: Column<ComplianceRow>[] = [
    { key: 'date', header: t('common.date'), cell: (r) => <span className="whitespace-nowrap tabular-nums">{formatDay(r.date, lang)}</span> },
    { key: 'route', header: t('pat.route'), cell: (r) => <span className="whitespace-nowrap font-medium text-gray-900">{r.route_name || '—'}</span> },
    { key: 'expected', header: t('p2.rep.comp.expected'), numeric: true, cell: (r) => r.expected },
    { key: 'ontime', header: t('p2.rep.comp.onTime'), numeric: true, cell: (r) => muted(r.started_on_time, 'font-medium text-emerald-700') },
    { key: 'late', header: t('p2.rep.comp.late'), numeric: true, cell: (r) => muted(r.late, 'font-medium text-amber-700') },
    { key: 'missed', header: t('p2.rep.comp.missed'), numeric: true, cell: (r) => muted(r.missed, 'font-medium text-red-700') },
    { key: 'pending', header: t('p2.rep.comp.pending'), numeric: true, cell: (r) => muted(r.pending) },
    {
      key: 'pct',
      header: t('p2.rep.comp.pct'),
      numeric: true,
      cell: (r) => (r.compliance_pct == null ? <span className="text-gray-500" title={t('p2.rep.comp.undecided')}>—</span> : <PctCell value={r.compliance_pct} />),
    },
  ]

  return (
    <ReportShell
      site={site}
      title={title}
      period={from && to ? `${formatDay(from, lang, false)} – ${formatDay(to, lang, false)}` : ''}
      csv={{ path: '/reports/patrol-compliance', params: { site_id: site.id, from, to }, filename: `patrol-compliance-${from}-${to}`, disabled: !valid }}
      filters={<RangePicker value={range} onChange={setRange} today={today} />}
      invalid={valid ? undefined : t('adm.rep.badRange', { n: MAX_DAYS })}
      stats={
        (tot && tot.expected > 0) || isLoading ? (
          <StatRow loading={isLoading}>
            <Stat
              icon={CheckCircle2}
              tone="green"
              label={t('p2.rep.comp.pct')}
              value={tot?.compliance_pct == null ? '—' : `${formatNumber(tot.compliance_pct, lang, 1)}%`}
              progress={tot?.compliance_pct ?? undefined}
              sub={t('p2.rep.comp.pctSub', { n: decided })}
            />
            <Stat icon={CalendarClock} tone="blue" label={t('p2.rep.comp.expected')} value={tot?.expected ?? 0} sub={t('p2.rep.comp.pendingSub', { n: tot?.pending ?? 0 })} />
            <Stat icon={Clock4} tone="amber" label={t('p2.rep.comp.late')} value={tot?.late ?? 0} sub={t('p2.rep.comp.lateSub')} />
            <Stat icon={CalendarX2} tone="red" label={t('p2.rep.comp.missed')} value={tot?.missed ?? 0} sub={t('p2.rep.comp.missedSub')} />
          </StatRow>
        ) : null
      }
    >
      {!isLoading && !error && rows.length === 0 ? (
        <ReportEmpty
          icon={Hourglass}
          title={t('p2.rep.comp.emptyTitle')}
          message={t('p2.rep.comp.empty')}
          cta={can('editRoutes') ? { to: '/patrol?tab=routes', label: t('adm.rep.openRoutes') } : undefined}
        />
      ) : (
        <>
          <Table columns={columns} rows={rows} rowKey={(r) => `${r.date}-${r.route_id}`} isLoading={isLoading} error={error} onRetry={() => refetch()} caption={title} />
          {rows.length > 0 && <p className="mt-3 text-xs text-gray-500">{t('p2.rep.comp.foot')}</p>}
        </>
      )}
    </ReportShell>
  )
}
