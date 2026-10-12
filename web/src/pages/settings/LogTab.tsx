import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { History, RotateCw } from 'lucide-react'
import apiClient, { apiGet } from '../../api/client'
import { StatusBadge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Select } from '../../components/ui/Form'
import { EmptyState } from '../../components/ui/States'
import { buttonClass } from '../../components/ui/styles'
import { Table, type Column } from '../../components/ui/Table'
import { useErrorToast, useToast } from '../../components/toast/context'
import { useSite } from '../../context/site'
import { useI18n, translateMaybe } from '../../i18n/context'
import { formatDateTime } from '../../lib/time'
import { CHANNEL_ICON, CHANNEL_TONE, EVENT_META, displayRecipient } from './channelMeta'
import { deliveryErrorText, relativeTime } from './format'
import { SectionHeader } from './kit'
import type { DeliveryStatus, NotificationLogEntry } from '../../types'

type Filter = '' | DeliveryStatus
const FILTERS: Filter[] = ['', 'failed', 'queued', 'sent']

const chipTone: Record<Filter, string> = {
  '': 'bg-gray-400',
  failed: 'bg-red-500',
  queued: 'bg-primary-500',
  sent: 'bg-emerald-500',
}

/**
 * Delivery log (FR-NOT-04): `site` = all sites of the realm (admin);
 * `ops` = install-wide operations alerts (platform-admin, D-027).
 */
