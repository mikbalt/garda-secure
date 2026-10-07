import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { apiGet } from '../../api/client'
import { SeverityBadge, StatusBadge } from '../../components/ui/Badge'
import { Table, type Column } from '../../components/ui/Table'
import { useI18n } from '../../i18n/context'
import { daysBetween, formatDateTime, formatDay } from '../../lib/time'
import { IncidentFilters } from '../incidents/IncidentFilters'
import { defaultIncidentFilter, type IncidentFilterValue } from '../incidents/incidentFilter'
import { ReportShell } from './ReportShell'
import type { Incident, Site } from '../../types'

/** Incident log (FR-REP-03) — printable for "Save as PDF". */
export function IncidentReport({ site }: { site: Site }) {
  const { t, lang } = useI18n()
  const tz = site.timezone
  const [filter, setFilter] = useState<IncidentFilterValue>(() => defaultIncidentFilter(tz))
  const valid = !!filter.from && !!filter.to && filter.from <= filter.to && daysBetween(filter.from, filter.to) <= 366

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['report-incidents', site.id, filter],
    queryFn: () => apiGet<Incident[]>('/reports/incidents', { site_id: site.id, ...filter }),
    enabled: valid,
  })
  const knownTypes = useMemo(() => Array.from(new Set((data ?? []).map((i) => i.type))).sort(), [data])

  const columns: Column<Incident>[] = [
    { key: 'occurred', header: t('inc.occurred'), cell: (r) => <span className="whitespace-nowrap">{formatDateTime(r.occurred_at, tz, lang)}</span> },
    { key: 'type', header: t('inc.category'), cell: (r) => <span className="capitalize">{r.type}</span> },
    { key: 'severity', header: t('severity.label'), cell: (r) => <SeverityBadge severity={r.severity} /> },
    { key: 'status', header: t('common.status'), cell: (r) => <StatusBadge status={r.status} /> },
    { key: 'guard', header: t('common.guard'), cell: (r) => r.guard_name || '—' },
    { key: 'location', header: t('common.location'), cell: (r) => r.location ?? '—' },
    { key: 'desc', header: t('common.description'), cell: (r) => <span className="whitespace-pre-wrap">{r.description ?? '—'}</span> },
    { key: 'photos', header: t('inc.photos'), numeric: true, cell: (r) => r.photo_object_keys.length },
  ]

  return (
    <ReportShell
      site={site}
      title={t('rep.incidents')}
      period={filter.from && filter.to ? `${formatDay(filter.from, lang, false)} – ${formatDay(filter.to, lang, false)}` : ''}
      printable
      csv={{ path: '/reports/incidents', params: { site_id: site.id, ...filter }, filename: 'incidents.csv', disabled: !valid }}
      filters={<IncidentFilters value={filter} onChange={setFilter} knownTypes={knownTypes} />}
    >
      {valid ? (
        <Table columns={columns} rows={data} rowKey={(r) => r.id} isLoading={isLoading} error={error} onRetry={() => refetch()} empty={t('inc.empty')} caption={t('rep.incidents')} />
      ) : (
        <p role="alert" className="text-sm text-red-700">
          {t('rep.rangeTooLong', { n: 366 })}
        </p>
      )}
    </ReportShell>
  )
}
