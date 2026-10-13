import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { DoorOpen, IdCard } from 'lucide-react'
import { apiGet } from '../../api/client'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { MediaPhotoButton } from '../../components/MediaPhoto'
import { EmptyState, Notice, Toolbar } from '../../components/ui/States'
import { Table, type Column } from '../../components/ui/Table'
import { useAuth } from '../../hooks/useAuth'
import { useI18n } from '../../i18n/context'
import { addDays, daysBetween, formatDay, formatTime, localDateOf, todayIn } from '../../lib/time'
import { fmtDuration, presetRange, useNow } from '../patrol/format'
import { Avatar, DateRangeFilter, Segmented } from '../patrol/kit'
import { VisitorDrawer } from './VisitorDrawer'
import type { Site, VisitorLog } from '../../types'

const MAX_DAYS = 92
type Filter = 'all' | 'inside'

function When({ iso, tz }: { iso?: string; tz: string }) {
  const { t, lang } = useI18n()
  if (!iso) return <span className="text-gray-500">—</span>
  const d = localDateOf(iso, tz)
  const today = todayIn(tz)
  const day = d === today ? t('ops.day.today') : d === addDays(today, -1) ? t('ops.day.yesterday') : formatDay(d, lang, false)
  return (
    <div className="whitespace-nowrap">
      <div className="font-medium tabular-nums text-gray-900">{formatTime(iso, tz)}</div>
      <div className="text-xs text-gray-500">{day}</div>
    </div>
  )
}

export function VisitorsTab({ site }: { site: Site }) {
  const { t } = useI18n()
  const tz = site.timezone
  const now = useNow(60_000)
  const [range, setRange] = useState(() => presetRange('7d', tz))
  const [filter, setFilter] = useState<Filter>('all')
  const [detail, setDetail] = useState<VisitorLog | null>(null)
  const { can } = useAuth()
  const { from, to } = range
  const valid = !!from && !!to && from <= to && daysBetween(from, to) <= MAX_DAYS

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['visitors', site.id, from, to],
    queryFn: () => apiGet<VisitorLog[]>('/visitors', { site_id: site.id, from, to }),
    enabled: valid,
  })

  const all = [...(data ?? [])].sort((a, b) => (b.entry_at ?? b.created_at).localeCompare(a.entry_at ?? a.created_at))
  const inside = all.filter((v) => !v.exit_at)
  const rows = filter === 'inside' ? inside : all

  const columns: Column<VisitorLog>[] = [
    {
      key: 'name',
      header: t('vis.name'),
      cell: (r) => (
        <div className="flex min-w-[12rem] items-center gap-3">
          <Avatar name={r.visitor_name} />
          <div className="min-w-0">
            <p className="truncate font-medium text-gray-900">{r.visitor_name}</p>
            <p className="truncate text-xs text-gray-500">{[r.id_type, r.id_number].filter(Boolean).join(' ') || '—'}</p>
          </div>
        </div>
      ),
    },
    {
      key: 'purpose',
      header: t('vis.purpose'),
      cell: (r) => (
        <div className="min-w-[10rem]">
          <p className="text-gray-800">{r.purpose || '—'}</p>
          {r.host_name && <p className="text-xs text-gray-500">{t('ops.vis.meeting', { host: r.host_name })}</p>}
        </div>
      ),
    },
    {
      key: 'plate',
      header: t('vis.plate'),
      cell: (r) =>
        r.plate ? (
          <span className="whitespace-nowrap rounded-md bg-gray-900 px-1.5 py-0.5 font-mono text-xs font-medium tracking-wide text-white">{r.plate}</span>
        ) : (
          <span className="text-gray-500">—</span>
        ),
    },
    { key: 'entry', header: t('vis.entry'), cell: (r) => <When iso={r.entry_at} tz={tz} /> },
    {
      key: 'exit',
      header: t('vis.exit'),
      cell: (r) =>
        r.exit_at ? (
          <When iso={r.exit_at} tz={tz} />
        ) : (
          <Badge tone="blue" dot>
            {t('ops.vis.inside')}
          </Badge>
        ),
    },
    {
      key: 'dur',
      header: t('ops.vis.duration'),
      cell: (r) =>
        r.entry_at ? (
          <span className={`whitespace-nowrap tabular-nums ${r.exit_at ? 'text-gray-700' : 'text-primary-700'}`}>
            {fmtDuration((r.exit_at ? Date.parse(r.exit_at) : now) - Date.parse(r.entry_at), t)}
          </span>
        ) : (
          '—'
        ),
    },
    { key: 'guard', header: t('ops.vis.loggedBy'), cell: (r) => <span className="whitespace-nowrap text-gray-600">{r.guard_name || '—'}</span> },
    {
      key: 'photo',
      header: <span className="relative block"><span className="sr-only">{t('common.photo')}</span></span>,
      cell: (r) => (
        <div className="relative [&_span]:whitespace-nowrap">
          <MediaPhotoButton objectKey={r.photo_object_key} title={`${t('vis.name')}: ${r.visitor_name}`} />
        </div>
      ),
    },
  ]
  if (can('viewVisitorDetail')) {
    columns.push({
      key: 'detail',
      header: <span className="sr-only">{t('common.details')}</span>,
      cell: (r) => (
        <Button size="sm" variant="ghost" onClick={() => setDetail(r)} aria-label={`${t('common.details')} — ${r.visitor_name}`}>
          <IdCard className="h-4 w-4" aria-hidden="true" />
          <span className="sr-only sm:not-sr-only">{t('common.details')}</span>
        </Button>
      ),
    })
  }

  return (
    <>
      <Toolbar>
        <DateRangeFilter from={from} to={to} tz={tz} onChange={setRange} />
        <Segmented<Filter>
          label={t('common.status')}
          value={filter}
          onChange={setFilter}
          options={[
            { value: 'all', label: t('common.all'), count: all.length },
            { value: 'inside', label: t('ops.vis.inside'), count: inside.length, tone: 'blue' },
          ]}
        />
      </Toolbar>
      {!valid ? (
        <Notice tone="warn">{t('ops.range.invalid', { n: MAX_DAYS })}</Notice>
      ) : !isLoading && !error && all.length === 0 ? (
        <EmptyState icon={DoorOpen} title={t('ops.vis.emptyTitle')} message={t('ops.vis.emptyBody')} />
      ) : (
        <Table
          columns={columns}
          rows={rows}
          rowKey={(r) => r.id}
          isLoading={isLoading}
          error={error}
          onRetry={() => refetch()}
          empty={t('ops.filterEmpty')}
          caption={t('inc.tabVisitors')}
        />
      )}
      {detail && <VisitorDrawer visitor={detail} tz={tz} onClose={() => setDetail(null)} />}
    </>
  )
}
