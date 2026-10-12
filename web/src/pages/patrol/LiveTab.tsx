import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ChevronRight, Clock4, MapPin, Route as RouteIcon, TimerOff } from 'lucide-react'
import { apiGet } from '../../api/client'
import { CardSkeleton } from '../../components/LoadingSkeleton'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { EmptyState, ErrorState } from '../../components/ui/States'
import { useI18n } from '../../i18n/context'
import { formatTime } from '../../lib/time'
import { fmtDuration, useNow } from './format'
import { Avatar, CheckpointLegend, LiveIndicator, Meta, SegmentedBar } from './kit'
import { SessionDrawer } from './SessionDrawer'
import type { LivePatrol, Site } from '../../types'

function LiveCard({ patrol, tz, now, onOpen }: { patrol: LivePatrol; tz: string; now: number; onOpen: () => void }) {
  const { t } = useI18n()
  const p = patrol.progress
  const next = p.next_index != null ? p.checkpoints[p.next_index] : undefined
  const overdue = p.checkpoints.filter((c) => c.status === 'overdue')
  const firstOverdue = overdue[0]
  return (
    <li>
      <button
        type="button"
        onClick={onOpen}
        className={`group flex h-full w-full flex-col rounded-2xl border bg-white p-5 text-left shadow-card transition-shadow hover:shadow-raised focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 ${
          overdue.length ? 'border-orange-200' : 'border-gray-200/80'
        }`}
      >
        <div className="flex w-full items-start gap-3">
          <Avatar name={patrol.guard_name} />
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-semibold text-gray-900">{patrol.guard_name || '—'}</p>
            <p className="truncate text-xs text-gray-500">{p.route_name}</p>
          </div>
          {overdue.length ? (
            <Badge tone="orange" dot>
              {t('status.overdue')}
            </Badge>
          ) : (
            <Badge tone="blue" dot>
              {t('ops.pat.onTrack')}
            </Badge>
          )}
        </div>

        <div className="mt-4 w-full">
          <SegmentedBar checkpoints={p.checkpoints} label={t('ops.pat.cpOf', { a: p.scanned, n: p.total })} thick />
          <div className="mt-2 flex items-center justify-between text-xs">
            <span className="font-medium text-gray-700">{t('ops.pat.cpOf', { a: p.scanned, n: p.total })}</span>
            <span className="tabular-nums text-gray-500">{Math.round(p.completion_pct)}%</span>
          </div>
        </div>

        <div className="mt-3 flex w-full flex-col gap-1.5 text-xs text-gray-600">
          {next ? (
            <Meta icon={MapPin}>
              <span className="text-gray-500">{t('ops.pat.nextCp')}:</span> <span className="font-medium text-gray-800">{next.name}</span>
              {next.due_at && next.status !== 'overdue' && <span className="text-gray-500"> · {t('ops.pat.dueBy', { time: formatTime(next.due_at, tz) })}</span>}
            </Meta>
          ) : (
            <Meta icon={MapPin}>{t('ops.pat.allScanned')}</Meta>
          )}
          <Meta icon={Clock4}>
            {t('ops.pat.startedRunning', { time: formatTime(p.session.started_at, tz), d: fmtDuration(now - Date.parse(p.session.started_at), t) })}
          </Meta>
        </div>

        {firstOverdue && (
          <div className="mt-3 flex w-full items-start gap-2 rounded-lg bg-orange-50 px-3 py-2 text-xs font-medium text-orange-800 ring-1 ring-inset ring-orange-200">
            <TimerOff className="mt-px h-3.5 w-3.5 shrink-0" aria-hidden="true" />
            <span>
              {firstOverdue.due_at
                ? t('ops.pat.overdueRow', { cp: firstOverdue.name, d: fmtDuration(now - Date.parse(firstOverdue.due_at), t) })
                : t('ops.pat.overdueRowShort', { cp: firstOverdue.name })}
              {overdue.length > 1 && ` ${t('ops.pat.moreOverdue', { n: overdue.length - 1 })}`}
            </span>
          </div>
        )}

        <span className="mt-auto flex w-full items-center justify-end gap-1 pt-3 text-xs font-medium text-primary-700 group-hover:text-primary-800">
          {t('ops.pat.openTimeline')}
          <ChevronRight className="h-3.5 w-3.5" aria-hidden="true" />
        </span>
      </button>
    </li>
  )
}

/** Running patrols, polled every 10 s (D-014). */
export function LiveTab({ site, onShowHistory }: { site: Site; onShowHistory: () => void }) {
  const { t } = useI18n()
  const tz = site.timezone
  const now = useNow(15_000)
  const [open, setOpen] = useState<LivePatrol | null>(null)
  const { data, isLoading, error, refetch, dataUpdatedAt, isFetching } = useQuery({
    queryKey: ['patrol-live', site.id],
    queryFn: () => apiGet<LivePatrol[]>('/patrol/live', { site_id: site.id }),
    refetchInterval: 10_000,
  })

  if (error) return <ErrorState error={error} onRetry={() => refetch()} />

  const list = data ?? []
  const overdueCount = list.filter((p) => p.progress.checkpoints.some((c) => c.status === 'overdue')).length
  // Patrols with overdue checkpoints first.
  const sorted = [...list].sort(
    (a, b) => Number(b.progress.checkpoints.some((c) => c.status === 'overdue')) - Number(a.progress.checkpoints.some((c) => c.status === 'overdue'))
  )

  return (
    <div>
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap items-center gap-2 text-sm text-gray-600">
          {!isLoading && list.length > 0 && (
            <>
              <span className="font-medium text-gray-900">{t('ops.pat.liveSummary', { n: list.length })}</span>
              {overdueCount > 0 && (
                <Badge tone="orange" dot>
                  {t('ops.pat.overdueCount', { n: overdueCount })}
                </Badge>
              )}
            </>
          )}
        </div>
        <LiveIndicator updatedAt={dataUpdatedAt} fetching={isFetching} />
      </div>

      <div aria-live="polite" aria-busy={isLoading}>
        {isLoading ? (
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
            {Array.from({ length: 3 }).map((_, i) => (
              <CardSkeleton key={i} />
            ))}
          </div>
        ) : list.length === 0 ? (
          <EmptyState icon={RouteIcon} title={t('ops.pat.liveEmptyTitle')} message={t('ops.pat.liveEmptyBody')}>
            <Button variant="secondary" onClick={onShowHistory}>
              {t('ops.pat.viewHistory')}
            </Button>
          </EmptyState>
        ) : (
          <>
            <ul className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
              {sorted.map((p) => (
                <LiveCard key={p.progress.session.id} patrol={p} tz={tz} now={now} onOpen={() => setOpen(p)} />
              ))}
            </ul>
            <div className="mt-4">
              <CheckpointLegend />
            </div>
          </>
        )}
      </div>
      {open && <SessionDrawer site={site} sessionId={open.progress.session.id} guardName={open.guard_name} onClose={() => setOpen(null)} />}
    </div>
  )
}
