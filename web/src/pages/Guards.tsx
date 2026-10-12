import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { CheckCircle2, KeyRound, Pencil, Search, ShieldCheck, TriangleAlert, UserPlus, Users, X } from 'lucide-react'
import { apiGet } from '../api/client'
import { RequireSite } from '../components/RequireSite'
import { TableSkeleton } from '../components/LoadingSkeleton'
import { Badge, StatusBadge } from '../components/ui/Badge'
import { Button } from '../components/ui/Button'
import { Checkbox, Input } from '../components/ui/Form'
import { EmptyState, ErrorState, Notice, PageHeader } from '../components/ui/States'
import { useSite } from '../context/site'
import { useAuth } from '../hooks/useAuth'
import { useNewParam } from '../hooks/useNewParam'
import { useI18n } from '../i18n/context'
import { GuardModal } from './guards/GuardModal'
import { PinModal } from './guards/PinModal'
import { FilterChips, PersonCell, type ChipItem } from './guards/PeopleKit'
import { matches } from './guards/peopleUtil'
import type { Guard, Site } from '../types'

type Filter = 'all' | 'active' | 'inactive' | 'nopin'

function PinStatus({ guard, canEdit, onSet }: { guard: Guard; canEdit: boolean; onSet: () => void }) {
  const { t } = useI18n()
  if (guard.has_pin) {
    return (
      <Badge tone="green">
        <CheckCircle2 className="h-3 w-3" aria-hidden="true" />
        {t('ppl.grd.pinActive')}
      </Badge>
    )
  }
  return (
    <span className="inline-flex flex-wrap items-center gap-2">
      <Badge tone={guard.status === 'active' ? 'yellow' : 'gray'}>
        <TriangleAlert className="h-3 w-3" aria-hidden="true" />
        {t('ppl.grd.pinMissing')}
      </Badge>
      {canEdit && guard.status === 'active' && (
        <Button size="sm" variant="soft" className="!h-7 !px-2.5" onClick={onSet} aria-label={`${t('grd.setPin')} — ${guard.name}`}>
          <KeyRound className="h-3.5 w-3.5" aria-hidden="true" />
          {t('grd.setPin')}
        </Button>
      )}
    </span>
  )
}

function RoleBadge({ guard }: { guard: Guard }) {
  const { t } = useI18n()
  if (guard.role !== 'supervisor') return null
  return (
    <Badge tone="purple">
      <ShieldCheck className="h-3 w-3" aria-hidden="true" />
      {t('role.supervisor')}
    </Badge>
  )
}

