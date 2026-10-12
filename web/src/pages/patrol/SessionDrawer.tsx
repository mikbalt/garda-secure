import { useQuery } from '@tanstack/react-query'
import { CheckCircle2, Circle, Clock, Flag, Hand, Play, QrCode, TimerOff, XCircle, type LucideIcon } from 'lucide-react'
import { apiGet } from '../../api/client'
import { FlagBadges } from '../../components/FlagBadges'
import { TableSkeleton } from '../../components/LoadingSkeleton'
import { StatusBadge } from '../../components/ui/Badge'
import { Modal } from '../../components/ui/Modal'
import { ErrorState } from '../../components/ui/States'
import { useI18n } from '../../i18n/context'
import { formatDay, formatTime, localDateOf } from '../../lib/time'
import { cpTone, fmtDuration, useNow } from './format'
import { Avatar, CheckpointLegend, SegmentedBar } from './kit'
import type { CheckpointProgress, CheckpointStatus, SessionProgress, Site } from '../../types'

const cpIcon: Record<CheckpointStatus, LucideIcon> = {
  on_time: CheckCircle2,
  late: Clock,
  overdue: TimerOff,
  missed: XCircle,
  pending: Circle,
}

function Node({ icon: Icon, cls }: { icon: LucideIcon; cls: string }) {
  return (
    <span className={`absolute left-0 top-0 flex h-8 w-8 items-center justify-center rounded-full ring-1 ${cls}`} aria-hidden="true">
      <Icon className="h-4 w-4" />
    </span>
  )
}

function CheckpointRow({ cp, tz, last }: { cp: CheckpointProgress; tz: string; last: boolean }) {
  const { t } = useI18n()
  const tone = cpTone[cp.status] ?? cpTone.pending
  const lateMs = cp.scanned_at && cp.due_at ? Date.parse(cp.scanned_at) - Date.parse(cp.due_at) : 0
  return (
    <li className="relative pb-5 pl-12">
      {!last && <span className="absolute bottom-0 left-4 top-8 w-px bg-gray-200" aria-hidden="true" />}
      <Node icon={cpIcon[cp.status] ?? Circle} cls={tone.ring} />
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1 pt-1">
        <span className="text-xs font-semibold tabular-nums text-gray-500">{String(cp.order_index + 1).padStart(2, '0')}</span>
        <span className="text-sm font-semibold text-gray-900">{cp.name}</span>
        <StatusBadge status={cp.status} />
        <FlagBadges flags={cp.scan_flags} />
      </div>
      <div className="mt-1 flex flex-wrap gap-x-4 gap-y-1 text-xs text-gray-600">
        {cp.scanned_at ? (
          <span className="inline-flex items-center gap-1 font-medium text-gray-700">
            {cp.method === 'manual' ? <Hand className="h-3.5 w-3.5 text-gray-500" aria-hidden="true" /> : <QrCode className="h-3.5 w-3.5 text-gray-500" aria-hidden="true" />}
            {t('pat.scannedAt', { time: formatTime(cp.scanned_at, tz) })} · {t(cp.method === 'manual' ? 'pat.method.manual' : 'pat.method.qr')}
          </span>
        ) : (
          <span>{t('ops.pat.notScanned')}</span>
        )}
        {cp.due_at ? <span>{t('pat.due', { time: formatTime(cp.due_at, tz) })}</span> : <span>{t('ops.pat.noLimit')}</span>}
        {lateMs >= 60_000 && <span className={`font-medium ${tone.text}`}>{t('ops.pat.lateBy', { d: fmtDuration(lateMs, t) })}</span>}
      </div>
    </li>
  )
}

