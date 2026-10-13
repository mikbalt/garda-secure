import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { CheckCircle2, CircleDot, ShieldAlert, ShieldCheck, Siren } from 'lucide-react'
import { apiGetPage } from '../../api/paging'
import { AmendedBadge } from '../../components/AmendedBadge'
import { SeverityBadge, StatusBadge } from '../../components/ui/Badge'
import { Notice } from '../../components/ui/States'
import { Field, Select } from '../../components/ui/Form'
import { Table, type Column } from '../../components/ui/Table'
import { incidentTypeLabel, useI18n } from '../../i18n/context'
import { addDays, daysBetween, formatDateTime, formatDay, todayIn } from '../../lib/time'
import { RangePicker, ReportEmpty, Stat, StatRow, type Range } from './ReportKit'
import { ReportShell } from './ReportShell'
import type { Incident, Site } from '../../types'

const MAX_DAYS = 366
const SEVERITIES = ['low', 'medium', 'high', 'critical'] as const
const STATUSES = ['open', 'in_progress', 'resolved'] as const

/** JSON rows shown on screen (the API maximum); downloads always contain every row. */
const SCREEN_LIMIT = 1000

/** Incident log (FR-REP-03): effective (amended) values, Excel / CSV / PDF download. */
export function IncidentReport({ site, title }: { site: Site; title: string }) {
  const { t, lang } = useI18n()
  const tz = site.timezone
  const today = todayIn(tz)
  const [range, setRange] = useState<Range>({ from: addDays(today, -29), to: today })
  const [severity, setSeverity] = useState('')
  const [status, setStatus] = useState('')
  const [type, setType] = useState('')
  const { from, to } = range
  const valid = !!from && !!to && from <= to && daysBetween(from, to) <= MAX_DAYS
  const params = { site_id: site.id, from, to, type, severity, status }

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['report-incidents', site.id, params],
    queryFn: () => apiGetPage<Incident>('/reports/incidents', { ...params, limit: SCREEN_LIMIT }),
    enabled: valid,
  })
  const rows = useMemo(() => data?.rows ?? [], [data])

  // Categories seen so far (kept while a category filter narrows the list).
  const [seenTypes, setSeenTypes] = useState<string[]>([])
  const types = useMemo(() => {
    const all = new Set([...seenTypes, ...rows.map((i) => i.type)])
    if (type) all.add(type)
    return Array.from(all).sort()
  }, [rows, seenTypes, type])
  if (types.length !== seenTypes.length) setSeenTypes(types)

  const high = rows.filter((i) => i.severity === 'high' || i.severity === 'critical').length
  const unresolved = rows.filter((i) => i.status !== 'resolved').length
  const resolved = rows.length - unresolved
  const filtered = !!(severity || status || type)

  const columns: Column<Incident>[] = [
    { key: 'occurred', header: t('inc.occurred'), cell: (r) => <span className="whitespace-nowrap tabular-nums">{formatDateTime(r.occurred_at, tz, lang)}</span> },
    {
      key: 'type',
      header: t('inc.category'),
      cell: (r) => (
        <div className="min-w-[8rem]">
          <p className="flex flex-wrap items-center gap-1.5 font-medium text-gray-900">
            {incidentTypeLabel(lang, r.type)}
            {r.amended && <AmendedBadge incident={r} />}
          </p>
          {r.location && <p className="text-xs text-gray-500">{r.location}</p>}
        </div>
      ),
    },
    { key: 'severity', header: t('severity.label'), cell: (r) => <SeverityBadge severity={r.severity} /> },
    { key: 'status', header: t('common.status'), cell: (r) => <StatusBadge status={r.status} /> },
    { key: 'guard', header: t('adm.rep.reportedBy'), cell: (r) => <span className="whitespace-nowrap">{r.guard_name || '—'}</span> },
    { key: 'desc', header: t('common.description'), cell: (r) => <p className="min-w-[16rem] max-w-md whitespace-pre-wrap text-gray-700">{r.description ?? '—'}</p> },
    { key: 'photos', header: t('inc.photos'), numeric: true, cell: (r) => r.photo_object_keys.length || <span className="text-gray-500">0</span> },
  ]

  return (
    <ReportShell
      site={site}
      title={title}
      period={from && to ? `${formatDay(from, lang, false)} – ${formatDay(to, lang, false)}` : ''}
      csv={{ path: '/reports/incidents', params, filename: `incidents-${from}-${to}`, disabled: !valid, formats: ['xlsx', 'csv', 'pdf'] }}
      invalid={valid ? undefined : t('adm.rep.badRange', { n: MAX_DAYS })}
      filters={
        <>
          <RangePicker value={range} onChange={setRange} today={today} />
          <Field label={t('inc.category')}>
            <Select value={type} onChange={(e) => setType(e.target.value)} className="w-40">
              <option value="">{t('common.all')}</option>
              {types.map((x) => (
                <option key={x} value={x}>
                  {incidentTypeLabel(lang, x)}
                </option>
              ))}
            </Select>
          </Field>
          <Field label={t('severity.label')}>
            <Select value={severity} onChange={(e) => setSeverity(e.target.value)} className="w-32">
              <option value="">{t('common.all')}</option>
              {SEVERITIES.map((s) => (
                <option key={s} value={s}>
                  {t(`severity.${s}`)}
                </option>
              ))}
            </Select>
          </Field>
          <Field label={t('common.status')}>
            <Select value={status} onChange={(e) => setStatus(e.target.value)} className="w-36">
              <option value="">{t('common.all')}</option>
              {STATUSES.map((s) => (
                <option key={s} value={s}>
                  {t(`status.${s}`)}
                </option>
              ))}
            </Select>
          </Field>
        </>
      }
      stats={
        rows.length > 0 || isLoading ? (
          <StatRow loading={isLoading}>
            <Stat icon={ShieldAlert} tone="blue" label={t('adm.rep.stat.incidents')} value={data?.total ?? rows.length} sub={t('adm.rep.stat.inPeriod')} />
            <Stat icon={Siren} tone="red" label={t('adm.rep.stat.highCritical')} value={high} sub={t('adm.rep.stat.highSub')} />
            <Stat icon={CircleDot} tone="orange" label={t('adm.rep.stat.unresolved')} value={unresolved} sub={t('adm.rep.stat.unresolvedSub')} />
            <Stat icon={CheckCircle2} tone="green" label={t('status.resolved')} value={resolved} sub={t('adm.rep.stat.resolvedSub')} />
          </StatRow>
        ) : null
      }
    >
      {!isLoading && !error && rows.length === 0 ? (
        <ReportEmpty
          icon={ShieldCheck}
          title={filtered ? t('adm.rep.incidents.emptyFiltered') : t('adm.rep.incidents.emptyTitle')}
          message={t('adm.rep.incidents.empty')}
        />
      ) : (
        <>
          {data?.truncated && <Notice tone="warn">{t('p2.page.reportTruncated', { n: rows.length, total: data.total })}</Notice>}
          <Table columns={columns} rows={rows} rowKey={(r) => r.id} isLoading={isLoading} error={error} onRetry={() => refetch()} caption={title} />
        </>
      )}
    </ReportShell>
  )
}
