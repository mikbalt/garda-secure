import { useState, type FormEvent, type ReactNode } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { FilePen, FileText, ImageOff, Plus } from 'lucide-react'
import apiClient, { apiGet } from '../../api/client'
import { TableSkeleton } from '../../components/LoadingSkeleton'
import { FlagBadges } from '../../components/FlagBadges'
import { canOpenInTab, mediaUrl } from '../../lib/media'
import { SeverityBadge, StatusBadge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Field, FormError, Input, Select, Textarea } from '../../components/ui/Form'
import { Modal } from '../../components/ui/Modal'
import { ErrorState } from '../../components/ui/States'
import { useErrorToast, useToast } from '../../components/toast/context'
import { useAuth } from '../../hooks/useAuth'
import { incidentTypeLabel, translateMaybe, useI18n } from '../../i18n/context'
import { errorText } from '../../i18n/errors'
import { formatDateTime } from '../../lib/time'
import { relTime, useNow } from '../patrol/format'
import { Avatar, Segmented } from '../patrol/kit'
import { KNOWN_TYPES, SEVERITIES, WORKFLOW, sevBar } from './constants'
import type { IncidentDetail, Photo, Severity, Site, WorkflowStatus } from '../../types'

function SectionTitle({ id, children, action }: { id: string; children: string; action?: ReactNode }) {
  return (
    <div className="mb-3 flex items-center justify-between gap-2">
      <h3 id={id} className="text-sm font-semibold text-gray-900">
        {children}
      </h3>
      {action}
    </div>
  )
}

