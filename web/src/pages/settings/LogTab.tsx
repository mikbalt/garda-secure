import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { RotateCw } from 'lucide-react'
import apiClient, { apiGet } from '../../api/client'
import { Badge, StatusBadge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Field, Select } from '../../components/ui/Form'
import { Toolbar } from '../../components/ui/States'
import { Table, type Column } from '../../components/ui/Table'
import { useErrorToast, useToast } from '../../components/toast/context'
import { useSite } from '../../context/site'
import { useI18n, translateMaybe } from '../../i18n/context'
import { formatDateTime } from '../../lib/time'
import type { NotificationLogEntry } from '../../types'

/** Delivery log of all sites of the realm (FR-NOT-04). */
export function LogTab() {
  const { t, lang } = useI18n()
  const { sites, site } = useSite()
  const qc = useQueryClient()
  const toast = useToast()
  const onError = useErrorToast()
  const [status, setStatus] = useState('')
  const [siteFilter, setSiteFilter] = useState('')
  const tz = site?.timezone ?? 'Asia/Jakarta'

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['notification-log', siteFilter, status],
    queryFn: () => apiGet<NotificationLogEntry[]>('/notifications/log', { site_id: siteFilter, status, limit: 200 }),
    refetchInterval: 30_000,
  })

  const retry = useMutation({
    mutationFn: (id: string) => apiClient.post(`/notifications/log/${id}/retry`),
    onSuccess: () => {
      toast.success(t('log.retried'))
      qc.invalidateQueries({ queryKey: ['notification-log'] })
    },
    onError,
  })

  const siteName = (id?: string) => (id ? sites.find((s) => s.id === id)?.name ?? '—' : '—')

  const columns: Column<NotificationLogEntry>[] = [
    { key: 'created', header: t('common.created'), cell: (r) => <span className="whitespace-nowrap">{formatDateTime(r.created_at, tz, lang)}</span> },
    { key: 'site', header: t('common.site'), cell: (r) => siteName(r.site_id) },
    { key: 'event', header: t('rule.event'), cell: (r) => translateMaybe(lang, `event.${r.event_type}`, r.event_type) },
    { key: 'channel', header: t('rule.channel'), cell: (r) => <Badge tone={r.channel === 'email' ? 'blue' : 'green'}>{translateMaybe(lang, `channel.${r.channel}`, r.channel)}</Badge> },
    { key: 'recipient', header: t('log.recipient'), cell: (r) => <span className="font-mono text-xs">{r.recipient}</span> },
    { key: 'subject', header: t('log.subject'), cell: (r) => <span className="line-clamp-2 max-w-xs" title={r.body}>{r.subject}</span> },
    {
      key: 'status',
      header: t('common.status'),
      cell: (r) => (
        <div>
          <StatusBadge status={r.status} />
          {r.last_error && <div className="mt-0.5 max-w-xs break-words text-xs text-red-800">{r.last_error}</div>}
        </div>
      ),
    },
    { key: 'attempts', header: t('log.attempts'), numeric: true, cell: (r) => r.attempts },
    { key: 'sent', header: t('log.sentAt'), cell: (r) => <span className="whitespace-nowrap">{formatDateTime(r.sent_at, tz, lang)}</span> },
    {
      key: 'actions',
      header: <span className="sr-only">{t('common.actions')}</span>,
      cell: (r) =>
        r.status === 'failed' ? (
          <Button size="sm" variant="secondary" loading={retry.isPending && retry.variables === r.id} onClick={() => retry.mutate(r.id)}>
            <RotateCw className="h-3.5 w-3.5" aria-hidden="true" />
            {t('log.retry')}
          </Button>
        ) : null,
    },
  ]

  return (
    <>
      <Toolbar>
        <Field label={t('common.site')}>
          <Select value={siteFilter} onChange={(e) => setSiteFilter(e.target.value)}>
            <option value="">{t('common.all')}</option>
            {sites.map((s) => (
              <option key={s.id} value={s.id}>
                {s.name}
              </option>
            ))}
          </Select>
        </Field>
        <Field label={t('common.status')}>
          <Select value={status} onChange={(e) => setStatus(e.target.value)}>
            <option value="">{t('common.all')}</option>
            <option value="queued">{t('status.queued')}</option>
            <option value="sent">{t('status.sent')}</option>
            <option value="failed">{t('status.failed')}</option>
          </Select>
        </Field>
        <Button variant="secondary" onClick={() => refetch()}>
          <RotateCw className="h-4 w-4" aria-hidden="true" />
          {t('common.refresh')}
        </Button>
      </Toolbar>
      <Table columns={columns} rows={data} rowKey={(r) => r.id} isLoading={isLoading} error={error} onRetry={() => refetch()} empty={t('log.empty')} />
    </>
  )
}
