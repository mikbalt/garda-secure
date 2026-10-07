import { useQuery } from '@tanstack/react-query'
import { apiGet } from '../../api/client'
import { TableSkeleton } from '../../components/LoadingSkeleton'
import { StatusBadge } from '../../components/ui/Badge'
import { EmptyState, ErrorState, Notice } from '../../components/ui/States'
import { useI18n } from '../../i18n/context'
import { formatTime } from '../../lib/time'
import { CheckpointTimeline, ProgressBar } from './shared'
import type { LivePatrol, Site } from '../../types'

/** Running patrols, polled every 10 s (D-014). */
export function LiveTab({ site }: { site: Site }) {
  const { t } = useI18n()
  const tz = site.timezone
  const { data, isLoading, error, refetch, dataUpdatedAt } = useQuery({
    queryKey: ['patrol-live', site.id],
    queryFn: () => apiGet<LivePatrol[]>('/patrol/live', { site_id: site.id }),
    refetchInterval: 10_000,
  })

  if (isLoading) return <TableSkeleton />
  if (error) return <ErrorState error={error} onRetry={() => refetch()} />

  return (
    <div>
      <Notice>
        {t('pat.liveHint')} {dataUpdatedAt ? `(${formatTime(new Date(dataUpdatedAt).toISOString(), tz)})` : ''}
      </Notice>
      <div aria-live="polite">
        {!data || data.length === 0 ? (
          <EmptyState message={t('pat.noLive')} />
        ) : (
          <ul className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            {data.map(({ guard_name, progress: p }) => {
              const next = p.next_index != null ? p.checkpoints[p.next_index] : undefined
              return (
                <li key={p.session.id} className="rounded-xl border border-gray-200 bg-white p-4 shadow-sm">
                  <div className="flex flex-wrap items-start justify-between gap-2">
                    <div>
                      <p className="font-semibold text-gray-900">{guard_name || '—'}</p>
                      <p className="text-sm text-gray-700">
                        {p.route_name} · {t('pat.started')} {formatTime(p.session.started_at, tz)}
                      </p>
                    </div>
                    <StatusBadge status={p.session.status} />
                  </div>
                  <div className="mt-3">
                    <div className="mb-1 flex justify-between text-xs text-gray-700">
                      <span>{t('pat.progress', { scanned: p.scanned, total: p.total })}</span>
                      <span className="tabular-nums">{Math.round(p.completion_pct)}%</span>
                    </div>
                    <ProgressBar pct={p.completion_pct} label={`${guard_name} — ${p.route_name}`} />
                  </div>
                  {next && <p className="mt-2 text-sm text-gray-800">{t('pat.next', { name: next.name })}</p>}
                  <div className="mt-3">
                    <CheckpointTimeline checkpoints={p.checkpoints} tz={tz} compact />
                  </div>
                </li>
              )
            })}
          </ul>
        )}
      </div>
    </div>
  )
}
