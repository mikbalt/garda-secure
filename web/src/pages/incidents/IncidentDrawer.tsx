import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ImageOff } from 'lucide-react'
import apiClient, { apiGet } from '../../api/client'
import { TableSkeleton } from '../../components/LoadingSkeleton'
import { SeverityBadge, StatusBadge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Field, FormError, Input, Select, Textarea } from '../../components/ui/Form'
import { Modal } from '../../components/ui/Modal'
import { ErrorState } from '../../components/ui/States'
import { useErrorToast, useToast } from '../../components/toast/context'
import { useAuth } from '../../hooks/useAuth'
import { useI18n, translateMaybe } from '../../i18n/context'
import { errorText } from '../../i18n/errors'
import { formatDateTime } from '../../lib/time'
import type { IncidentDetail, Photo, Severity, Site, WorkflowStatus } from '../../types'

function PhotoGrid({ photos }: { photos: Photo[] }) {
  const { t } = useI18n()
  if (photos.length === 0) return <p className="text-sm text-gray-600">{t('inc.noPhotos')}</p>
  return (
    <ul className="grid grid-cols-2 gap-2 sm:grid-cols-3">
      {photos.map((p, i) => (
        <li key={p.key} className="aspect-square overflow-hidden rounded-lg border border-gray-200 bg-gray-100">
          {p.uploaded && p.url ? (
            <a href={p.url} target="_blank" rel="noreferrer" className="block h-full focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-600">
              <img src={p.url} alt={`${t('common.photo')} ${i + 1}`} className="h-full w-full object-cover" loading="lazy" />
            </a>
          ) : (
            <div className="flex h-full flex-col items-center justify-center gap-1 p-2 text-center text-xs text-gray-700">
              <ImageOff className="h-6 w-6" aria-hidden="true" />
              {t('common.pendingUpload')}
            </div>
          )}
        </li>
      ))}
    </ul>
  )
}

function AmendForm({ site, id, onDone }: { site: Site; id: string; onDone: () => void }) {
  const { t, lang } = useI18n()
  const qc = useQueryClient()
  const toast = useToast()
  const [type, setType] = useState('')
  const [severity, setSeverity] = useState('')
  const [description, setDescription] = useState('')
  const [location, setLocation] = useState('')
  const [reason, setReason] = useState('')
  const [formError, setFormError] = useState('')

  const changes: Record<string, string> = {}
  if (type.trim()) changes.type = type.trim().toLowerCase()
  if (severity) changes.severity = severity
  if (description.trim()) changes.description = description.trim()
  if (location.trim()) changes.location = location.trim()

  const save = useMutation({
    mutationFn: () => apiClient.post(`/incidents/${id}/amendments`, { changes, reason: reason.trim() }),
    onSuccess: () => {
      toast.success(t('inc.amendDone'))
      qc.invalidateQueries({ queryKey: ['incident', id] })
      qc.invalidateQueries({ queryKey: ['incidents', site.id] })
      onDone()
    },
  })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (Object.keys(changes).length === 0) return setFormError(t('inc.amendNothing'))
    if (!reason.trim()) return setFormError(`${t('common.reason')}: ${t('common.required')}`)
    setFormError('')
    save.mutate()
  }

  return (
    <form onSubmit={submit} className="space-y-3 rounded-lg border border-gray-200 bg-gray-50 p-4" noValidate>
      <p className="text-sm text-gray-700">{t('inc.amendHint')}</p>
      <FormError message={formError || (save.error ? errorText(save.error, lang) : null)} />
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <Field label={t('inc.category')} optional>
          <Input value={type} onChange={(e) => setType(e.target.value)} />
        </Field>
        <Field label={t('severity.label')} optional>
          <Select value={severity} onChange={(e) => setSeverity(e.target.value as Severity | '')}>
            <option value="">—</option>
            {(['low', 'medium', 'high', 'critical'] as const).map((s) => (
              <option key={s} value={s}>
                {t(`severity.${s}`)}
              </option>
            ))}
          </Select>
        </Field>
        <Field label={t('common.location')} optional className="sm:col-span-2">
          <Input value={location} onChange={(e) => setLocation(e.target.value)} />
        </Field>
        <Field label={t('common.description')} optional className="sm:col-span-2">
          <Textarea value={description} onChange={(e) => setDescription(e.target.value)} />
        </Field>
        <Field label={t('common.reason')} required className="sm:col-span-2">
          <Textarea value={reason} onChange={(e) => setReason(e.target.value)} rows={2} />
        </Field>
      </div>
      <div className="flex justify-end gap-2">
        <Button variant="secondary" onClick={onDone}>
          {t('common.cancel')}
        </Button>
        <Button type="submit" loading={save.isPending}>
          {t('common.save')}
        </Button>
      </div>
    </form>
  )
}