function GuardsContent({ site }: { site: Site }) {
  const { t } = useI18n()
  const { can } = useAuth()
  const { sites } = useSite()
  const canEdit = can('editGuards')
  const [allSites, setAllSites] = useState(false)
  const [query, setQuery] = useState('')
  const [filter, setFilter] = useState<Filter>('all')
  const [editing, setEditing] = useState<Guard | 'new' | null>(null)
  const [pinFor, setPinFor] = useState<{ guard: Guard; justCreated: boolean } | null>(null)

  useNewParam(() => setEditing('new'), canEdit)

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['guards', allSites ? 'all' : site.id],
    queryFn: () => apiGet<Guard[]>('/guards', allSites ? {} : { site_id: site.id }),
  })

  const siteName = (id?: string) => (id ? sites.find((s) => s.id === id)?.name ?? '—' : t('common.unassigned'))

  const searched = useMemo(
    () =>
      [...(data ?? [])]
        .filter((g) => matches([g.name, g.employee_id], query))
        .sort((a, b) => (a.status === b.status ? a.name.localeCompare(b.name) : a.status === 'active' ? -1 : 1)),
    [data, query]
  )
  const missingPin = (g: Guard) => g.status === 'active' && !g.has_pin
  const counts = {
    all: searched.length,
    active: searched.filter((g) => g.status === 'active').length,
    inactive: searched.filter((g) => g.status === 'inactive').length,
    nopin: searched.filter(missingPin).length,
  }
  const rows = searched.filter((g) =>
    filter === 'active' ? g.status === 'active' : filter === 'inactive' ? g.status === 'inactive' : filter === 'nopin' ? missingPin(g) : true
  )
  const totalNoPin = (data ?? []).filter(missingPin).length

  const chips: ChipItem<Filter>[] = [
    { key: 'all', label: t('ppl.grd.f.all'), count: counts.all },
    { key: 'active', label: t('ppl.grd.f.active'), count: counts.active, dot: 'bg-emerald-500' },
    { key: 'inactive', label: t('ppl.grd.f.inactive'), count: counts.inactive, dot: 'bg-gray-400' },
    { key: 'nopin', label: t('ppl.grd.f.noPin'), count: counts.nopin, dot: 'bg-amber-500' },
  ]

  const openPin = (g: Guard) => setPinFor({ guard: g, justCreated: false })
  const clearFilters = () => {
    setQuery('')
    setFilter('all')
  }

  const actions = (g: Guard) =>
    canEdit ? (
      <span className="flex items-center justify-end gap-1">
        <Button size="sm" variant="ghost" onClick={() => setEditing(g)} aria-label={`${t('common.edit')} — ${g.name}`}>
          <Pencil className="h-3.5 w-3.5" aria-hidden="true" />
          <span className="hidden lg:inline">{t('common.edit')}</span>
        </Button>
        {g.has_pin && (
          <Button size="sm" variant="ghost" onClick={() => openPin(g)} aria-label={`${t('ppl.grd.changePin')} — ${g.name}`} title={t('ppl.grd.changePin')}>
            <KeyRound className="h-3.5 w-3.5" aria-hidden="true" />
          </Button>
        )}
      </span>
    ) : null

  const header = (
    <PageHeader
      icon={Users}
      title={t('grd.title')}
      subtitle={t('ppl.grd.subtitle')}
      actions={
        canEdit ? (
          <Button onClick={() => setEditing('new')}>
            <UserPlus className="h-4 w-4" aria-hidden="true" />
            {t('ppl.grd.add')}
          </Button>
        ) : undefined
      }
    />
  )

  let body
  if (isLoading) {
    body = <TableSkeleton />
  } else if (error) {
    body = <ErrorState error={error} onRetry={() => refetch()} />
  } else if ((data ?? []).length === 0) {
    body = (
      <EmptyState
        icon={Users}
        title={t('ppl.grd.emptyTitle')}
        message={canEdit ? t('ppl.grd.emptyBody', { site: allSites ? t('ppl.grd.allLocations') : site.name }) : t('ppl.grd.emptyBodyRo', { site: site.name })}
      >
        {canEdit && (
          <Button onClick={() => setEditing('new')}>
            <UserPlus className="h-4 w-4" aria-hidden="true" />
            {t('ppl.grd.add')}
          </Button>
        )}
      </EmptyState>
    )
  } else if (rows.length === 0) {
    body = (
      <EmptyState icon={Search} title={t('ppl.grd.noMatchTitle')} message={t('ppl.grd.noMatchBody')}>
        <Button variant="secondary" onClick={clearFilters}>
          {t('ppl.grd.clearFilters')}
        </Button>
      </EmptyState>
    )
  } else {
    body = (
      <>
        {/* Desktop / tablet: table */}
        <div tabIndex={0} className="scroll-thin relative hidden overflow-x-auto rounded-2xl border border-gray-200/80 bg-white shadow-card focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 md:block">
          <table className="min-w-full text-sm">
            <caption className="sr-only">{t('grd.title')}</caption>
            <thead className="border-b border-gray-200 bg-gray-50/80">
              <tr className="text-left text-[11px] font-semibold uppercase tracking-wider text-gray-500">
                <th scope="col" className="px-5 py-2.5">{t('common.guard')}</th>
                {allSites && <th scope="col" className="px-4 py-2.5">{t('common.site')}</th>}
                <th scope="col" className="px-4 py-2.5">{t('ppl.grd.tabletPin')}</th>
                <th scope="col" className="px-4 py-2.5">{t('common.status')}</th>
                {canEdit && (
                  <th scope="col" className="px-4 py-2.5">
                    <span className="sr-only">{t('common.actions')}</span>
                  </th>
                )}
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {rows.map((g) => (
                <tr key={g.id} className="transition-colors hover:bg-gray-50/80">
                  <td className="px-5 py-3">
                    <PersonCell name={g.name} sub={g.employee_id ?? t('ppl.grd.noEmpId')} muted={g.status === 'inactive'} extra={<RoleBadge guard={g} />} />
                  </td>
                  {allSites && <td className="whitespace-nowrap px-4 py-3 text-gray-700">{siteName(g.site_id)}</td>}
                  <td className="px-4 py-3">
                    <PinStatus guard={g} canEdit={canEdit} onSet={() => openPin(g)} />
                  </td>
                  <td className="px-4 py-3">
                    <StatusBadge status={g.status} />
                  </td>
                  {canEdit && <td className="px-4 py-3 text-right">{actions(g)}</td>}
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        {/* Phone: cards */}
        <ul className="space-y-3 md:hidden" aria-label={t('grd.title')}>
          {rows.map((g) => (
            <li key={g.id} className="rounded-2xl border border-gray-200/80 bg-white p-4 shadow-card">
              <div className="flex items-start justify-between gap-3">
                <PersonCell name={g.name} sub={[g.employee_id, allSites ? siteName(g.site_id) : null].filter(Boolean).join(' · ') || t('ppl.grd.noEmpId')} muted={g.status === 'inactive'} />
                {actions(g)}
              </div>
              <div className="mt-3 flex flex-wrap items-center gap-2">
                <RoleBadge guard={g} />
                <StatusBadge status={g.status} />
                <PinStatus guard={g} canEdit={canEdit} onSet={() => openPin(g)} />
              </div>
            </li>
          ))}
        </ul>
        <p className="mt-3 text-xs text-gray-500">{t('ppl.grd.showing', { n: rows.length, total: (data ?? []).length })}</p>
      </>
    )
  }

  return (
    <>
      {header}
      {!canEdit && <Notice>{t('ppl.grd.readOnly')}</Notice>}
      {canEdit && totalNoPin > 0 && filter !== 'nopin' && (
        <div className="mb-4 flex flex-wrap items-center gap-3 rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900">
          <KeyRound className="h-4 w-4 shrink-0 text-amber-600" aria-hidden="true" />
          <p className="min-w-0 flex-1">{t('ppl.grd.noPinBanner', { n: totalNoPin })}</p>
          <Button size="sm" variant="secondary" onClick={() => setFilter('nopin')}>
            {t('ppl.grd.noPinShow')}
          </Button>
        </div>
      )}

      {(data ?? []).length > 0 && (
        <div className="no-print mb-4 flex flex-col gap-3 rounded-2xl border border-gray-200/80 bg-white px-4 py-3 shadow-card lg:flex-row lg:items-center">
          <div className="relative lg:w-72">
            <label htmlFor="guard-search" className="sr-only">
              {t('ppl.grd.searchLabel')}
            </label>
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-500" aria-hidden="true" />
            <Input
              id="guard-search"
              type="search"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={t('ppl.grd.searchPlaceholder')}
              className="pl-9 pr-9"
              autoComplete="off"
            />
            {query && (
              <button
                type="button"
                onClick={() => setQuery('')}
                aria-label={t('ppl.grd.clearSearch')}
                className="absolute right-2 top-1/2 -translate-y-1/2 rounded p-1 text-gray-500 hover:text-gray-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
              >
                <X className="h-4 w-4" aria-hidden="true" />
              </button>
            )}
          </div>
          <FilterChips label={t('ppl.grd.filterLabel')} items={chips} active={filter} onChange={setFilter} />
          {sites.length > 1 && (
            <Checkbox className="lg:ml-auto" label={t('grd.showAll')} checked={allSites} onChange={(e) => setAllSites(e.target.checked)} />
          )}
        </div>
      )}

      {body}

      {editing && (
        <GuardModal
          guard={editing === 'new' ? null : editing}
          defaultSiteId={site.id}
          onClose={() => setEditing(null)}
          onSaved={(g, created) => {
            if (created && g.id && !g.has_pin) setPinFor({ guard: g, justCreated: true })
          }}
        />
      )}
      {pinFor && <PinModal guard={pinFor.guard} justCreated={pinFor.justCreated} onClose={() => setPinFor(null)} />}
    </>
  )
}

export default function Guards() {
  return <RequireSite>{(site) => <GuardsContent site={site} />}</RequireSite>
}
