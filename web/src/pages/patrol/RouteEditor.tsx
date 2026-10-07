import { useRef, useState, type FormEvent } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ArrowDown, ArrowUp, Plus, Trash2 } from 'lucide-react'
import apiClient from '../../api/client'
import { Button } from '../../components/ui/Button'
import { Checkbox, Field, FormError, Input } from '../../components/ui/Form'
import { Modal } from '../../components/ui/Modal'
import { useToast } from '../../components/toast/context'
import { useI18n } from '../../i18n/context'
import { errorText } from '../../i18n/errors'
import type { PatrolRoute, RouteInput, Site } from '../../types'

interface Row {
  key: string
  id?: string
  name: string
  qr_code: string
  sla: string
  lat: string
  lng: string
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
    }))
}

function num(v: string): number | null {
  if (v.trim() === '') return null
  const n = Number(v)
  return Number.isFinite(n) ? n : null
}

/** Route editor: ordered checkpoints (add / remove / reorder) + active toggle. */
export function RouteEditor({ site, route, onClose }: { site: Site; route: PatrolRoute | null; onClose: () => void }) {
  const { t, lang } = useI18n()
  const qc = useQueryClient()
  const toast = useToast()
  const [name, setName] = useState(route?.name ?? '')
  const [active, setActive] = useState(route?.active ?? true)
  const [rows, setRows] = useState<Row[]>(() => toRows(route))
  const [formError, setFormError] = useState('')
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
  const add = () => setRows((rs) => [...rs, { key: `new-${seq.current++}`, name: '', qr_code: '', sla: '', lat: '', lng: '' }])

  const save = useMutation({
    mutationFn: () => {
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
      return route ? apiClient.put(`/patrol/routes/${route.id}`, body) : apiClient.post('/patrol/routes', { ...body, site_id: site.id })
    },
    onSuccess: () => {
      toast.success(t('pat.routeSaved'))
      qc.invalidateQueries({ queryKey: ['patrol-routes', site.id] })
      onClose()
    },
  })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!name.trim() || rows.some((r) => !r.name.trim())) return setFormError(t('common.required'))
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
            {t('common.save')}
          </Button>
        </>
      }
    >
      <form id="route-form" onSubmit={submit} className="space-y-4" noValidate>
        <FormError message={formError || (save.error ? errorText(save.error, lang) : null)} />
        <div className="flex flex-wrap items-end gap-4">
          <Field label={t('pat.routeName')} required className="min-w-[16rem] flex-1">
            <Input value={name} onChange={(e) => setName(e.target.value)} maxLength={100} />
          </Field>
          <Checkbox label={t('common.active')} checked={active} onChange={(e) => setActive(e.target.checked)} className="pb-2" />
        </div>

        <fieldset>
          <legend className="mb-2 text-sm font-semibold text-gray-900">{t('pat.checkpoints')}</legend>
          <ol className="space-y-3">
            {rows.map((r, i) => (
              <li key={r.key} className="rounded-lg border border-gray-200 bg-gray-50 p-3">
                <div className="mb-2 flex items-center gap-2">
                  <span className="flex h-6 w-6 items-center justify-center rounded-full bg-primary-700 text-xs font-semibold text-white" aria-hidden="true">
                    {i + 1}
                  </span>
                  <span className="sr-only">#{i + 1}</span>
                  <span className="ml-auto flex gap-1">
                    <Button size="sm" variant="ghost" onClick={() => move(i, -1)} disabled={i === 0} aria-label={`${t('pat.moveUp')} — ${r.name || i + 1}`}>
                      <ArrowUp className="h-4 w-4" aria-hidden="true" />
                    </Button>
                    <Button size="sm" variant="ghost" onClick={() => move(i, 1)} disabled={i === rows.length - 1} aria-label={`${t('pat.moveDown')} — ${r.name || i + 1}`}>
                      <ArrowDown className="h-4 w-4" aria-hidden="true" />
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      className="text-red-700"
                      onClick={() => setRows((rs) => rs.filter((_, j) => j !== i))}
                      aria-label={`${t('pat.removeCp')} — ${r.name || i + 1}`}
                    >
                      <Trash2 className="h-4 w-4" aria-hidden="true" />
                    </Button>
                  </span>
                </div>
                <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-5">
                  <Field label={t('pat.cpName')} required className="lg:col-span-2">
                    <Input value={r.name} onChange={(e) => update(i, { name: e.target.value })} maxLength={100} />
                  </Field>
                  <Field label={t('pat.qrCode')} optional hint={r.qr_code ? undefined : t('pat.qrAuto')}>
                    <Input value={r.qr_code} onChange={(e) => update(i, { qr_code: e.target.value })} className="font-mono" maxLength={100} />
                  </Field>
                  <Field label={t('pat.sla')} optional hint={t('pat.slaHint')}>
                    <Input type="number" min={1} max={1440} value={r.sla} onChange={(e) => update(i, { sla: e.target.value })} />
                  </Field>
                  <div className="grid grid-cols-2 gap-2">
                    <Field label={t('pat.lat')} optional>
                      <Input type="number" step="any" min={-90} max={90} value={r.lat} onChange={(e) => update(i, { lat: e.target.value })} />
                    </Field>
                    <Field label={t('pat.lng')} optional>
                      <Input type="number" step="any" min={-180} max={180} value={r.lng} onChange={(e) => update(i, { lng: e.target.value })} />
                    </Field>
                  </div>
                </div>
              </li>
            ))}
          </ol>
          <Button variant="secondary" className="mt-3" onClick={add}>
            <Plus className="h-4 w-4" aria-hidden="true" />
            {t('pat.addCheckpoint')}
          </Button>
        </fieldset>
      </form>
    </Modal>
  )
}