/** Vertical timeline: start → checkpoints → end. */
export function SessionTimeline({ progress, tz }: { progress: SessionProgress; tz: string }) {
  const { t } = useI18n()
  const s = progress.session
  const ended = s.status !== 'in_progress' && s.ended_at
  return (
    <ol aria-label={t('pat.timeline')}>
      <li className="relative pb-5 pl-12">
        <span className="absolute bottom-0 left-4 top-8 w-px bg-gray-200" aria-hidden="true" />
        <Node icon={Play} cls="bg-primary-50 text-primary-600 ring-primary-200" />
        <p className="pt-1.5 text-sm font-medium text-gray-900">{t('ops.pat.startedNode', { time: formatTime(s.started_at, tz) })}</p>
      </li>
      {progress.checkpoints.map((cp, i) => (
        <CheckpointRow key={cp.id} cp={cp} tz={tz} last={!ended && i === progress.checkpoints.length - 1} />
      ))}
      {ended && (
        <li className="relative pl-12">
          <Node icon={Flag} cls={s.status === 'completed' ? 'bg-emerald-50 text-emerald-600 ring-emerald-200' : 'bg-violet-50 text-violet-600 ring-violet-200'} />
          <p className="pt-1.5 text-sm font-medium text-gray-900">
            {t(s.status === 'completed' ? 'ops.pat.endedNode' : 'ops.pat.abandonedNode', { time: formatTime(s.ended_at, tz) })}
          </p>
        </li>
      )}
    </ol>
  )
}

function Stat({ label, value, sub, tone = 'text-gray-900' }: { label: string; value: string; sub?: string; tone?: string }) {
  return (
    <div className="rounded-xl border border-gray-200/80 bg-gray-50/60 px-3.5 py-3">
      <dt className="text-xs font-medium text-gray-500">{label}</dt>
      <dd className={`mt-0.5 text-lg font-semibold tabular-nums tracking-tight ${tone}`}>{value}</dd>
      {sub && <dd className="text-xs text-gray-500">{sub}</dd>}
    </div>
  )
}

/** Drawer with a session's details and checkpoint timeline. */
export function SessionDrawer({ site, sessionId, guardName, onClose }: { site: Site; sessionId: string; guardName?: string; onClose: () => void }) {
  const { t, lang } = useI18n()
  const tz = site.timezone
  const now = useNow(30_000)
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['patrol-session', sessionId],
    queryFn: () => apiGet<SessionProgress>(`/patrol/sessions/${sessionId}`),
    refetchInterval: (q) => (q.state.data?.session.status === 'in_progress' ? 10_000 : false),
  })

  const s = data?.session
  const breaches = data?.checkpoints.filter((c) => c.status === 'late' || c.status === 'overdue' || c.status === 'missed').length ?? 0
  const endMs = s?.ended_at ? Date.parse(s.ended_at) : now

  return (
    <Modal open onClose={onClose} variant="drawer" title={t('ops.pat.sessionTitle')}>
      {isLoading ? (
        <TableSkeleton rows={6} cols={2} />
      ) : error ? (
        <ErrorState error={error} onRetry={() => refetch()} />
      ) : data && s ? (
        <div className="space-y-6">
          <div className="flex items-start gap-3">
            <Avatar name={guardName} className="h-11 w-11 text-sm" />
            <div className="min-w-0 flex-1">
              <p className="truncate text-base font-semibold text-gray-900">{guardName || '—'}</p>
              <p className="truncate text-sm text-gray-500">
                {data.route_name} · {formatDay(localDateOf(s.started_at, tz), lang)}
              </p>
            </div>
            <StatusBadge status={s.status} />
          </div>

          <dl className="grid grid-cols-3 gap-2.5">
            <Stat label={t('pat.completion')} value={`${Math.round(data.completion_pct)}%`} sub={t('ops.pat.cpOf', { a: data.scanned, n: data.total })} />
            <Stat
              label={t('ops.pat.duration')}
              value={fmtDuration(endMs - Date.parse(s.started_at), t)}
              sub={`${formatTime(s.started_at, tz)} – ${s.ended_at ? formatTime(s.ended_at, tz) : t('ops.pat.now')}`}
            />
            <Stat label={t('ops.pat.issues')} value={String(breaches)} tone={breaches ? 'text-orange-600' : 'text-gray-900'} sub={breaches ? t('ops.pat.breachSub') : t('ops.pat.noBreachSub')} />
          </dl>

          <div className="space-y-2">
            <SegmentedBar checkpoints={data.checkpoints} label={t('ops.pat.cpOf', { a: data.scanned, n: data.total })} thick />
            <CheckpointLegend />
          </div>

          <section aria-labelledby="sess-timeline">
            <h3 id="sess-timeline" className="mb-1 text-sm font-semibold text-gray-900">
              {t('pat.timeline')}
            </h3>
            <p className="mb-4 text-xs text-gray-500">{t('ops.pat.timelineHint')}</p>
            <SessionTimeline progress={data} tz={tz} />
          </section>
        </div>
      ) : null}
    </Modal>
  )
}
