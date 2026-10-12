import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { CheckCircle2, MapPinOff, Route as RouteIcon, TimerOff } from 'lucide-react'
import { apiGet } from '../../api/client'
import { StatusBadge } from '../../components/ui/Badge'
import { Table, type Column } from '../../components/ui/Table'
import { useAuth } from '../../hooks/useAuth'
import { useI18n } from '../../i18n/context'
import { addDays, daysBetween, formatDateTime, formatDay, formatNumber, todayIn } from '../../lib/time'
import { average } from './format'
import { PctCell, RangePicker, ReportEmpty, Stat, StatRow, type Range } from './ReportKit'
import { ReportShell } from './ReportShell'
import type { RowsReport, SessionSummary, Site } from '../../types'

const MAX_DAYS = 92

/** Patrol compliance: every round, scanned vs. planned checkpoints, SLA breaches. */
export function PatrolReport({ site, title }: { site: Site; title: string }) {
  const { t, lang } = useI18n()
  const { can } = useAuth()
  const tz = site.timezone
  const today = todayIn(tz)
  const [range, setRange] = useState<Range>({ from: addDays(today, -6), to: today })
  const { from, to } = range
  const valid = !!from && !!to && from <= to && daysBetween(from, to) <= MAX_DAYS

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['report-patrol', site.id, from, to],
    queryFn: () => apiGet<RowsReport<SessionSummary>>('/reports/patrol/sessions', { site_id: site.id, from, to }),
    enabled: valid,
  })

  const rows = data?.rows ?? []
  // Rounds still running are not counted as missed / incomplete yet.
  const finished = rows.filter((r) => r.status !== 'in_progress')
  const running = rows.length - finished.length
  const avg = average(finished.map((r) => r.completion_pct))
  const breaches = rows.reduce((n, r) => n + r.sla_breaches, 0)
  const missed = finished.reduce((n, r) => n + Math.max(0, r.total_checkpoints - r.scanned_count), 0)
  const planned = finished.reduce((n, r) => n + r.total_checkpoints, 0)

  const columns: Column<SessionSummary>[] = [
    {
      key: 'route',
      header: t('pat.route'),
      cell: (r) => (
        <div className="min-w-[9rem]">
          <p className="font-medium text-gray-900">{r.route_name}</p>
          <p className="text-xs text-gray-500">{r.guard_name}</p>
        </div>
      ),
    },
    { key: 'started', header: t('pat.started'), cell: (r) => <span className="whitespace-nowrap tabular-nums">{formatDateTime(r.started_at, tz, lang)}</span> },
    { key: 'ended', header: t('pat.ended'), cell: (r) => <span className="whitespace-nowrap tabular-nums">{formatDateTime(r.ended_at, tz, lang)}</span> },
    { key: 'status', header: t('common.status'), cell: (r) => <StatusBadge status={r.status} /> },
    {
      key: 'scanned',
      header: t('adm.rep.scannedOf'),
      numeric: true,
      cell: (r) => (
        <span className="whitespace-nowrap">
          {r.scanned_count}
          <span className="text-gray-500"> / {r.total_checkpoints}</span>
        </span>
      ),
    },
    {
      key: 'missed',
      header: t('rep.missed'),
      numeric: true,
      cell: (r) => {
        if (r.status === 'in_progress') return <span className="text-gray-500">—</span>
        const n = Math.max(0, r.total_checkpoints - r.scanned_count)
        return n > 0 ? <span className="font-medium text-red-700">{n}</span> : <span className="text-gray-500">0</span>
      },
    },
    {
      key: 'sla',
      header: t('pat.slaBreaches'),
      numeric: true,
      cell: (r) => (r.sla_breaches > 0 ? <span className="font-medium text-orange-700">{r.sla_breaches}</span> : <span className="text-gray-500">0</span>),
    },
    { key: 'pct', header: t('pat.completion'), numeric: true, cell: (r) => <PctCell value={r.completion_pct} /> },
  ]

  return (
    <ReportShell
      site={site}
      title={title}
      period={from && to ? `${formatDay(from, lang, false)} – ${formatDay(to, lang, false)}` : ''}
      csv={{ path: '/reports/patrol/sessions', params: { site_id: site.id, from, to }, filename: `patrol-sessions-${from}-${to}.csv`, disabled: !valid }}
      filters={<RangePicker value={range} onChange={setRange} today={today} />}
      invalid={valid ? undefined : t('adm.rep.badRange', { n: MAX_DAYS })}
      stats={
        rows.length > 0 || isLoading ? (
          <StatRow loading={isLoading}>
            <Stat icon={RouteIcon} tone="violet" label={t('adm.rep.stat.sessions')} value={rows.length} sub={running > 0 ? t('adm.rep.stat.sessionsSub', { n: finished.length, m: running }) : t('adm.rep.stat.completedSub', { n: finished.length })} />
            <Stat
              icon={CheckCircle2}
              tone="green"
              label={t('adm.rep.stat.avgCompletionLabel')}
              value={avg === undefined ? '—' : `${formatNumber(avg, lang, 1)}%`}
              progress={avg}
              sub={running > 0 ? t('adm.rep.stat.finishedOnly') : t('adm.rep.stat.target')}
            />
            <Stat icon={TimerOff} tone="orange" label={t('pat.slaBreaches')} value={breaches} sub={t('adm.rep.stat.slaSub')} />
            <Stat icon={MapPinOff} tone="red" label={t('adm.rep.stat.missed')} value={missed} sub={t('adm.rep.stat.missedSub', { n: planned })} />
          </StatRow>
        ) : null
      }
    >
      {!isLoading && !error && rows.length === 0 ? (
        <ReportEmpty
          icon={RouteIcon}
          title={t('adm.rep.patrol.emptyTitle')}
          message={t('adm.rep.patrol.empty')}
          cta={can('editRoutes') ? { to: '/patrol?tab=routes', label: t('adm.rep.openRoutes') } : undefined}
        />
      ) : (
        <Table columns={columns} rows={rows} rowKey={(r) => r.session_id} isLoading={isLoading} error={error} onRetry={() => refetch()} caption={title} />
      )}
    </ReportShell>
  )
}
