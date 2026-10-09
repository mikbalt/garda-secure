import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Building2, Clock, Globe2, MapPin, Pencil, Plus, Tablet } from 'lucide-react'
import { apiGet } from '../../api/client'
import { TableSkeleton } from '../../components/LoadingSkeleton'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { EmptyState, ErrorState } from '../../components/ui/States'
import { useSite } from '../../context/site'
import { useNewParam } from '../../hooks/useNewParam'
import { useI18n } from '../../i18n/context'
import { formatTime } from '../../lib/time'
import { IconTile, SectionHeader } from './kit'
import { SiteModal } from './SiteModal'
import type { Device, Site, SiteConfig } from '../../types'

const DEFAULTS: Omit<SiteConfig, 'language'> = {
  clock_window_minutes: 15,
  late_grace_minutes: 5,
  early_out_grace_minutes: 5,
  absent_after_minutes: 30,
  idle_lock_minutes: 5,
}

function SiteCard({ site, current, tablets, onEdit, onSelect }: { site: Site; current: boolean; tablets: number; onEdit: () => void; onSelect: () => void }) {
  const { t } = useI18n()
  const cfg = { ...DEFAULTS, ...(site.config_json ?? {}) }
  const rules: { label: string; value: string }[] = [
    { label: t('adm.site.window'), value: t('adm.site.windowVal', { n: cfg.clock_window_minutes }) },
    { label: t('adm.site.late'), value: t('adm.site.minutes', { n: cfg.late_grace_minutes }) },
    { label: t('adm.site.absent'), value: t('adm.site.minutes', { n: cfg.absent_after_minutes }) },
    { label: t('adm.site.lock'), value: t('adm.site.minutes', { n: cfg.idle_lock_minutes }) },
  ]
  return (
    <article className={`flex flex-col rounded-2xl border bg-white shadow-card ${current ? 'border-primary-200 ring-4 ring-primary-50' : 'border-gray-200/80'}`}>
      <div className="flex items-start gap-3.5 p-5 pb-4">
        <IconTile icon={Building2} tone={current ? 'blue' : 'gray'} />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="text-[15px] font-semibold text-gray-900">{site.name}</h3>
            {current && (
              <Badge tone="blue" dot>
                {t('adm.site.current')}
              </Badge>
            )}
          </div>
          {site.client_id && <p className="text-xs font-medium text-gray-500">{site.client_id}</p>}
          <p className="mt-1.5 flex items-start gap-1.5 text-sm text-gray-600">
            <MapPin className="mt-0.5 h-3.5 w-3.5 shrink-0 text-gray-400" aria-hidden="true" />
            <span className="min-w-0">{site.address || <span className="text-gray-400">{t('adm.site.noAddress')}</span>}</span>
          </p>
          <p className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-gray-500">
            <span className="inline-flex items-center gap-1.5">
              <Globe2 className="h-3.5 w-3.5 text-gray-400" aria-hidden="true" />
              {site.timezone}
            </span>
            <span className="inline-flex items-center gap-1.5">
              <Clock className="h-3.5 w-3.5 text-gray-400" aria-hidden="true" />
              {t('adm.site.localNow', { time: formatTime(new Date().toISOString(), site.timezone) })}
            </span>
            <span className="inline-flex items-center gap-1.5">
              <Tablet className="h-3.5 w-3.5 text-gray-400" aria-hidden="true" />
              {t('adm.site.tablets', { n: tablets })}
            </span>
          </p>
        </div>
      </div>
      <dl className="mx-5 grid grid-cols-2 gap-2 sm:grid-cols-4">
        {rules.map((r) => (
          <div key={r.label} className="rounded-lg bg-gray-50 px-3 py-2 ring-1 ring-inset ring-gray-100">
            <dt className="text-[11px] leading-tight text-gray-500">{r.label}</dt>
            <dd className="mt-0.5 text-sm font-semibold tabular-nums text-gray-900">{r.value}</dd>
          </div>
        ))}
      </dl>
      <div className="mt-4 flex flex-wrap items-center justify-end gap-2 border-t border-gray-100 px-5 py-3">
        {!current && (
          <Button variant="ghost" size="sm" onClick={onSelect}>
            {t('adm.site.makeCurrent')}
          </Button>
        )}
        <Button variant="secondary" size="sm" onClick={onEdit} aria-label={`${t('adm.site.edit')} — ${site.name}`}>
          <Pencil className="h-3.5 w-3.5" aria-hidden="true" />
          {t('adm.site.edit')}
        </Button>
      </div>
    </article>
  )
}

export function SitesTab() {
  const { t } = useI18n()
  const { sites, siteId, setSiteId, isLoading, error } = useSite()
  const [editing, setEditing] = useState<Site | 'new' | null>(null)
  useNewParam(() => setEditing('new'))

  const devices = useQuery({ queryKey: ['devices'], queryFn: () => apiGet<Device[]>('/devices') })
  const tabletsAt = (id: string) => (devices.data ?? []).filter((d) => d.site_id === id && d.status === 'active').length

  return (
    <>
      <SectionHeader
        title={t('adm.set.sites')}
        description={t('adm.site.intro')}
        actions={
          sites.length > 0 && (
            <Button onClick={() => setEditing('new')}>
              <Plus className="h-4 w-4" aria-hidden="true" />
              {t('adm.site.new')}
            </Button>
          )
        }
      />
      {isLoading ? (
        <TableSkeleton />
      ) : error ? (
        <ErrorState error={error} />
      ) : sites.length === 0 ? (
        <EmptyState icon={Building2} title={t('adm.site.emptyTitle')} message={t('adm.site.empty')}>
          <Button onClick={() => setEditing('new')}>
            <Plus className="h-4 w-4" aria-hidden="true" />
            {t('adm.site.new')}
          </Button>
        </EmptyState>
      ) : (
        <div className="grid grid-cols-1 gap-4 2xl:grid-cols-2">
          {sites.map((s) => (
            <SiteCard key={s.id} site={s} current={s.id === siteId} tablets={tabletsAt(s.id)} onEdit={() => setEditing(s)} onSelect={() => setSiteId(s.id)} />
          ))}
          <button
            type="button"
            onClick={() => setEditing('new')}
            className="flex min-h-[8rem] flex-col items-center justify-center gap-2 rounded-2xl border border-dashed border-gray-300 bg-white/50 p-5 text-sm font-medium text-gray-600 transition-colors hover:border-primary-300 hover:bg-primary-50/40 hover:text-primary-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
          >
            <Plus className="h-5 w-5" aria-hidden="true" />
            {t('adm.site.addAnother')}
          </button>
        </div>
      )}
      {editing && <SiteModal site={editing === 'new' ? null : editing} onClose={() => setEditing(null)} />}
    </>
  )
}
