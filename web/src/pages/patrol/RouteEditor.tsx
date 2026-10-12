import { useRef, useState, type FormEvent } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ArrowDown, ArrowUp, MapPin, Plus, Trash2 } from 'lucide-react'
import apiClient from '../../api/client'
import { Button } from '../../components/ui/Button'
import { Checkbox, Field, FormError, Input } from '../../components/ui/Form'
import { Modal } from '../../components/ui/Modal'
import { Notice } from '../../components/ui/States'
import { useToast } from '../../components/toast/context'
import { useI18n } from '../../i18n/context'
import { errorText } from '../../i18n/errors'
import { fmtDuration } from './format'
import type { PatrolRoute, RouteInput, Site } from '../../types'

interface Row {
  key: string
  id?: string
  name: string
  qr_code: string
  sla: string
  lat: string
  lng: string
  showGps: boolean
}

function toRows(route: PatrolRoute | null): Row[] {
  if (!route) return []
  return [...route.checkpoints]
    .sort((a, b) => a.order_index - b.order_index)
    .map((c) => ({
      key: c.id,
      id: c.id,
      name: c.name,
      qr_code: c.qr_code,
      sla: c.sla_minutes != null ? String(c.sla_minutes) : '',
      lat: c.lat != null ? String(c.lat) : '',
      lng: c.lng != null ? String(c.lng) : '',
      showGps: false,
    }))
}

function num(v: string): number | null {
  if (v.trim() === '') return null
  const n = Number(v)
  return Number.isFinite(n) ? n : null
}

const iconBtn = 'h-8 w-8 !px-0'
const miniLabel = 'mb-1 block text-xs font-medium text-gray-500 sm:sr-only'

