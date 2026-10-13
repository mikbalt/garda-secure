import { useEffect, useId, useMemo, useState, type FormEvent } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ImageOff, Map as MapIcon, Trash2, Upload } from 'lucide-react'
import apiClient from '../../../api/client'
import { Button } from '../../../components/ui/Button'
import { Field, FormError, Input } from '../../../components/ui/Form'
import { Modal } from '../../../components/ui/Modal'
import { Card, ErrorState } from '../../../components/ui/States'
import { useErrorToast, useToast } from '../../../components/toast/context'
import { useI18n } from '../../../i18n/context'
import { ERROR_CODES, errorText } from '../../../i18n/errors'
import { mediaUrl } from '../../../lib/media'
import { formatBytes, formatDay, localDateOf } from '../../../lib/time'
import { ConfirmDialog } from '../../settings/kit'
import { FLOORPLAN_MAX_PER_SITE, FLOORPLAN_TYPES, floorplanFileError, floorplansKey, useFloorplans } from './floorplans'
import type { Floorplan, PatrolRoute, Site } from '../../../types'

function UploadModal({ site, onClose }: { site: Site; onClose: () => void }) {
  const { t, lang } = useI18n()
  const qc = useQueryClient()
  const toast = useToast()
  const [file, setFile] = useState<File | null>(null)
  const [name, setName] = useState('')
  const [err, setErr] = useState('')
  const [preview, setPreview] = useState<string | null>(null)
  const fileId = useId()
  const hintId = useId()

  // blob: preview of the chosen file (allowed by the CSP img-src).
  useEffect(() => {
    if (!file) return setPreview(null)
    const url = URL.createObjectURL(file)
    setPreview(url)
    return () => URL.revokeObjectURL(url)
  }, [file])

  const upload = useMutation({
    mutationFn: async () => {
      const form = new FormData()
      form.append('file', file as File)
      form.append('name', name.trim())
      const { data } = await apiClient.post<Floorplan>(`/sites/${site.id}/floorplans`, form, { headers: { 'Content-Type': 'multipart/form-data' } })
      return data
    },
    onSuccess: () => {
      toast.success(t('p2.fp.uploaded'))
      void qc.invalidateQueries({ queryKey: floorplansKey(site.id) })
      onClose()
    },
  })

  const choose = (f: File | null) => {
    setFile(f)
    setErr('')
    if (f && !name.trim()) setName(f.name.replace(/\.[^.]+$/, '').slice(0, 200))
    const code = f ? floorplanFileError(f) : null
    if (code) setErr(ERROR_CODES[code][lang])
  }

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const code = floorplanFileError(file)
    if (code) return setErr(code === 'file_required' ? t('p2.fp.chooseFile') : ERROR_CODES[code][lang])
    if (!name.trim()) return setErr(ERROR_CODES.name_required[lang])
    setErr('')
    upload.mutate()
  }

  return (
    <Modal
      open
      onClose={onClose}
      title={t('p2.fp.uploadTitle')}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form="fp-upload" loading={upload.isPending}>
            <Upload className="h-4 w-4" aria-hidden="true" />
            {t('p2.fp.upload')}
          </Button>
        </>
      }
    >
      <form id="fp-upload" onSubmit={submit} noValidate className="space-y-4">
        <FormError message={err || (upload.error ? errorText(upload.error, lang) : null)} />
        <div>
          <label htmlFor={fileId} className="mb-1.5 block text-sm font-medium text-gray-700">
            {t('p2.fp.file')}
            <span className="text-red-700" aria-hidden="true">
              {' '}
              *
            </span>
          </label>
          <input
            id={fileId}
            type="file"
            accept={FLOORPLAN_TYPES.join(',')}
            aria-describedby={hintId}
            required
            onChange={(e) => choose(e.target.files?.[0] ?? null)}
            className="block w-full text-sm text-gray-700 file:mr-3 file:rounded-lg file:border-0 file:bg-primary-50 file:px-3 file:py-2 file:text-sm file:font-medium file:text-primary-700 hover:file:bg-primary-100"
          />
          <p id={hintId} className="mt-1.5 text-xs text-gray-500">
            {t('p2.fp.fileHint')}
          </p>
        </div>
        {preview && (
          <figure className="overflow-hidden rounded-xl border border-gray-200 bg-gray-50">
            <img src={preview} alt={t('p2.fp.previewAlt')} className="max-h-72 w-full object-contain" />
            {file && (
              <figcaption className="border-t border-gray-200 px-3 py-1.5 text-xs text-gray-600">
                {file.name} · {formatBytes(file.size)}
              </figcaption>
            )}
          </figure>
        )}
        <Field label={t('p2.fp.name')} required hint={t('p2.fp.nameHint')}>
          <Input value={name} onChange={(e) => setName(e.target.value)} maxLength={200} placeholder={t('p2.fp.namePh')} />
        </Field>
      </form>
    </Modal>
  )
}

