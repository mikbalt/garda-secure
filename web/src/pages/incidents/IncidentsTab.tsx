import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Camera, ChevronRight, Clock4, MapPin, ShieldAlert, SlidersHorizontal, UserRound, X } from 'lucide-react'
import { apiGet } from '../../api/client'
import { TableSkeleton } from '../../components/LoadingSkeleton'
import { SeverityBadge, StatusBadge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Select } from '../../components/ui/Form'
import { EmptyState, ErrorState, Notice, Toolbar } from '../../components/ui/States'
import { incidentTypeLabel, useI18n } from '../../i18n/context'
import { addDays, daysBetween, formatDay, formatTime, localDateOf, todayIn } from '../../lib/time'
import { ChipGroup, DateRangeFilter, Meta, Segmented } from '../patrol/kit'
import { IncidentDrawer } from './IncidentDrawer'
import { KNOWN_TYPES, SEVERITIES, WORKFLOW, sevBar } from './constants'
import { defaultIncidentFilter, type IncidentFilterValue } from './incidentFilter'
import type { Incident, Site } from '../../types'

const MAX_DAYS = 366

function IncidentRow({ inc, tz, onOpen }: { inc: Incident; tz: string; onOpen: () => void }) {
  const { t, lang } = useI18n()
  const photos = inc.photo_object_keys.length
  return (
    <li>
      <button
        type="button"
        onClick={onOpen}
        className="group relative flex w-full items-start gap-4 px-5 py-3.5 pl-6 text-left transition-colors hover:bg-gray-50/80 focus:outline-none focus-visible:bg-gray-50 focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary-500"
      >
        <span className={`absolute inset-y-2.5 left-0 w-1 rounded-r-full ${sevBar[inc.severity] ?? sevBar.low}`} aria-hidden="true" />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <p className="text-sm font-semibold text-gray-900">{incidentTypeLabel(lang, inc.type)}</p>
            <SeverityBadge severity={inc.severity} />
          </div>
          {inc.description && <p className="mt-0.5 line-clamp-1 text-sm text-gray-600">{inc.description}</p>}
          <div className="mt-1.5 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-gray-500">
            <Meta icon={Clock4}>{formatTime(inc.occurred_at, tz)}</Meta>
            {inc.location && <Meta icon={MapPin}>{inc.location}</Meta>}
            {inc.guard_name && <Meta icon={UserRound}>{inc.guard_name}</Meta>}
            {photos > 0 && <Meta icon={Camera}>{t('ops.inc.photoCount', { n: photos })}</Meta>}
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-2 self-center">
          <StatusBadge status={inc.status} />
          <ChevronRight className="h-4 w-4 text-gray-300 transition-colors group-hover:text-gray-500" aria-hidden="true" />
        </div>
      </button>
    </li>
  )
}