/** Route editor: ordered checkpoints (add / remove / reorder) + active toggle. */
export function RouteEditor({ site, route, onClose }: { site: Site; route: PatrolRoute | null; onClose: () => void }) {
  const { t, lang } = useI18n()
  const qc = useQueryClient()
  const toast = useToast()
  const [name, setName] = useState(route?.name ?? '')
  const [active, setActive] = useState(route?.active ?? true)
  const [rows, setRows] = useState<Row[]>(() => toRows(route))
  const [formError, setFormError] = useState('')
  const [submitted, setSubmitted] = useState(false)
  const [focusKey, setFocusKey] = useState<string | null>(null)
  const seq = useRef(0)

  const update = (i: number, patch: Partial<Row>) => setRows((rs) => rs.map((r, j) => (j === i ? { ...r, ...patch } : r)))
  const move = (i: number, d: number) =>
    setRows((rs) => {
      const j = i + d
      if (j < 0 || j >= rs.length) return rs
      const next = [...rs]
      ;[next[i], next[j]] = [next[j], next[i]]
      return next
    })
  const add = () => {
    const key = `new-${seq.current++}`
    setFocusKey(key)
    setRows((rs) => [...rs, { key, name: '', qr_code: '', sla: '', lat: '', lng: '', showGps: false }])
  }

  const totalSla = rows.reduce((n, r) => n + (num(r.sla) ?? 0), 0)

  const save = useMutation({
    mutationFn: async () => {
      const body: RouteInput = {
        name: name.trim(),
        active,
        checkpoints: rows.map((r, i) => ({
          ...(r.id ? { id: r.id } : {}),
          name: r.name.trim(),
          ...(r.qr_code.trim() ? { qr_code: r.qr_code.trim() } : {}),
          sla_minutes: num(r.sla),
          order_index: i,
          lat: num(r.lat),
          lng: num(r.lng),
        })),
      }
      if (route) await apiClient.put(`/patrol/routes/${route.id}`, body)
      else await apiClient.post('/patrol/routes', { ...body, site_id: site.id })
    },
    onSuccess: () => {
      toast.success(t('pat.routeSaved'))
      qc.invalidateQueries({ queryKey: ['patrol-routes', site.id] })
      onClose()
    },
  })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    setSubmitted(true)
    if (!name.trim() || rows.some((r) => !r.name.trim())) return setFormError(t('ops.pat.fillNames'))
    if (rows.length === 0) return setFormError(t('pat.needCheckpoint'))
    setFormError('')
    save.mutate()
  }

  return (
    <Modal
      open
      onClose={onClose}
      size="xl"
      title={route ? t('pat.editRoute') : t('pat.newRoute')}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form="route-form" loading={save.isPending}>
            {route ? t('common.save') : t('ops.pat.createRoute')}
          </Button>
        </>
      }
    >
      <form id="route-form" onSubmit={submit} className="space-y-5" noValidate>
        <FormError message={formError || (save.error ? errorText(save.error, lang) : null)} />
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-start">
          <Field label={t('pat.routeName')} required error={submitted && !name.trim() ? t('common.required') : undefined}>
            <Input value={name} onChange={(e) => setName(e.target.value)} maxLength={100} placeholder={t('ops.pat.routeNamePh')} />
          </Field>
          <div className="sm:pt-8">
            <Checkbox label={t('ops.pat.activeLabel')} checked={active} onChange={(e) => setActive(e.target.checked)} />
          </div>
        </div>

        <fieldset>
          <legend className="text-sm font-semibold text-gray-900">{t('pat.checkpoints')}</legend>
          <p className="mb-3 mt-0.5 text-sm text-gray-500">{t('ops.pat.cpIntro')}</p>
          <Notice>{t('ops.pat.slaHelp')}</Notice>

          {rows.length === 0 ? (
            <div className="rounded-xl border border-dashed border-gray-300 bg-gray-50/60 px-4 py-6 text-center text-sm text-gray-600">{t('ops.pat.cpEmpty')}</div>
          ) : (
            <>
              <div className="hidden grid-cols-[2rem_minmax(0,1fr)_10rem_7.5rem_9.5rem] gap-3 px-3 pb-1.5 text-[11px] font-semibold uppercase tracking-wider text-gray-500 sm:grid" aria-hidden="true">
                <span>#</span>
                <span>{t('pat.cpName')}</span>
                <span>{t('pat.qrCode')}</span>
                <span>{t('ops.pat.slaCol')}</span>
                <span />
              </div>
              <ol className="space-y-2">
                {rows.map((r, i) => {
                  const label = r.name || `#${i + 1}`
                  const hasGps = !!(r.lat || r.lng)
                  return (
                    <li key={r.key} className="rounded-xl border border-gray-200 bg-white p-3 shadow-sm">
                      <div className="grid grid-cols-[2rem_minmax(0,1fr)] items-start gap-3 sm:grid-cols-[2rem_minmax(0,1fr)_10rem_7.5rem_9.5rem] sm:items-center">
                        <span className="mt-6 flex h-7 w-7 items-center justify-center rounded-full bg-primary-600 text-xs font-semibold text-white sm:mt-0" aria-hidden="true">
                          {i + 1}
                        </span>
                        <div className="min-w-0">
                          <label htmlFor={`cp-name-${r.key}`} className={miniLabel}>
                            {t('pat.cpName')} {i + 1}
                          </label>
                          <Input
                            id={`cp-name-${r.key}`}
                            value={r.name}
                            onChange={(e) => update(i, { name: e.target.value })}
                            maxLength={100}
                            placeholder={t('ops.pat.cpNamePh')}
                            aria-invalid={submitted && !r.name.trim() ? true : undefined}
                            autoFocus={r.key === focusKey}
                          />
                        </div>
                        <div className="col-start-2 sm:col-start-auto">
                          <label htmlFor={`cp-qr-${r.key}`} className={miniLabel}>
                            {t('pat.qrCode')}
                          </label>
                          <Input
                            id={`cp-qr-${r.key}`}
                            value={r.qr_code}
                            onChange={(e) => update(i, { qr_code: e.target.value })}
                            className="font-mono text-xs"
                            maxLength={100}
                            placeholder={t('ops.pat.qrPh')}
                          />
                        </div>
                        <div className="col-start-2 sm:col-start-auto">
                          <label htmlFor={`cp-sla-${r.key}`} className={miniLabel}>
                            {t('ops.pat.slaCol')}
                          </label>
                          <div className="relative">
                            <Input
                              id={`cp-sla-${r.key}`}
                              type="number"
                              inputMode="numeric"
                              min={1}
                              max={1440}
                              value={r.sla}
                              onChange={(e) => update(i, { sla: e.target.value })}
                              placeholder="—"
                              className="pr-11 tabular-nums"
                            />
                            <span className="pointer-events-none absolute inset-y-0 right-3 flex items-center text-xs text-gray-500">{t('ops.pat.minShort')}</span>
                          </div>
                        </div>
                        <div className="col-start-2 flex items-center gap-0.5 sm:col-start-auto sm:justify-end">
                          <Button
                            size="sm"
                            variant="ghost"
                            className={`${iconBtn} ${hasGps || r.showGps ? 'text-primary-600' : ''}`}
                            onClick={() => update(i, { showGps: !r.showGps })}
                            aria-expanded={r.showGps}
                            aria-label={`${t('ops.pat.gpsToggle')} — ${label}`}
                            title={t('ops.pat.gpsToggle')}
                          >
                            <MapPin className="h-4 w-4" aria-hidden="true" />
                          </Button>
                          <Button size="sm" variant="ghost" className={iconBtn} onClick={() => move(i, -1)} disabled={i === 0} aria-label={`${t('pat.moveUp')} — ${label}`} title={t('pat.moveUp')}>
                            <ArrowUp className="h-4 w-4" aria-hidden="true" />
                          </Button>
                          <Button
                            size="sm"
                            variant="ghost"
                            className={iconBtn}
                            onClick={() => move(i, 1)}
                            disabled={i === rows.length - 1}
                            aria-label={`${t('pat.moveDown')} — ${label}`}
                            title={t('pat.moveDown')}
                          >
                            <ArrowDown className="h-4 w-4" aria-hidden="true" />
                          </Button>
                          <Button
                            size="sm"
                            variant="ghost"
                            className={`${iconBtn} text-red-600 hover:bg-red-50 hover:text-red-700`}
                            onClick={() => setRows((rs) => rs.filter((_, j) => j !== i))}
                            aria-label={`${t('pat.removeCp')} — ${label}`}
                            title={t('pat.removeCp')}
                          >
                            <Trash2 className="h-4 w-4" aria-hidden="true" />
                          </Button>
                        </div>
                      </div>
                      {r.showGps && (
                        <div className="mt-3 grid grid-cols-2 gap-3 border-t border-gray-100 pt-3 sm:ml-11 sm:max-w-md">
                          <Field label={t('pat.lat')} optional>
                            <Input type="number" step="any" min={-90} max={90} value={r.lat} onChange={(e) => update(i, { lat: e.target.value })} placeholder="-6.2944" />
                          </Field>
                          <Field label={t('pat.lng')} optional>
                            <Input type="number" step="any" min={-180} max={180} value={r.lng} onChange={(e) => update(i, { lng: e.target.value })} placeholder="107.1531" />
                          </Field>
                        </div>
                      )}
                    </li>
                  )
                })}
              </ol>
            </>
          )}

          <div className="mt-3 flex flex-wrap items-center justify-between gap-3">
            <Button variant="soft" onClick={add}>
              <Plus className="h-4 w-4" aria-hidden="true" />
              {t('pat.addCheckpoint')}
            </Button>
            {totalSla > 0 && <p className="text-xs text-gray-500">{t('ops.pat.totalSla', { d: fmtDuration(totalSla * 60_000, t) })}</p>}
          </div>
        </fieldset>
      </form>
    </Modal>
  )
}
