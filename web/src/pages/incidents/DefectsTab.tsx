import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowRight, Check, MapPin, RotateCcw, Wrench } from 'lucide-react'
import apiClient from '../../api/client'
import { apiGetPage } from '../../api/paging'
import { PageFooter } from '../../components/PageInfo'
import { MediaPhotoButton } from '../../components/MediaPhoto'
import { CardSkeleton } from '../../components/LoadingSkeleton'
import { Dot, type Tone } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { EmptyState, ErrorState, Notice } from '../../components/ui/States'
import { useErrorToast, useToast } from '../../components/toast/context'
import { useI18n } from '../../i18n/context'
import { relTime, useNow } from '../patrol/format'
import { WORKFLOW } from './constants'
import type { DefectReport, Site, WorkflowStatus } from '../../types'

const colTone: Record<WorkflowStatus, Tone> = { open: 'orange', in_progress: 'blue', resolved: 'green' }
const RESOLVED_PREVIEW = 6
/** The API maximum per request; more is unusual for one site's board. */
const LIMIT = 1000

interface Move {
  to: WorkflowStatus
  label: 'ops.def.start' | 'ops.def.resolve' | 'ops.def.reopen'
  primary?: boolean
}

const moves: Record<WorkflowStatus, Move[]> = {
  open: [{ to: 'in_progress', label: 'ops.def.start', primary: true }],
  in_progress: [
    { to: 'open', label: 'ops.def.reopen' },
    { to: 'resolved', label: 'ops.def.resolve', primary: true },
  ],
  resolved: [{ to: 'open', label: 'ops.def.reopen' }],
}

function DefectCard({ d, now, busy, onMove }: { d: DefectReport; now: number; busy: boolean; onMove: (s: WorkflowStatus) => void }) {
  const { t, lang } = useI18n()
  const title = d.defect_type || t('def.type')
  return (
    <li className="rounded-xl border border-gray-200/80 bg-white p-4 shadow-card">
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <p className="truncate text-sm font-semibold text-gray-900">{title}</p>
          {d.location && (
            <p className="mt-0.5 flex items-center gap-1 text-xs text-gray-500">
              <MapPin className="h-3.5 w-3.5 shrink-0 text-gray-500" aria-hidden="true" />
              <span className="truncate">{d.location}</span>
            </p>
          )}
        </div>
        {d.photo_object_key && (
          <div className="-mr-2 -mt-1 shrink-0">
            <MediaPhotoButton objectKey={d.photo_object_key} title={`${title}${d.location ? ` — ${d.location}` : ''}`} />
          </div>
        )}
      </div>
      {d.description && <p className="mt-2 line-clamp-3 text-sm text-gray-700">{d.description}</p>}
      <p className="mt-2 text-xs text-gray-500">
        {d.guard_name || '—'} · {relTime(d.created_at, lang, now)}
      </p>
      <div className="mt-3 flex flex-wrap justify-end gap-1.5 border-t border-gray-100 pt-3">
        {moves[d.status].map((m) => (
          <Button
            key={m.to}
            size="sm"
            variant={m.primary ? (m.to === 'resolved' ? 'primary' : 'soft') : 'ghost'}
            disabled={busy}
            onClick={() => onMove(m.to)}
            aria-label={`${t(m.label)} — ${title}`}
          >
            {m.to === 'resolved' ? (
              <Check className="h-3.5 w-3.5" aria-hidden="true" />
            ) : m.to === 'open' ? (
              <RotateCcw className="h-3.5 w-3.5" aria-hidden="true" />
            ) : (
              <ArrowRight className="h-3.5 w-3.5" aria-hidden="true" />
            )}
            {t(m.label)}
          </Button>
        ))}
      </div>
    </li>
  )
}

/** Defect board: Open → In progress → Resolved. Stacks into a list on small screens. */
export function DefectsTab({ site }: { site: Site }) {
  const { t } = useI18n()
  const qc = useQueryClient()
  const toast = useToast()
  const onError = useErrorToast()
  const now = useNow(60_000)
  const [showAllResolved, setShowAllResolved] = useState(false)

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['defects', site.id, 'board'],
    queryFn: () => apiGetPage<DefectReport>('/defects', { site_id: site.id, limit: LIMIT }),
  })

  const update = useMutation({
    mutationFn: ({ id, s }: { id: string; s: WorkflowStatus }) => apiClient.put(`/defects/${id}/status`, { status: s }),
    onSuccess: () => {
      toast.success(t('inc.statusDone'))
      qc.invalidateQueries({ queryKey: ['defects', site.id] })
      qc.invalidateQueries({ queryKey: ['dashboard', site.id] })
    },
    onError,
  })

  if (error) return <ErrorState error={error} onRetry={() => refetch()} />
  if (isLoading)
    return (
      <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
        {WORKFLOW.map((s) => (
          <CardSkeleton key={s} />
        ))}
      </div>
    )

  const all = data?.rows ?? []
  if (all.length === 0) return <EmptyState icon={Wrench} title={t('ops.def.emptyTitle')} message={t('ops.def.emptyBody')} />

  return (
    <>
      <Notice>{t('ops.def.hint')}</Notice>
      <div className="grid grid-cols-1 items-start gap-4 md:grid-cols-3">
        {WORKFLOW.map((status) => {
          const items = all
            .filter((d) => d.status === status)
            .sort((a, b) => (status === 'resolved' ? b.updated_at.localeCompare(a.updated_at) : a.created_at.localeCompare(b.created_at)))
          const shown = status === 'resolved' && !showAllResolved ? items.slice(0, RESOLVED_PREVIEW) : items
          return (
            <section key={status} aria-labelledby={`def-col-${status}`} className="rounded-2xl bg-gray-100/70 p-3 ring-1 ring-inset ring-gray-200/70">
              <h3 id={`def-col-${status}`} className="mb-3 flex items-center gap-2 px-1 text-sm font-semibold text-gray-800">
                <Dot tone={colTone[status]} />
                {t(`status.${status}`)}
                <span className="ml-auto rounded-full bg-white px-2 py-px text-xs font-medium tabular-nums text-gray-600 ring-1 ring-gray-200">{items.length}</span>
              </h3>
              {items.length === 0 ? (
                <p className="rounded-xl border border-dashed border-gray-300 px-3 py-6 text-center text-xs text-gray-500">{t('ops.def.colEmpty')}</p>
              ) : (
                <ul className="space-y-2.5">
                  {shown.map((d) => (
                    <DefectCard
                      key={d.id}
                      d={d}
                      now={now}
                      busy={update.isPending && update.variables?.id === d.id}
                      onMove={(s) => update.mutate({ id: d.id, s })}
                    />
                  ))}
                </ul>
              )}
              {shown.length < items.length && (
                <Button variant="ghost" size="sm" className="mt-2 w-full" onClick={() => setShowAllResolved(true)}>
                  {t('ops.def.showAll', { n: items.length })}
                </Button>
              )}
            </section>
          )
        })}
      </div>
      {data && <PageFooter shown={all.length} total={data.total} truncated={data.truncated} />}
    </>
  )
}