export function IncidentsTab({ site }: { site: Site }) {
  const { t, lang } = useI18n()
  const tz = site.timezone
  const [filter, setFilter] = useState<IncidentFilterValue>(() => defaultIncidentFilter(tz))
  const [openId, setOpenId] = useState<string | null>(null)
  const [showFilters, setShowFilters] = useState(false)
  const valid = !!filter.from && !!filter.to && filter.from <= filter.to && daysBetween(filter.from, filter.to) <= MAX_DAYS
  const set = (patch: Partial<IncidentFilterValue>) => setFilter((f) => ({ ...f, ...patch }))

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['incidents', site.id, filter],
    queryFn: () => apiGet<Incident[]>('/incidents', { site_id: site.id, ...filter }),
    enabled: valid,
  })

  const list = useMemo(() => data ?? [], [data])
  const types = useMemo(() => Array.from(new Set([...KNOWN_TYPES, ...list.map((i) => i.type), ...(filter.type ? [filter.type] : [])])), [list, filter.type])
  const groups = useMemo(() => {
    const m = new Map<string, Incident[]>()
    for (const i of list) {
      const d = localDateOf(i.occurred_at, tz)
      m.set(d, [...(m.get(d) ?? []), i])
    }
    return [...m.entries()]
  }, [list, tz])
  const openCount = list.filter((i) => i.status !== 'resolved').length
  const activeCount = [filter.type, filter.severity, filter.status].filter(Boolean).length
  const filtered = activeCount > 0
  const today = todayIn(tz)
  const dayLabel = (d: string) => (d === today ? t('ops.day.today') : d === addDays(today, -1) ? t('ops.day.yesterday') : formatDay(d, lang))

  return (
    <>
      <Toolbar>
        <DateRangeFilter from={filter.from} to={filter.to} tz={tz} onChange={(r) => set(r)} />
        <Button
          variant="secondary"
          size="sm"
          className="sm:hidden"
          aria-expanded={showFilters}
          aria-controls="inc-more-filters"
          onClick={() => setShowFilters((v) => !v)}
        >
          <SlidersHorizontal className="h-3.5 w-3.5" aria-hidden="true" />
          {t('ops.inc.moreFilters')}
          {activeCount > 0 && <span className="rounded-full bg-primary-600 px-1.5 text-[11px] font-semibold text-white">{activeCount}</span>}
        </Button>
        <div id="inc-more-filters" className={`${showFilters ? 'flex' : 'hidden'} w-full flex-col gap-3 sm:contents`}>
          <div>
            <label htmlFor="inc-cat" className="mb-1.5 block text-sm font-medium text-gray-700">
              {t('inc.category')}
            </label>
            <Select id="inc-cat" value={filter.type} onChange={(e) => set({ type: e.target.value })} className="!h-9 !w-48 !py-1">
              <option value="">{t('ops.inc.allCategories')}</option>
              {types.map((x) => (
                <option key={x} value={x}>
                  {incidentTypeLabel(lang, x)}
                </option>
              ))}
            </Select>
          </div>
          <Segmented
            label={t('common.status')}
            value={filter.status || 'all'}
            onChange={(v) => set({ status: v === 'all' ? '' : v })}
            options={[{ value: 'all', label: t('common.all') }, ...WORKFLOW.map((s) => ({ value: s, label: t(`status.${s}`) }))]}
          />
          <ChipGroup
            label={t('severity.label')}
            value={filter.severity || 'all'}
            onChange={(v) => set({ severity: v === 'all' ? '' : v })}
            options={[
              { value: 'all', label: t('common.all') },
              ...SEVERITIES.map((s) => ({ value: s, label: t(`severity.${s}`), tone: ({ low: 'gray', medium: 'yellow', high: 'orange', critical: 'red' } as const)[s] })),
            ]}
          />
          {filtered && (
            <Button variant="ghost" size="sm" className="mb-0.5 self-start sm:ml-auto sm:self-auto" onClick={() => set({ type: '', severity: '', status: '' })}>
              <X className="h-3.5 w-3.5" aria-hidden="true" />
              {t('ops.inc.clearFilters')}
            </Button>
          )}
        </div>
      </Toolbar>

      {!valid ? (
        <Notice tone="warn">{t('ops.range.invalid', { n: MAX_DAYS })}</Notice>
      ) : isLoading ? (
        <TableSkeleton cols={4} />
      ) : error ? (
        <ErrorState error={error} onRetry={() => refetch()} />
      ) : list.length === 0 ? (
        <EmptyState icon={ShieldAlert} title={filtered ? t('ops.filterEmpty') : t('ops.inc.emptyTitle')} message={t('ops.inc.emptyBody')}>
          {filtered && (
            <Button variant="secondary" onClick={() => set({ type: '', severity: '', status: '' })}>
              {t('ops.inc.clearFilters')}
            </Button>
          )}
        </EmptyState>
      ) : (
        <>
          <p className="mb-3 text-sm text-gray-600" aria-live="polite">
            {t('ops.inc.summary', { n: list.length, o: openCount })}
          </p>
          <div className="overflow-hidden rounded-2xl border border-gray-200/80 bg-white shadow-card">
            {groups.map(([day, items]) => (
              <section key={day} aria-label={dayLabel(day)} className="border-t border-gray-100 first:border-t-0">
                <h3 className="border-b border-gray-100 bg-gray-50/80 px-5 py-2 text-xs font-semibold text-gray-500">
                  {dayLabel(day)} <span className="font-normal text-gray-400">· {items.length}</span>
                </h3>
                <ul className="divide-y divide-gray-100">
                  {items.map((i) => (
                    <IncidentRow key={i.id} inc={i} tz={tz} onOpen={() => setOpenId(i.id)} />
                  ))}
                </ul>
              </section>
            ))}
          </div>
        </>
      )}
      {openId && <IncidentDrawer site={site} id={openId} onClose={() => setOpenId(null)} />}
    </>
  )
}
