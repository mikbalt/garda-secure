import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Map as MapIcon, Pencil, Plus, QrCode, Route as RouteIcon, ShieldAlert, Timer } from 'lucide-react'
import { apiGet } from '../../api/client'
import { CardSkeleton } from '../../components/LoadingSkeleton'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { EmptyState, ErrorState, Notice } from '../../components/ui/States'
import { useNewParam } from '../../hooks/useNewParam'
import { useI18n } from '../../i18n/context'
import { fmtDuration } from './format'
import { RouteEditor } from './RouteEditor'
import { QrSheet } from './QrSheet'
import { RegenerateQrDialog } from './RegenerateQr'
import { FloorplansCard } from './plan/FloorplansCard'
import type { PatrolRoute, Site } from '../../types'

const PREVIEW = 5

function RouteCard({ route, canEdit, onEdit, onQr }: { route: PatrolRoute; canEdit: boolean; onEdit: () => void; onQr: () => void }) {
  const { t } = useI18n()
  const cps = [...route.checkpoints].sort((a, b) => a.order_index - b.order_index)
  const slas = cps.map((c) => c.sla_minutes).filter((v): v is number => v != null && v > 0)
  const min = Math.min(...slas)
  const max = Math.max(...slas)
  const total = slas.reduce((n, v) => n + v, 0)
  const legacy = cps.filter((c) => c.qr_signed === false).length
  const placed = cps.filter((c) => c.floorplan_id).length
  const slaText = slas.length === 0 ? t('ops.pat.slaNone') : min === max ? t('ops.pat.slaSingle', { a: min }) : t('ops.pat.slaRange', { a: min, b: max })

  return (
    <li className={`flex flex-col rounded-2xl border bg-white shadow-card ${route.active ? 'border-gray-200/80' : 'border-dashed border-gray-300'}`}>
      <div className="flex items-start gap-3 px-5 pt-5">
        <span
          className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-xl ring-1 ${
            route.active ? 'bg-primary-50 text-primary-600 ring-primary-100' : 'bg-gray-100 text-gray-500 ring-gray-200'
          }`}
        >
          <RouteIcon className="h-[18px] w-[18px]" aria-hidden="true" />
        </span>
        <div className="min-w-0 flex-1">
          <h3 className="truncate text-[15px] font-semibold text-gray-900">{route.name}</h3>
          <p className="text-xs text-gray-500">{t('pat.cpCount', { n: cps.length })}</p>
        </div>
        <Badge tone={route.active ? 'green' : 'gray'} dot>
          {t(route.active ? 'common.active' : 'common.inactive')}
        </Badge>
      </div>

      <div className="mx-5 mt-3 flex items-start gap-2 rounded-lg bg-gray-50 px-3 py-2 text-xs">
        <Timer className="mt-px h-3.5 w-3.5 shrink-0 text-gray-500" aria-hidden="true" />
        <div>
          <p className="font-medium text-gray-700">{slaText}</p>
          {total > 0 && <p className="text-gray-500">{t('ops.pat.roundTime', { d: fmtDuration(total * 60_000, t) })}</p>}
        </div>
      </div>

      {(legacy > 0 || placed > 0) && (
        <div className="mx-5 mt-2 flex flex-wrap gap-x-3 gap-y-1 text-xs">
          {legacy > 0 && (
            <span className="inline-flex items-center gap-1 font-medium text-amber-800">
              <ShieldAlert className="h-3.5 w-3.5" aria-hidden="true" />
              {t('p2.qr.legacyCount', { n: legacy })}
            </span>
          )}
          {placed > 0 && (
            <span className="inline-flex items-center gap-1 text-gray-600">
              <MapIcon className="h-3.5 w-3.5 text-gray-500" aria-hidden="true" />
              {t('p2.place.count', { a: placed, n: cps.length })}
            </span>
          )}
        </div>
      )}

      <ol className="mx-5 mb-4 mt-3 flex-1 space-y-1.5" aria-label={t('pat.checkpoints')}>
        {cps.slice(0, PREVIEW).map((c, i) => (
          <li key={c.id} className="flex items-center gap-2.5 text-sm">
            <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-gray-100 text-[11px] font-semibold tabular-nums text-gray-600">{i + 1}</span>
            <span className="min-w-0 flex-1 truncate text-gray-800">{c.name}</span>
            <span className="shrink-0 text-xs tabular-nums text-gray-500">{c.sla_minutes ? t('ops.dur.m', { m: c.sla_minutes }) : '—'}</span>
          </li>
        ))}
        {cps.length > PREVIEW && <li className="pl-7 text-xs text-gray-500">{t('ops.pat.moreCps', { n: cps.length - PREVIEW })}</li>}
      </ol>

      <div className="flex flex-wrap items-center gap-2 border-t border-gray-100 px-5 py-3">
        <Button size="sm" variant="secondary" onClick={onQr} aria-label={`${t('pat.printQr')} — ${route.name}`}>
          <QrCode className="h-3.5 w-3.5" aria-hidden="true" />
          {t('pat.printQr')}
        </Button>
        {canEdit && (
          <Button size="sm" variant="ghost" onClick={onEdit} aria-label={`${t('common.edit')} — ${route.name}`} className="ml-auto">
            <Pencil className="h-3.5 w-3.5" aria-hidden="true" />
            {t('common.edit')}
          </Button>
        )}
      </div>
    </li>
  )
}

export function RoutesTab({ site, canEdit }: { site: Site; canEdit: boolean }) {
  const { t } = useI18n()
  const [editing, setEditing] = useState<PatrolRoute | 'new' | null>(null)
  const [qrFor, setQrFor] = useState<PatrolRoute | null>(null)
  const [regenFor, setRegenFor] = useState<PatrolRoute | null>(null)

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['patrol-routes', site.id],
    queryFn: () => apiGet<PatrolRoute[]>('/patrol/routes', { site_id: site.id }),
  })

  // Setup guide / header button link here with ?new=1.
  useNewParam(() => canEdit && setEditing('new'))

  const routes = [...(data ?? [])].sort((a, b) => Number(b.active) - Number(a.active) || a.name.localeCompare(b.name))

  return (
    <>
      {!error && !isLoading && <FloorplansCard site={site} routes={data ?? []} canEdit={canEdit} />}
      {error ? (
        <ErrorState error={error} onRetry={() => refetch()} />
      ) : isLoading ? (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {Array.from({ length: 3 }).map((_, i) => (
            <CardSkeleton key={i} />
          ))}
        </div>
      ) : routes.length === 0 ? (
        <EmptyState icon={RouteIcon} title={t('ops.pat.routesEmptyTitle')} message={canEdit ? t('ops.pat.routesEmptyBody') : t('ops.pat.routesEmptyRo')}>
          {canEdit && (
            <Button onClick={() => setEditing('new')}>
              <Plus className="h-4 w-4" aria-hidden="true" />
              {t('ops.pat.createFirst')}
            </Button>
          )}
        </EmptyState>
      ) : (
        <>
          <Notice>{t('ops.pat.routesHint')}</Notice>
          <ul className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
            {routes.map((r) => (
              <RouteCard key={r.id} route={r} canEdit={canEdit} onEdit={() => setEditing(r)} onQr={() => setQrFor(r)} />
            ))}
          </ul>
        </>
      )}
      {editing && (
        <RouteEditor
          site={site}
          route={editing === 'new' ? null : editing}
          onClose={() => setEditing(null)}
          onRegenerate={(r) => {
            setEditing(null)
            setRegenFor(r)
          }}
        />
      )}
      {qrFor && (
        <QrSheet
          route={qrFor}
          onClose={() => setQrFor(null)}
          onRegenerate={
            canEdit
              ? () => {
                  setRegenFor(qrFor)
                  setQrFor(null)
                }
              : undefined
          }
        />
      )}
      {regenFor && (
        <RegenerateQrDialog
          route={regenFor}
          onClose={() => setRegenFor(null)}
          onDone={(r) => {
            setRegenFor(null)
            // Reprint right away: the old codes no longer match.
            setQrFor(r)
          }}
        />
      )}
    </>
  )
}