function PhotoGrid({ photos }: { photos: Photo[] }) {
  const { t } = useI18n()
  if (photos.length === 0) return <p className="rounded-xl border border-dashed border-gray-300 px-4 py-5 text-center text-sm text-gray-600">{t('ops.inc.noPhotos')}</p>
  return (
    <ul className="grid grid-cols-3 gap-2 sm:grid-cols-4">
      {photos.map((p, i) => {
        // Only https:/blob:/same-origin URLs are rendered (WEB-9).
        const url = p.uploaded ? mediaUrl(p.url) : undefined
        const alt = t('ops.inc.photoAlt', { n: i + 1 })
        return (
          <li key={p.key} className="aspect-square overflow-hidden rounded-xl border border-gray-200 bg-gray-100">
            {url && canOpenInTab(url) ? (
              <a
                href={url}
                target="_blank"
                rel="noreferrer noopener"
                className="group block h-full focus:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary-500"
                aria-label={`${alt} — ${t('common.openFull')}`}
              >
                <img src={url} alt="" className="h-full w-full object-cover transition-transform duration-300 group-hover:scale-105" loading="lazy" />
              </a>
            ) : url ? (
              <img src={url} alt={alt} className="h-full w-full object-cover" loading="lazy" />
            ) : (
              <div className="flex h-full flex-col items-center justify-center gap-1.5 p-2 text-center text-xs text-gray-600">
                <ImageOff className="h-6 w-6 text-gray-500" aria-hidden="true" />
                {p.uploaded ? t('plat.photoBlocked') : t('common.pendingUpload')}
              </div>
            )}
          </li>
        )
      })}
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
    <form onSubmit={submit} className="mt-4 space-y-3 rounded-xl border border-primary-100 bg-primary-50/40 p-4" noValidate aria-label={t('inc.addAmendment')}>
      <p className="text-sm text-gray-700">{t('inc.amendHint')}</p>
      <FormError message={formError || (save.error ? errorText(save.error, lang) : null)} />
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <Field label={t('inc.category')} optional>
          <Select value={type} onChange={(e) => setType(e.target.value)}>
            <option value="">{t('ops.inc.noChange')}</option>
            {KNOWN_TYPES.map((x) => (
              <option key={x} value={x}>
                {incidentTypeLabel(lang, x)}
              </option>
            ))}
          </Select>
        </Field>
        <Field label={t('severity.label')} optional>
          <Select value={severity} onChange={(e) => setSeverity(e.target.value as Severity | '')}>
            <option value="">{t('ops.inc.noChange')}</option>
            {SEVERITIES.map((s) => (
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
        <Field label={t('common.reason')} required className="sm:col-span-2" hint={t('ops.inc.reasonHint')}>
          <Textarea value={reason} onChange={(e) => setReason(e.target.value)} rows={2} placeholder={t('ops.inc.reasonPh')} />
        </Field>
      </div>
      <div className="flex justify-end gap-2">
        <Button variant="ghost" onClick={onDone}>
          {t('common.cancel')}
        </Button>
        <Button type="submit" loading={save.isPending}>
          {t('ops.inc.saveAmendment')}
        </Button>
      </div>
    </form>
  )
}

/** Incident detail drawer: report, photos, history (amendments) and actions. */
export function IncidentDrawer({ site, id, onClose }: { site: Site; id: string; onClose: () => void }) {
  const { t, lang } = useI18n()
  const { can } = useAuth()
  const qc = useQueryClient()
  const toast = useToast()
  const onError = useErrorToast()
  const tz = site.timezone
  const now = useNow(60_000)
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
  const fieldValue = (k: string, v: string) => (k === 'severity' ? translateMaybe(lang, `severity.${v}`, v) : k === 'type' ? incidentTypeLabel(lang, v) : v)

  return (
    <Modal open onClose={onClose} variant="drawer" title={t('inc.detail')}>
      {isLoading ? (
        <TableSkeleton rows={6} cols={2} />
      ) : error ? (
        <ErrorState error={error} onRetry={() => refetch()} />
      ) : inc && data ? (
        <div className="space-y-7">
          {/* Header */}
          <div className="relative overflow-hidden rounded-2xl border border-gray-200/80 bg-white p-4 pl-5 shadow-card">
            <span className={`absolute inset-y-0 left-0 w-1.5 ${sevBar[inc.severity] ?? sevBar.low}`} aria-hidden="true" />
            <div className="flex flex-wrap items-center gap-2">
              <SeverityBadge severity={inc.severity} />
              <StatusBadge status={inc.status} />
              <FlagBadges flags={inc.flags} />
            </div>
            <p className="mt-2 text-lg font-semibold tracking-tight text-gray-900">{incidentTypeLabel(lang, inc.type)}</p>
            <p className="text-sm text-gray-500">
              {formatDateTime(inc.occurred_at, tz, lang)} · {relTime(inc.occurred_at, lang, now)}
            </p>
          </div>

          {manage && (
            <section aria-labelledby="inc-status">
              <Segmented
                label={t('ops.inc.statusLabel')}
                value={inc.status}
                disabled={status.isPending}
                onChange={(s) => s !== inc.status && status.mutate(s)}
                options={WORKFLOW.map((s) => ({ value: s, label: t(`status.${s}`), tone: ({ open: 'orange', in_progress: 'blue', resolved: 'green' } as const)[s] }))}
              />
              <p id="inc-status" className="mt-1.5 text-xs text-gray-500">
                {t('ops.inc.statusHint')}
              </p>
            </section>
          )}

          <section aria-labelledby="inc-desc">
            <SectionTitle id="inc-desc">{t('common.description')}</SectionTitle>
            <p className="whitespace-pre-wrap rounded-xl bg-gray-50 px-4 py-3 text-sm leading-relaxed text-gray-800">{inc.description || t('ops.inc.noDesc')}</p>
            <dl className="mt-4 grid grid-cols-1 gap-x-6 gap-y-3 text-sm sm:grid-cols-2">
              <div>
                <dt className="text-xs font-medium text-gray-500">{t('common.location')}</dt>
                <dd className="mt-0.5 text-gray-900">{inc.location || '—'}</dd>
              </div>
              <div>
                <dt className="text-xs font-medium text-gray-500">{t('inc.reportedBy')}</dt>
                <dd className="mt-0.5 flex items-center gap-2 text-gray-900">
                  <Avatar name={data.guard_name} className="!h-6 !w-6 !text-[10px]" />
                  {data.guard_name || '—'}
                </dd>
              </div>
              <div>
                <dt className="text-xs font-medium text-gray-500">{t('inc.occurred')}</dt>
                <dd className="mt-0.5 text-gray-900">{formatDateTime(inc.occurred_at, tz, lang)}</dd>
              </div>
              <div>
                <dt className="text-xs font-medium text-gray-500">{t('ops.inc.receivedAt')}</dt>
                <dd className="mt-0.5 text-gray-900">{formatDateTime(inc.created_at, tz, lang)}</dd>
              </div>
            </dl>
          </section>

          <section aria-labelledby="inc-photos">
            <SectionTitle id="inc-photos">{`${t('inc.photos')} (${data.photos.length})`}</SectionTitle>
            <PhotoGrid photos={data.photos} />
          </section>

          <section aria-labelledby="inc-history">
            <SectionTitle
              id="inc-history"
              action={
                manage && !amending ? (
                  <Button size="sm" variant="soft" onClick={() => setAmending(true)}>
                    <Plus className="h-3.5 w-3.5" aria-hidden="true" />
                    {t('inc.addAmendment')}
                  </Button>
                ) : undefined
              }
            >
              {t('ops.inc.history')}
            </SectionTitle>
            <p className="mb-4 text-xs text-gray-500">{t('ops.inc.immutable')}</p>
            <ol>
              <li className="relative pb-5 pl-11">
                {data.amendments.length > 0 && <span className="absolute bottom-0 left-[15px] top-8 w-px bg-gray-200" aria-hidden="true" />}
                <span className="absolute left-0 top-0 flex h-8 w-8 items-center justify-center rounded-full bg-gray-100 text-gray-500 ring-1 ring-gray-200" aria-hidden="true">
                  <FileText className="h-4 w-4" />
                </span>
                <p className="pt-1 text-sm font-medium text-gray-900">{t('ops.inc.createdBy', { who: data.guard_name || '—' })}</p>
                <p className="text-xs text-gray-500">{formatDateTime(inc.created_at, tz, lang)}</p>
              </li>
              {data.amendments.map((a, i) => (
                <li key={a.id} className="relative pb-5 pl-11">
                  {i < data.amendments.length - 1 && <span className="absolute bottom-0 left-[15px] top-8 w-px bg-gray-200" aria-hidden="true" />}
                  <span className="absolute left-0 top-0 flex h-8 w-8 items-center justify-center rounded-full bg-primary-50 text-primary-600 ring-1 ring-primary-200" aria-hidden="true">
                    <FilePen className="h-4 w-4" />
                  </span>
                  <p className="pt-1 text-sm font-medium text-gray-900">{t('ops.inc.amendedBy', { who: a.amended_by })}</p>
                  <p className="text-xs text-gray-500">{formatDateTime(a.created_at, tz, lang)}</p>
                  <dl className="mt-2 space-y-1 rounded-xl border border-gray-200/80 bg-white p-3 text-sm">
                    {Object.entries(a.changes ?? {}).map(([k, v]) => (
                      <div key={k} className="flex gap-2">
                        <dt className="w-32 shrink-0 text-gray-500">{fieldLabel(k)}</dt>
                        <dd className="min-w-0 font-medium text-gray-900">{fieldValue(k, v)}</dd>
                      </div>
                    ))}
                  </dl>
                  <p className="mt-2 border-l-2 border-gray-200 pl-3 text-sm italic text-gray-600">
                    <span className="sr-only">{t('common.reason')}: </span>
                    {a.reason}
                  </p>
                </li>
              ))}
            </ol>
            {amending && <AmendForm site={site} id={id} onDone={() => setAmending(false)} />}
          </section>
        </div>
      ) : null}
    </Modal>
  )
}