export function LogTab({ mode = 'site' }: { mode?: 'site' | 'ops' }) {
  const ops = mode === 'ops'
  const base = ops ? '/notifications/ops-log' : '/notifications/log'
  const { t, lang } = useI18n()
  const { sites, site } = useSite()
  const qc = useQueryClient()
  const toast = useToast()
  const onError = useErrorToast()
  const [status, setStatus] = useState<Filter>('')
  const [siteFilter, setSiteFilter] = useState('')
  const tz = site?.timezone ?? 'Asia/Jakarta'

  // The latest 200 deliveries; status chips filter (and count) client-side.
  const { data, isLoading, error, refetch, isFetching } = useQuery({
    queryKey: ops ? ['ops-log'] : ['notification-log', siteFilter],
    queryFn: () => apiGet<NotificationLogEntry[]>(base, ops ? { limit: 200 } : { site_id: siteFilter, limit: 200 }),
    refetchInterval: 30_000,
  })

  const retry = useMutation({
    mutationFn: (id: string) => apiClient.post(`${base}/${id}/retry`),
    onSuccess: () => {
      toast.success(t('log.retried'))
      qc.invalidateQueries({ queryKey: ops ? ['ops-log'] : ['notification-log'] })
    },
    onError,
  })

  const all = data ?? []
  const rows = status ? all.filter((r) => r.status === status) : all
  const count = (f: Filter) => (f ? all.filter((r) => r.status === f).length : all.length)
  const siteName = (id?: string) => (id ? sites.find((s) => s.id === id)?.name ?? '—' : '—')
  const now = Date.now()

  const columns: Column<NotificationLogEntry>[] = [
    {
      key: 'created',
      header: t('common.time'),
      cell: (r) => (
        <span className="whitespace-nowrap" title={formatDateTime(r.created_at, tz, lang)}>
          <span className="block text-gray-900">{relativeTime(r.created_at, lang, now)}</span>
          <span className="block text-xs text-gray-500 tabular-nums">{formatDateTime(r.created_at, tz, lang)}</span>
        </span>
      ),
    },
    {
      key: 'event',
      header: t('adm.log.message'),
      cell: (r) => {
        const meta = EVENT_META[r.event_type]
        const Icon = CHANNEL_ICON[r.channel]
        return (
          <div className="relative min-w-[14rem] max-w-[22rem]">
            <p className="font-medium text-gray-900">{meta ? t(meta.title) : translateMaybe(lang, `event.${r.event_type}`, r.event_type)}</p>
            <p className="mt-0.5 flex items-center gap-1.5 text-sm text-gray-700">
              <Icon className={`h-3.5 w-3.5 shrink-0 ${CHANNEL_TONE[r.channel]}`} aria-hidden="true" />
              <span className="sr-only">{translateMaybe(lang, `channel.${r.channel}`, r.channel)}: </span>
              <span className="truncate tabular-nums">{displayRecipient(r.channel, r.recipient)}</span>
            </p>
            <p className="truncate text-xs text-gray-500" title={r.subject}>
              {!ops && sites.length > 1 && r.site_id ? `${siteName(r.site_id)} · ` : ''}
              {r.subject}
            </p>
          </div>
        )
      },
    },
    {
      key: 'status',
      header: t('common.status'),
      cell: (r) => (
        <div className="min-w-[8rem]">
          <StatusBadge status={r.status} />
          {r.last_error && <p className="mt-1 max-w-[14rem] break-words text-xs text-red-700">{deliveryErrorText(r.last_error, lang)}</p>}
        </div>
      ),
    },
    { key: 'attempts', header: t('log.attempts'), numeric: true, cell: (r) => <span className={r.attempts > 1 ? 'font-medium text-amber-700' : ''}>{r.attempts}</span> },
    {
      key: 'actions',
      header: (
        <span className="relative">
          <span className="sr-only">{t('common.actions')}</span>
        </span>
      ),
      cell: (r) =>
        r.status === 'failed' ? (
          <Button size="sm" variant="secondary" loading={retry.isPending && retry.variables === r.id} onClick={() => retry.mutate(r.id)} aria-label={`${t('log.retry')} — ${r.recipient}`}>
            {!(retry.isPending && retry.variables === r.id) && <RotateCw className="h-3.5 w-3.5" aria-hidden="true" />}
            {t('log.retry')}
          </Button>
        ) : null,
    },
  ]

  const label = (f: Filter) => (f ? t(`status.${f}`) : t('common.all'))

  return (
    <>
      <SectionHeader
        title={ops ? t('plat.opsLog') : t('adm.set.log')}
        description={ops ? t('plat.opsLogIntro') : t('adm.log.intro')}
        actions={
          <Button variant="secondary" onClick={() => refetch()} loading={isFetching && !isLoading}>
            {!(isFetching && !isLoading) && <RotateCw className="h-4 w-4" aria-hidden="true" />}
            {t('common.refresh')}
          </Button>
        }
      />

      <div className="no-print mb-4 flex flex-wrap items-center justify-between gap-3">
        <div role="group" aria-label={t('common.status')} className="flex flex-wrap gap-2">
          {FILTERS.map((f) => {
            const on = status === f
            const n = count(f)
            return (
              <button
                key={f || 'all'}
                type="button"
                aria-pressed={on}
                onClick={() => setStatus(f)}
                className={`inline-flex h-8 items-center gap-2 rounded-full px-3 text-sm font-medium transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 ${
                  on ? 'bg-ink-900 text-white' : 'bg-white text-gray-700 ring-1 ring-inset ring-gray-200 hover:bg-gray-50'
                }`}
              >
                {f && <span className={`h-2 w-2 rounded-full ${chipTone[f]}`} aria-hidden="true" />}
                {label(f)}
                <span className={`tabular-nums ${on ? 'text-white/80' : 'text-gray-600'}`}>{n}</span>
              </button>
            )
          })}
        </div>
        {!ops && sites.length > 1 && (
          <div className="flex items-center gap-2">
            <label htmlFor="log-site" className="text-sm text-gray-600">
              {t('common.site')}
            </label>
            <Select id="log-site" value={siteFilter} onChange={(e) => setSiteFilter(e.target.value)} className="w-48">
              <option value="">{t('adm.log.allSites')}</option>
              {sites.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name}
                </option>
              ))}
            </Select>
          </div>
        )}
      </div>

      {!isLoading && !error && all.length === 0 ? (
        <EmptyState icon={History} title={t('adm.log.emptyTitle')} message={ops ? t('plat.opsLogEmpty') : t('adm.log.empty')}>
          <Link to={ops ? '/settings?tab=ops' : '/settings?tab=rules'} className={buttonClass('secondary', 'md')}>
            {ops ? t('plat.toOpsRules') : t('adm.log.toRules')}
          </Link>
        </EmptyState>
      ) : (
        <Table
          columns={columns}
          rows={rows}
          rowKey={(r) => r.id}
          isLoading={isLoading}
          error={error}
          onRetry={() => refetch()}
          empty={t('adm.log.emptyFilter', { status: label(status).toLowerCase() })}
          caption={ops ? t('plat.opsLog') : t('adm.set.log')}
          rowClassName={(r) => (r.status === 'failed' ? 'bg-red-50/30' : '')}
        />
      )}
    </>
  )
}
