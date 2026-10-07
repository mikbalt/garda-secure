import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { apiGet } from '../../api/client'
import { Badge } from '../../components/ui/Badge'
import { MediaPhotoButton } from '../../components/MediaPhoto'
import { Field, Input } from '../../components/ui/Form'
import { Toolbar } from '../../components/ui/States'
import { Table, type Column } from '../../components/ui/Table'
import { useI18n } from '../../i18n/context'
import { addDays, daysBetween, formatDateTime, todayIn } from '../../lib/time'
import type { Site, VisitorLog } from '../../types'

export function VisitorsTab({ site }: { site: Site }) {
  const { t, lang } = useI18n()
  const tz = site.timezone
  const [from, setFrom] = useState(() => addDays(todayIn(tz), -6))
  const [to, setTo] = useState(() => todayIn(tz))
  const valid = from && to && from <= to && daysBetween(from, to) <= 92

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['visitors', site.id, from, to],
    queryFn: () => apiGet<VisitorLog[]>('/visitors', { site_id: site.id, from, to }),
    enabled: !!valid,
  })

  const columns: Column<VisitorLog>[] = [
    { key: 'entry', header: t('vis.entry'), cell: (r) => <span className="whitespace-nowrap">{formatDateTime(r.entry_at, tz, lang)}</span> },
    {
      key: 'exit',
      header: t('vis.exit'),
      cell: (r) => (r.exit_at ? <span className="whitespace-nowrap">{formatDateTime(r.exit_at, tz, lang)}</span> : <Badge tone="blue">{t('vis.onSite')}</Badge>),
    },
    { key: 'name', header: t('vis.name'), cell: (r) => <span className="font-medium">{r.visitor_name}</span> },
    { key: 'id', header: t('vis.id'), cell: (r) => [r.id_type, r.id_number].filter(Boolean).join(' ') || '—' },
    { key: 'plate', header: t('vis.plate'), cell: (r) => r.plate ?? '—' },
    { key: 'host', header: t('vis.host'), cell: (r) => r.host_name ?? '—' },
    { key: 'purpose', header: t('vis.purpose'), cell: (r) => r.purpose ?? '—' },
    { key: 'guard', header: t('common.guard'), cell: (r) => (r.guard_name || '—') },
    { key: 'photo', header: t('common.photo'), cell: (r) => <MediaPhotoButton objectKey={r.photo_object_key} title={`${t('vis.name')}: ${r.visitor_name}`} /> },
  ]

  return (
    <>
      <Toolbar>
        <Field label={t('common.from')}>
          <Input type="date" value={from} onChange={(e) => setFrom(e.target.value)} />
        </Field>
        <Field label={t('common.to')}>
          <Input type="date" value={to} onChange={(e) => setTo(e.target.value)} />
        </Field>
      </Toolbar>
      {!valid ? (
        <p role="alert" className="text-sm text-red-700">
          {t('rep.rangeTooLong', { n: 92 })}
        </p>
      ) : (
        <Table columns={columns} rows={data} rowKey={(r) => r.id} isLoading={isLoading} error={error} onRetry={() => refetch()} empty={t('vis.empty')} />
      )}
    </>
  )
}
