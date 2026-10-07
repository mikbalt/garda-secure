import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { apiGet } from '../../api/client'
import { StatusBadge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Field, Input } from '../../components/ui/Form'
import { Modal } from '../../components/ui/Modal'
import { ErrorState, Toolbar } from '../../components/ui/States'
import { Table, type Column } from '../../components/ui/Table'
import { TableSkeleton } from '../../components/LoadingSkeleton'
import { useI18n } from '../../i18n/context'
import { addDays, daysBetween, formatDateTime, formatNumber, todayIn } from '../../lib/time'
import { CheckpointTimeline, ProgressBar } from './shared'
import type { SessionProgress, SessionSummary, Site } from '../../types'

function SessionDetail({ site, summary, onClose }: { site: Site; summary: SessionSummary; onClose: () => void }) {
  const { t, lang } = useI18n()
  const tz = site.timezone
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['patrol-session', summary.session_id],
    queryFn: () => apiGet<SessionProgress>(`/patrol/sessions/${summary.session_id}`),
  })
  return (
    <Modal open onClose={onClose} variant="drawer" title={`${t('pat.timeline')} — ${summary.route_name}`}>
      <dl className="mb-4 grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
        <dt className="text-gray-600">{t('common.guard')}</dt>
        <dd className="font-medium text-gray-900">{summary.guard_name}</dd>
        <dt className="text-gray-600">{t('pat.started')}</dt>
        <dd>{formatDateTime(summary.started_at, tz, lang)}</dd>
        <dt className="text-gray-600">{t('pat.ended')}</dt>
        <dd>{formatDateTime(summary.ended_at, tz, lang)}</dd>
        <dt className="text-gray-600">{t('common.status')}</dt>
        <dd>
          <StatusBadge status={summary.status} />
        </dd>
      </dl>
      {isLoading ? (
        <TableSkeleton rows={4} cols={3} />
      ) : error ? (
        <ErrorState error={error} onRetry={() => refetch()} />
      ) : data ? (
        <>
          <div className="mb-4">
            <div className="mb-1 flex justify-between text-xs text-gray-700">
              <span>{t('pat.progress', { scanned: data.scanned, total: data.total })}</span>
              <span>{Math.round(data.completion_pct)}%</span>
            </div>
            <ProgressBar pct={data.completion_pct} label={t('pat.completion')} />
          </div>
          <CheckpointTimeline checkpoints={data.checkpoints} tz={tz} />
        </>
      ) : null}
    </Modal>
  )
}

export function SessionsTab({ site }: { site: Site }) {
  const { t, lang } = useI18n()
  const tz = site.timezone
  const [from, setFrom] = useState(() => addDays(todayIn(tz), -6))
  const [to, setTo] = useState(() => todayIn(tz))
  const [selected, setSelected] = useState<SessionSummary | null>(null)
  const valid = from && to && from <= to && daysBetween(from, to) <= 62

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['patrol-sessions', site.id, from, to],
    queryFn: () => apiGet<SessionSummary[]>('/patrol/sessions', { site_id: site.id, from, to }),
    enabled: !!valid,
  })

  const columns: Column<SessionSummary>[] = [
    { key: 'started', header: t('pat.started'), cell: (r) => <span className="whitespace-nowrap">{formatDateTime(r.started_at, tz, lang)}</span> },
    { key: 'route', header: t('pat.route'), cell: (r) => r.route_name },
    { key: 'guard', header: t('common.guard'), cell: (r) => r.guard_name },
    { key: 'status', header: t('common.status'), cell: (r) => <StatusBadge status={r.status} /> },
    { key: 'scanned', header: t('pat.scanned'), numeric: true, cell: (r) => `${r.scanned_count}/${r.total_checkpoints}` },
    { key: 'sla', header: t('pat.slaBreaches'), numeric: true, cell: (r) => (r.sla_breaches ? <span className="font-semibold text-red-700">{r.sla_breaches}</span> : 0) },
    {
      key: 'pct',
      header: t('pat.completion'),
      cell: (r) => (
        <div className="flex w-32 items-center gap-2">
          <ProgressBar pct={r.completion_pct} label={`${r.route_name} ${r.guard_name}`} />
          <span className="text-xs tabular-nums">{formatNumber(r.completion_pct, lang)}%</span>
        </div>
      ),
    },
    {
      key: 'view',
      header: <span className="sr-only">{t('common.actions')}</span>,
      cell: (r) => (
        <Button size="sm" variant="secondary" onClick={() => setSelected(r)}>
          {t('common.details')}
        </Button>
      ),
    },
  ]

  return (
    <>
      <Toolbar>
        <Field label={t('common.from')}>
          <Input type="date" value={from} onChange={(e) => setFrom(e.target.value)} />
        </Field>
        <Field label={t('common.to')}>
          <Input type="date" value={to} onChange={(e) => setTo(e.target.value)} />
        </Field>
      </Toolbar>
      {!valid ? (
        <p role="alert" className="text-sm text-red-700">
          {t('rep.rangeTooLong', { n: 62 })}
        </p>
      ) : (
        <Table
          columns={columns}
          rows={data}
          rowKey={(r) => r.session_id}
          isLoading={isLoading}
          error={error}
          onRetry={() => refetch()}
          empty={t('pat.noSessions')}
        />
      )}
      {selected && <SessionDetail site={site} summary={selected} onClose={() => setSelected(null)} />}
    </>
  )
}
