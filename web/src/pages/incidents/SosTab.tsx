import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { MapPin } from 'lucide-react'
import apiClient, { apiGet } from '../../api/client'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Checkbox } from '../../components/ui/Form'
import { Toolbar } from '../../components/ui/States'
import { Table, type Column } from '../../components/ui/Table'
import { useErrorToast, useToast } from '../../components/toast/context'
import { useI18n } from '../../i18n/context'
import { formatDateTime, mapsUrl } from '../../lib/time'
import type { Site, SosAlert } from '../../types'

export function SosTab({ site }: { site: Site }) {
  const { t, lang } = useI18n()
  const tz = site.timezone
  const qc = useQueryClient()
  const toast = useToast()
  const onError = useErrorToast()
  const [unackOnly, setUnackOnly] = useState(false)

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['sos', site.id, unackOnly ? 'unack' : 'all'],
    queryFn: () => apiGet<SosAlert[]>('/sos', { site_id: site.id, unack_only: unackOnly || undefined }),
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

  const columns: Column<SosAlert>[] = [
    { key: 'fired', header: t('sos.firedAt'), cell: (r) => <span className="whitespace-nowrap">{formatDateTime(r.fired_at, tz, lang)}</span> },
    { key: 'guard', header: t('common.guard'), cell: (r) => <span className="font-medium">{(r.guard_name || '—')}</span> },
    {
      key: 'gps',
      header: t('sos.gps'),
      cell: (r) =>
        r.gps_lat != null && r.gps_lng != null ? (
          <a href={mapsUrl(r.gps_lat, r.gps_lng)} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-primary-800 underline">
            <MapPin className="h-3.5 w-3.5" aria-hidden="true" />
            {t('common.openMap')}
          </a>
        ) : (
          <span className="text-gray-600">{t('sos.noGps')}</span>
        ),
    },
    {
      key: 'ack',
      header: t('common.status'),
      cell: (r) =>
        r.acknowledged_at ? (
          <div>
            <Badge tone="green">{t('sos.acknowledged')}</Badge>
            <div className="mt-0.5 text-xs text-gray-600">
              {formatDateTime(r.acknowledged_at, tz, lang)}
              {r.acknowledged_by ? ` · ${r.acknowledged_by}` : ''}
            </div>
          </div>
        ) : (
          <Button size="sm" variant="danger" loading={ack.isPending && ack.variables === r.id} onClick={() => ack.mutate(r.id)}>
            {t('sos.acknowledge')}
          </Button>
        ),
    },
  ]

  return (
    <>
      <Toolbar>
        <Checkbox label={t('sos.unackOnly')} checked={unackOnly} onChange={(e) => setUnackOnly(e.target.checked)} />
      </Toolbar>
      <Table
        columns={columns}
        rows={data}
        rowKey={(r) => r.id}
        isLoading={isLoading}
        error={error}
        onRetry={() => refetch()}
        rowClassName={(r) => (r.acknowledged_at ? '' : 'bg-red-50')}
      />
    </>
  )
}
