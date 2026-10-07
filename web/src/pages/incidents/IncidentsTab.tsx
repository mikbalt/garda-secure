import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Camera } from 'lucide-react'
import { apiGet } from '../../api/client'
import { SeverityBadge, StatusBadge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Toolbar } from '../../components/ui/States'
import { Table, type Column } from '../../components/ui/Table'
import { useI18n } from '../../i18n/context'
import { daysBetween, formatDateTime } from '../../lib/time'
import { IncidentDrawer } from './IncidentDrawer'
import { IncidentFilters } from './IncidentFilters'
import { defaultIncidentFilter, type IncidentFilterValue } from './incidentFilter'
import type { Incident, Site } from '../../types'

export function IncidentsTab({ site }: { site: Site }) {
  const { t, lang } = useI18n()
  const tz = site.timezone
  const [filter, setFilter] = useState<IncidentFilterValue>(() => defaultIncidentFilter(tz))
  const [openId, setOpenId] = useState<string | null>(null)
  const valid = filter.from && filter.to && filter.from <= filter.to && daysBetween(filter.from, filter.to) <= 366

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['incidents', site.id, filter],
    queryFn: () => apiGet<Incident[]>('/incidents', { site_id: site.id, ...filter }),
    enabled: !!valid,
  })

  const knownTypes = useMemo(() => Array.from(new Set((data ?? []).map((i) => i.type))).sort(), [data])

  const columns: Column<Incident>[] = [
    { key: 'occurred', header: t('inc.occurred'), cell: (r) => <span className="whitespace-nowrap">{formatDateTime(r.occurred_at, tz, lang)}</span> },
    { key: 'type', header: t('inc.category'), cell: (r) => <span className="capitalize">{r.type}</span> },
    { key: 'severity', header: t('severity.label'), cell: (r) => <SeverityBadge severity={r.severity} /> },
    { key: 'status', header: t('common.status'), cell: (r) => <StatusBadge status={r.status} /> },
    { key: 'guard', header: t('common.guard'), cell: (r) => r.guard_name || '—' },
    { key: 'location', header: t('common.location'), cell: (r) => r.location ?? '—' },
    {
      key: 'desc',
      header: t('common.description'),
      cell: (r) => <span className="line-clamp-2 max-w-xs text-gray-800">{r.description ?? '—'}</span>,
    },
    {
      key: 'photos',
      header: <span className="sr-only">{t('inc.photos')}</span>,
      cell: (r) =>
        r.photo_object_keys.length ? (
          <span className="inline-flex items-center gap-1 text-xs text-gray-700">
            <Camera className="h-3.5 w-3.5" aria-hidden="true" />
            {r.photo_object_keys.length}
            <span className="sr-only">{t('inc.photos')}</span>
          </span>
        ) : null,
    },
    {
      key: 'view',
      header: <span className="sr-only">{t('common.actions')}</span>,
      cell: (r) => (
        <Button size="sm" variant="secondary" onClick={() => setOpenId(r.id)}>
          {t('common.details')}
        </Button>
      ),
    },
  ]

  return (
    <>
      <Toolbar>
        <IncidentFilters value={filter} onChange={setFilter} knownTypes={knownTypes} />
      </Toolbar>
      {!valid ? (
        <p role="alert" className="text-sm text-red-700">
          {t('rep.rangeTooLong', { n: 366 })}
        </p>
      ) : (
        <Table columns={columns} rows={data} rowKey={(r) => r.id} isLoading={isLoading} error={error} onRetry={() => refetch()} empty={t('inc.empty')} />
      )}
      {openId && <IncidentDrawer site={site} id={openId} onClose={() => setOpenId(null)} />}
    </>
  )
}
