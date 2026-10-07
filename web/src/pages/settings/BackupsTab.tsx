import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { DatabaseBackup, RotateCw } from 'lucide-react'
import apiClient, { apiGet } from '../../api/client'
import { StatusBadge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Notice, Toolbar } from '../../components/ui/States'
import { Table, type Column } from '../../components/ui/Table'
import { useErrorToast, useToast } from '../../components/toast/context'
import { useSite } from '../../context/site'
import { useI18n } from '../../i18n/context'
import { formatBytes, formatDateTime } from '../../lib/time'
import type { Backup } from '../../types'

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
    refetchInterval: (q) => (q.state.data?.some((b) => b.status === 'pending' || b.status === 'running') ? 10_000 : false),
  })

  const run = useMutation({
    mutationFn: () => apiClient.post('/backups'),
    onSuccess: () => {
      toast.success(t('bk.queued'))
      qc.invalidateQueries({ queryKey: ['backups'] })
    },
    onError,
  })

  const columns: Column<Backup>[] = [
    { key: 'created', header: t('common.created'), cell: (b) => <span className="whitespace-nowrap">{formatDateTime(b.created_at, tz, lang)}</span> },
    { key: 'trigger', header: t('bk.trigger'), cell: (b) => t(b.trigger === 'manual' ? 'bk.trigger.manual' : 'bk.trigger.scheduled') },
    {
      key: 'status',
      header: t('common.status'),
      cell: (b) => (
        <div>
          <StatusBadge status={b.status} />
          {b.error && <div className="mt-0.5 max-w-xs break-words text-xs text-red-800">{b.error}</div>}
        </div>
      ),
    },
    { key: 'size', header: t('bk.size'), numeric: true, cell: (b) => formatBytes(b.size_bytes) },
    { key: 'finished', header: t('bk.finished'), cell: (b) => <span className="whitespace-nowrap">{formatDateTime(b.finished_at, tz, lang)}</span> },
    { key: 'object', header: t('bk.object'), cell: (b) => <span className="break-all font-mono text-xs">{b.object_key}</span> },
  ]

  return (
    <>
      <Notice>{t('ch.installWide')}</Notice>
      <Toolbar>
        <Button variant="secondary" onClick={() => refetch()}>
          <RotateCw className="h-4 w-4" aria-hidden="true" />
          {t('common.refresh')}
        </Button>
        <Button className="ml-auto" onClick={() => run.mutate()} loading={run.isPending}>
          <DatabaseBackup className="h-4 w-4" aria-hidden="true" />
          {t('bk.now')}
        </Button>
      </Toolbar>
      <Table columns={columns} rows={data} rowKey={(b) => b.id} isLoading={isLoading} error={error} onRetry={() => refetch()} empty={t('bk.empty')} />
    </>
  )
}