function Thumb({ plan }: { plan: Floorplan }) {
  const { t } = useI18n()
  const [failed, setFailed] = useState(false)
  const src = mediaUrl(plan.url)
  return (
    <div className="flex aspect-[3/2] items-center justify-center overflow-hidden rounded-lg bg-gray-50 ring-1 ring-inset ring-gray-200">
      {src && !failed ? (
        <img src={src} alt="" className="h-full w-full object-contain" loading="lazy" onError={() => setFailed(true)} />
      ) : (
        <span className="flex flex-col items-center gap-1 text-xs text-gray-500">
          <ImageOff className="h-5 w-5" aria-hidden="true" />
          {t('p2.fp.imageFailed')}
        </span>
      )}
    </div>
  )
}

/** Patrol → Routes: the site's floor plans (D-048) — upload, preview, delete. */
export function FloorplansCard({ site, routes, canEdit }: { site: Site; routes: PatrolRoute[]; canEdit: boolean }) {
  const { t, lang } = useI18n()
  const qc = useQueryClient()
  const toast = useToast()
  const onError = useErrorToast()
  const [uploading, setUploading] = useState(false)
  const [removing, setRemoving] = useState<Floorplan | null>(null)
  const { data, isLoading, error, refetch } = useFloorplans(site.id)
  const plans = useMemo(() => [...(data ?? [])].sort((a, b) => a.name.localeCompare(b.name)), [data])

  const usage = useMemo(() => {
    const m = new Map<string, number>()
    for (const r of routes) for (const c of r.checkpoints) if (c.floorplan_id) m.set(c.floorplan_id, (m.get(c.floorplan_id) ?? 0) + 1)
    return m
  }, [routes])

  const del = useMutation({
    mutationFn: (id: string) => apiClient.delete(`/sites/${site.id}/floorplans/${id}`),
    onSuccess: () => {
      toast.success(t('p2.fp.deleted'))
      setRemoving(null)
      void qc.invalidateQueries({ queryKey: floorplansKey(site.id) })
    },
    onError: (e) => {
      setRemoving(null)
      onError(e)
    },
  })

  const full = plans.length >= FLOORPLAN_MAX_PER_SITE
  const inUse = removing ? (usage.get(removing.id) ?? 0) : 0

  return (
    <Card
      icon={MapIcon}
      title={t('p2.fp.title')}
      description={t(canEdit ? 'p2.fp.desc' : 'p2.fp.descRo')}
      className="mb-6"
      actions={
        canEdit ? (
          <Button variant="secondary" onClick={() => setUploading(true)} disabled={full} title={full ? t('p2.fp.full') : undefined}>
            <Upload className="h-4 w-4" aria-hidden="true" />
            {t('p2.fp.upload')}
          </Button>
        ) : undefined
      }
    >
      {error ? (
        <ErrorState error={error} onRetry={() => refetch()} />
      ) : isLoading ? (
        <div className="h-24 animate-pulse rounded-xl bg-gray-100" aria-hidden="true" />
      ) : plans.length === 0 ? (
        <div className="rounded-xl border border-dashed border-gray-300 bg-gray-50/60 px-4 py-5 text-sm text-gray-600">
          <p className="font-medium text-gray-900">{t('p2.fp.emptyTitle')}</p>
          <p className="mt-0.5">{t(canEdit ? 'p2.fp.empty' : 'p2.fp.emptyRo')}</p>
        </div>
      ) : (
        <>
          {full && canEdit && <p className="mb-3 text-xs font-medium text-amber-800">{t('p2.fp.full')}</p>}
          <ul className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4" aria-label={t('p2.fp.title')}>
            {plans.map((p) => {
              const n = usage.get(p.id) ?? 0
              return (
                <li key={p.id} className="flex flex-col gap-2 rounded-xl border border-gray-200/80 bg-white p-2.5">
                  <Thumb plan={p} />
                  <div className="flex min-w-0 items-start gap-1">
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium text-gray-900" title={p.name}>
                        {p.name}
                      </p>
                      <p className="text-xs text-gray-500">
                        {n ? t('p2.fp.used', { n }) : t('p2.fp.unused')} · {formatDay(localDateOf(p.created_at, site.timezone), lang, false)}
                      </p>
                    </div>
                    {canEdit && (
                      <Button
                        size="sm"
                        variant="ghost"
                        className="h-8 w-8 shrink-0 !px-0 text-red-600 hover:bg-red-50 hover:text-red-700"
                        onClick={() => setRemoving(p)}
                        aria-label={`${t('common.delete')} — ${p.name}`}
                        title={t('common.delete')}
                      >
                        <Trash2 className="h-4 w-4" aria-hidden="true" />
                      </Button>
                    )}
                  </div>
                </li>
              )
            })}
          </ul>
        </>
      )}
      {uploading && <UploadModal site={site} onClose={() => setUploading(false)} />}
      <ConfirmDialog
        open={!!removing}
        title={t('p2.fp.deleteTitle')}
        message={removing ? (inUse ? t('p2.fp.deleteInUse', { name: removing.name, n: inUse }) : t('p2.fp.deleteBody', { name: removing.name })) : ''}
        confirmLabel={t('common.delete')}
        loading={del.isPending}
        onConfirm={() => removing && del.mutate(removing.id)}
        onClose={() => setRemoving(null)}
      />
    </Card>
  )
}
