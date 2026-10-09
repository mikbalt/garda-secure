import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CheckCircle2, MapPin, Siren } from 'lucide-react'
import apiClient, { apiGet } from '../../api/client'
import { TableSkeleton } from '../../components/LoadingSkeleton'
import { Button } from '../../components/ui/Button'
import { EmptyState, ErrorState, Notice } from '../../components/ui/States'
import { useErrorToast, useToast } from '../../components/toast/context'
import { useI18n } from '../../i18n/context'
import { formatDateTime, formatTime, mapsUrl } from '../../lib/time'
import { fmtDuration, relTime, useNow } from '../patrol/format'
import { Segmented } from '../patrol/kit'
import type { Site, SosAlert } from '../../types'

type Filter = 'all' | 'unack'

export function SosTab({ site }: { site: Site }) {
  const { t, lang } = useI18n()
  const tz = site.timezone
  const qc = useQueryClient()
  const toast = useToast()
  const onError = useErrorToast()
  const now = useNow(15_000)
  const [filter, setFilter] = useState<Filter>('all')

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['sos', site.id, 'all'],
    queryFn: () => apiGet<SosAlert[]>('/sos', { site_id: site.id }),
    refetchInterval: 10_000,
  })

  const ack = useMutation({
    mutationFn: (id: string) => apiClient.post(`/sos/${id}/acknowledge`),
    onSuccess: () => {
      toast.success(t('sos.ackDone'))
      qc.invalidateQueries({ queryKey: ['sos'] })
      qc.invalidateQueries({ queryKey: ['dashboard', site.id] })
    },
    onError,
  })

  if (isLoading) return <TableSkeleton cols={3} />
  if (error) return <ErrorState error={error} onRetry={() => refetch()} />

  const all = data ?? []
  const unackCount = all.filter((s) => !s.acknowledged_at).length
  const rows = filter === 'unack' ? all.filter((s) => !s.acknowledged_at) : all

  if (all.length === 0) return <EmptyState icon={Siren} title={t('ops.sos.emptyTitle')} message={t('ops.sos.emptyBody')} />

  return (
    <>
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <Segmented<Filter>
          label={t('common.status')}
          hideLabel
          value={filter}
          onChange={setFilter}
          options={[
            { value: 'all', label: t('common.all'), count: all.length },
            { value: 'unack', label: t('ops.sos.filterUnack'), count: unackCount, tone: unackCount ? 'red' : undefined },
          ]}
        />
      </div>
      {unackCount > 0 && <Notice tone="warn">{t('ops.sos.unackHint')}</Notice>}
      {rows.length === 0 ? (
        <EmptyState icon={CheckCircle2} message={t('ops.sos.allHandled')} />
      ) : (
        <ul className="divide-y divide-gray-100 overflow-hidden rounded-2xl border border-gray-200/80 bg-white shadow-card">
          {rows.map((s) => {
            const open = !s.acknowledged_at
            return (
              <li key={s.id} className={`flex flex-wrap items-center gap-x-4 gap-y-3 px-5 py-4 ${open ? 'bg-red-50/60' : ''}`}>
                <span
                  className={`relative flex h-10 w-10 shrink-0 items-center justify-center rounded-xl ring-1 ${
                    open ? 'bg-red-600 text-white ring-red-600' : 'bg-emerald-50 text-emerald-600 ring-emerald-100'
                  }`}
                  aria-hidden="true"
                >
                  {open && <span className="absolute inset-0 animate-ping2 rounded-xl bg-red-500 opacity-40" />}
                  {open ? <Siren className="relative h-5 w-5" /> : <CheckCircle2 className="h-5 w-5" />}
                </span>
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-semibold text-gray-900">
                    {s.guard_name || '—'} <span className="font-normal text-gray-600">{t('ops.sos.pressed')}</span>
                  </p>
                  <p className="mt-0.5 flex flex-wrap items-center gap-x-3 gap-y-0.5 text-xs text-gray-500">
                    <span>
                      {formatDateTime(s.fired_at, tz, lang)} · {relTime(s.fired_at, lang, now)}
                    </span>
                    {s.gps_lat != null && s.gps_lng != null ? (
                      <a
                        href={mapsUrl(s.gps_lat, s.gps_lng)}
                        target="_blank"
                        rel="noreferrer"
                        className="inline-flex items-center gap-1 font-medium text-primary-700 hover:text-primary-800 hover:underline"
                      >
                        <MapPin className="h-3.5 w-3.5" aria-hidden="true" />
                        {t('common.openMap')}
                      </a>
                    ) : (
                      <span>{t('sos.noGps')}</span>
                    )}
                  </p>
                </div>
                {open ? (
                  <div className="flex items-center gap-3">
                    <span className="text-xs font-medium text-red-700">{t('ops.sos.waiting', { d: fmtDuration(now - Date.parse(s.fired_at), t) })}</span>
                    <Button variant="danger" size="sm" loading={ack.isPending && ack.variables === s.id} onClick={() => ack.mutate(s.id)}>
                      {t('sos.acknowledge')}
                    </Button>
                  </div>
                ) : (
                  <div className="text-right text-xs text-gray-500">
                    <p className="font-medium text-gray-700">{t('ops.sos.ackedBy', { who: s.acknowledged_by || '—' })}</p>
                    <p>
                      {formatTime(s.acknowledged_at, tz)} · {t('ops.sos.response', { d: fmtDuration(Date.parse(s.acknowledged_at!) - Date.parse(s.fired_at), t) })}
                    </p>
                  </div>
                )}
              </li>
            )
          })}
        </ul>
      )}
    </>
  )
}