/** Incident detail drawer: original report, photos, amendments, actions. */
export function IncidentDrawer({ site, id, onClose }: { site: Site; id: string; onClose: () => void }) {
  const { t, lang } = useI18n()
  const { can } = useAuth()
  const qc = useQueryClient()
  const toast = useToast()
  const onError = useErrorToast()
  const tz = site.timezone
  const manage = can('manageIncidents')
  const [amending, setAmending] = useState(false)

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['incident', id],
    queryFn: () => apiGet<IncidentDetail>(`/incidents/${id}`),
  })

  const status = useMutation({
    mutationFn: (s: WorkflowStatus) => apiClient.put(`/incidents/${id}/status`, { status: s }),
    onSuccess: () => {
      toast.success(t('inc.statusDone'))
      qc.invalidateQueries({ queryKey: ['incident', id] })
      qc.invalidateQueries({ queryKey: ['incidents', site.id] })
      qc.invalidateQueries({ queryKey: ['dashboard', site.id] })
    },
    onError,
  })

  const inc = data?.incident
  const fieldLabel = (k: string) =>
    ({ type: t('inc.category'), severity: t('severity.label'), description: t('common.description'), location: t('common.location') })[k] ?? k

  return (
    <Modal open onClose={onClose} variant="drawer" title={t('inc.detail')}>
      {isLoading ? (
        <TableSkeleton rows={6} cols={2} />
      ) : error ? (
        <ErrorState error={error} onRetry={() => refetch()} />
      ) : inc && data ? (
        <div className="space-y-6">
          <section aria-labelledby="inc-original">
            <h3 id="inc-original" className="mb-2 text-sm font-semibold text-gray-900">
              {t('inc.original')}
            </h3>
            <dl className="grid grid-cols-3 gap-x-4 gap-y-2 text-sm">
              <dt className="text-gray-600">{t('inc.occurred')}</dt>
              <dd className="col-span-2">{formatDateTime(inc.occurred_at, tz, lang)}</dd>
              <dt className="text-gray-600">{t('inc.category')}</dt>
              <dd className="col-span-2 capitalize">{inc.type}</dd>
              <dt className="text-gray-600">{t('severity.label')}</dt>
              <dd className="col-span-2">
                <SeverityBadge severity={inc.severity} />
              </dd>
              <dt className="text-gray-600">{t('common.status')}</dt>
              <dd className="col-span-2">
                <StatusBadge status={inc.status} />
              </dd>
              <dt className="text-gray-600">{t('inc.reportedBy')}</dt>
              <dd className="col-span-2">{data.guard_name || '—'}</dd>
              <dt className="text-gray-600">{t('common.location')}</dt>
              <dd className="col-span-2">{inc.location ?? '—'}</dd>
              <dt className="text-gray-600">{t('common.description')}</dt>
              <dd className="col-span-2 whitespace-pre-wrap">{inc.description ?? '—'}</dd>
            </dl>
          </section>

          {manage && (
            <section aria-labelledby="inc-status">
              <h3 id="inc-status" className="sr-only">
                {t('inc.changeStatus')}
              </h3>
              <div className="flex flex-wrap items-end gap-2">
                <Field label={t('inc.changeStatus')}>
                  <Select value={inc.status} onChange={(e) => status.mutate(e.target.value as WorkflowStatus)} disabled={status.isPending}>
                    {(['open', 'in_progress', 'resolved'] as const).map((s) => (
                      <option key={s} value={s}>
                        {t(`status.${s}`)}
                      </option>
                    ))}
                  </Select>
                </Field>
              </div>
            </section>
          )}

          <section aria-labelledby="inc-photos">
            <h3 id="inc-photos" className="mb-2 text-sm font-semibold text-gray-900">
              {t('inc.photos')}
            </h3>
            <PhotoGrid photos={data.photos} />
          </section>

          <section aria-labelledby="inc-amend">
            <div className="mb-2 flex items-center justify-between">
              <h3 id="inc-amend" className="text-sm font-semibold text-gray-900">
                {t('inc.amendments')}
              </h3>
              {manage && !amending && (
                <Button size="sm" variant="secondary" onClick={() => setAmending(true)}>
                  {t('inc.addAmendment')}
                </Button>
              )}
            </div>
            {amending && <AmendForm site={site} id={id} onDone={() => setAmending(false)} />}
            {data.amendments.length === 0 ? (
              <p className="mt-2 text-sm text-gray-600">{t('inc.noAmendments')}</p>
            ) : (
              <ol className="mt-3 space-y-3">
                {data.amendments.map((a) => (
                  <li key={a.id} className="rounded-lg border border-gray-200 p-3 text-sm">
                    <div className="mb-1 text-xs text-gray-600">
                      {formatDateTime(a.created_at, tz, lang)} · {t('inc.by', { who: a.amended_by })}
                    </div>
                    <dl className="grid grid-cols-3 gap-x-3 gap-y-1">
                      {Object.entries(a.changes ?? {}).map(([k, v]) => (
                        <div key={k} className="contents">
                          <dt className="text-gray-600">{fieldLabel(k)}</dt>
                          <dd className="col-span-2">{k === 'severity' ? translateMaybe(lang, `severity.${v}`, v) : v}</dd>
                        </div>
                      ))}
                    </dl>
                    <p className="mt-1 text-gray-800">
                      <span className="text-gray-600">{t('common.reason')}:</span> {a.reason}
                    </p>
                  </li>
                ))}
              </ol>
            )}
          </section>
        </div>
      ) : null}
    </Modal>
  )
}
