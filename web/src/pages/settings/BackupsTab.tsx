import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CalendarClock, CheckCircle2, DatabaseBackup, HardDrive, Loader2, ShieldCheck, XCircle } from 'lucide-react'
import apiClient, { apiGet } from '../../api/client'
import { TableSkeleton } from '../../components/LoadingSkeleton'
import { StatusBadge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { EmptyState, ErrorState } from '../../components/ui/States'
import { Table, type Column } from '../../components/ui/Table'
import { useErrorToast, useToast } from '../../components/toast/context'
import { useSite } from '../../context/site'
import { useI18n } from '../../i18n/context'
import { formatBytes, formatDateTime } from '../../lib/time'
import { relativeTime } from './format'
import { SectionHeader } from './kit'
import type { Backup } from '../../types'

const busy = (b: Backup) => b.status === 'pending' || b.status === 'running'

export function BackupsTab() {
  const { t, lang } = useI18n()
  const { site } = useSite()
  const qc = useQueryClient()
  const toast = useToast()
  const onError = useErrorToast()
  const tz = site?.timezone ?? 'Asia/Jakarta'

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['backups'],
    queryFn: () => apiGet<Backup[]>('/backups'),
    // Poll while a backup is queued or running.
    refetchInterval: (q) => (q.state.data?.some(busy) ? 3_000 : false),
  })

  const run = useMutation({
    mutationFn: () => apiClient.post('/backups'),
    onSuccess: () => {
      toast.success(t('bk.queued'))
      qc.invalidateQueries({ queryKey: ['backups'] })
    },
    onError,
  })

  const all = [...(data ?? [])].sort((a, b) => b.created_at.localeCompare(a.created_at))
  const running = all.find(busy)
  const lastDone = all.find((b) => !busy(b))
  const lastOk = all.find((b) => b.status === 'succeeded')
  const now = Date.now()

  const columns: Column<Backup>[] = [
    {
      key: 'created',
      header: t('common.time'),
      cell: (b) => (
        <span className="whitespace-nowrap">
          <span className="block text-gray-900 tabular-nums">{formatDateTime(b.created_at, tz, lang)}</span>
          <span className="block text-xs text-gray-500">{relativeTime(b.created_at, lang, now)}</span>
        </span>
      ),
    },
    { key: 'trigger', header: t('bk.trigger'), cell: (b) => t(b.trigger === 'manual' ? 'adm.bk.manual' : 'adm.bk.auto') },
    {
      key: 'status',
      header: t('common.status'),
      cell: (b) => (
        <div className="min-w-[8rem]">
          <StatusBadge status={b.status} />
          {b.error && <p className="mt-1 max-w-xs break-words text-xs text-red-700">{b.error}</p>}
        </div>
      ),
    },
    { key: 'size', header: t('bk.size'), numeric: true, cell: (b) => formatBytes(b.size_bytes) },
    { key: 'object', header: t('adm.bk.file'), cell: (b) => <span className="break-all font-mono text-xs text-gray-500">{b.object_key}</span> },
  ]

  const action = (
    <Button onClick={() => run.mutate()} loading={run.isPending} disabled={!!running}>
      {!run.isPending && <DatabaseBackup className="h-4 w-4" aria-hidden="true" />}
      {t('bk.now')}
    </Button>
  )

  let status: { icon: typeof CheckCircle2; ring: string; title: string; body: string } | null = null
  if (running) {
    status = { icon: Loader2, ring: 'bg-primary-50 text-primary-600 ring-primary-100', title: t('adm.bk.running'), body: t('adm.bk.runningBody', { when: relativeTime(running.created_at, lang, now) }) }
  } else if (lastDone?.status === 'succeeded') {
    status = {
      icon: CheckCircle2,
      ring: 'bg-emerald-50 text-emerald-600 ring-emerald-100',
      title: t('adm.bk.okTitle', { when: relativeTime(lastDone.finished_at ?? lastDone.created_at, lang, now) }),
      body: t('adm.bk.okBody', { time: formatDateTime(lastDone.finished_at ?? lastDone.created_at, tz, lang), size: formatBytes(lastDone.size_bytes) }),
    }
  } else if (lastDone?.status === 'failed') {
    status = {
      icon: XCircle,
      ring: 'bg-red-50 text-red-600 ring-red-100',
      title: t('adm.bk.failTitle', { when: relativeTime(lastDone.created_at, lang, now) }),
      body: lastOk ? t('adm.bk.failBody', { time: formatDateTime(lastOk.finished_at ?? lastOk.created_at, tz, lang) }) : t('adm.bk.failNone'),
    }
  }

  return (
    <>
      <SectionHeader title={t('adm.set.backups')} description={t('adm.bk.intro')} actions={all.length > 0 && action} />

      {isLoading ? (
        <TableSkeleton />
      ) : error ? (
        <ErrorState error={error} onRetry={() => refetch()} />
      ) : all.length === 0 ? (
        <EmptyState icon={DatabaseBackup} title={t('adm.bk.emptyTitle')} message={t('adm.bk.empty')}>
          {action}
        </EmptyState>
      ) : (
        <div className="space-y-6">
          {status && (
            <div className="flex flex-wrap items-center gap-4 rounded-2xl border border-gray-200/80 bg-white p-5 shadow-card" role="status">
              <span className={`flex h-12 w-12 shrink-0 items-center justify-center rounded-xl ring-1 ${status.ring}`}>
                <status.icon className={`h-6 w-6 ${running ? 'animate-spin' : ''}`} aria-hidden="true" />
              </span>
              <div className="min-w-0 flex-1">
                <p className="text-base font-semibold text-gray-900">{status.title}</p>
                <p className="mt-0.5 text-sm text-gray-500">{status.body}</p>
              </div>
              <dl className="grid w-full grid-cols-3 gap-2 sm:w-auto">
                {[
                  { icon: HardDrive, label: t('bk.size'), value: formatBytes(lastOk?.size_bytes) },
                  { icon: CalendarClock, label: t('adm.bk.schedule'), value: t('adm.bk.daily') },
                  { icon: ShieldCheck, label: t('adm.bk.kept'), value: t('adm.bk.keptVal') },
                ].map((m) => (
                  <div key={m.label} className="rounded-lg bg-gray-50 px-3 py-2 ring-1 ring-inset ring-gray-100">
                    <dt className="flex items-center gap-1 text-[11px] text-gray-500">
                      <m.icon className="h-3 w-3" aria-hidden="true" />
                      {m.label}
                    </dt>
                    <dd className="mt-0.5 whitespace-nowrap text-sm font-semibold tabular-nums text-gray-900">{m.value}</dd>
                  </div>
                ))}
              </dl>
            </div>
          )}

          <div>
            <h3 className="text-[15px] font-semibold text-gray-900">{t('adm.bk.history')}</h3>
            <p className="mb-3 text-sm text-gray-500">{t('adm.bk.historyDesc')}</p>
            <Table columns={columns} rows={all} rowKey={(b) => b.id} caption={t('adm.bk.history')} />
          </div>
        </div>
      )}
    </>
  )
}
