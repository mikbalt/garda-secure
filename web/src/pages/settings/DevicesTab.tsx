import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Ban, CheckCircle2, Clock, MapPin, Pencil, RotateCcw, Tablet, TriangleAlert } from 'lucide-react'
import apiClient, { apiGet } from '../../api/client'
import { TableSkeleton } from '../../components/LoadingSkeleton'
import { Badge, Dot } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Field, FormError, Input, Select } from '../../components/ui/Form'
import { Modal } from '../../components/ui/Modal'
import { Card, EmptyState, ErrorState } from '../../components/ui/States'
import { useErrorToast, useToast } from '../../components/toast/context'
import { useSite } from '../../context/site'
import { useI18n } from '../../i18n/context'
import { errorText } from '../../i18n/errors'
import { formatDateTime } from '../../lib/time'
import { isOnline, relativeTime } from './format'
import { ConfirmDialog, IconTile, SectionHeader } from './kit'
import { DeviceAccounts } from './DeviceAccounts'
import { DeviceHealthDrawer, HealthBadge } from './DeviceHealthDrawer'
import type { Device, DeviceAccount } from '../../types'

const byName = (a: Device, b: Device) => a.name.localeCompare(b.name)

/** Rename / move an active tablet, or re-activate a revoked one. */
function DeviceModal({ device, onClose }: { device: Device; onClose: () => void }) {
  const { t, lang } = useI18n()
  const { sites, siteId: currentSite } = useSite()
  const qc = useQueryClient()
  const toast = useToast()
  const reactivate = device.status !== 'active'
  const [name, setName] = useState(device.name)
  const [siteId, setSiteId] = useState(device.site_id ?? currentSite)
  const save = useMutation({
    mutationFn: () => apiClient.put(`/devices/${device.id}`, { name: name.trim() || device.name, site_id: siteId, status: 'active' }),
    onSuccess: () => {
      toast.success(reactivate ? t('dev.approved') : t('common.saved'))
      qc.invalidateQueries({ queryKey: ['devices'] })
      onClose()
    },
  })
  return (
    <Modal
      open
      onClose={onClose}
      title={reactivate ? t('adm.dev.reactivateTitle', { name: device.name }) : t('adm.dev.editTitle', { name: device.name })}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button onClick={() => save.mutate()} loading={save.isPending} disabled={!siteId}>
            {reactivate ? t('adm.dev.reactivate') : t('common.save')}
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <FormError message={save.error ? errorText(save.error, lang) : null} />
        <Field label={t('adm.dev.name')} hint={t('adm.dev.nameHint')}>
          <Input value={name} onChange={(e) => setName(e.target.value)} maxLength={100} />
        </Field>
        <Field label={t('common.site')} required hint={t('adm.dev.siteHint')}>
          <Select value={siteId} onChange={(e) => setSiteId(e.target.value)}>
            <option value="">—</option>
            {sites.map((s) => (
              <option key={s.id} value={s.id}>
                {s.name}
              </option>
            ))}
          </Select>
        </Field>
      </div>
    </Modal>
  )
}

/** One pending tablet with inline site selection and approval. */
function PendingRow({ device, onReject }: { device: Device; onReject: () => void }) {
  const { t, lang } = useI18n()
  const { sites, siteId: currentSite, site } = useSite()
  const qc = useQueryClient()
  const toast = useToast()
  const onError = useErrorToast()
  const tz = site?.timezone ?? 'Asia/Jakarta'
  const [picked, setSiteId] = useState<string | null>(null)
  const siteId = picked ?? device.site_id ?? currentSite ?? ''
  const approve = useMutation({
    mutationFn: () => apiClient.put(`/devices/${device.id}`, { name: device.name, site_id: siteId, status: 'active' }),
    onSuccess: () => {
      toast.success(t('adm.dev.approvedAt', { name: device.name, site: sites.find((s) => s.id === siteId)?.name ?? '' }))
      qc.invalidateQueries({ queryKey: ['devices'] })
    },
    onError,
  })
  const selectId = `approve-site-${device.id}`
  return (
    <li id={`device-${device.id}`} className="flex scroll-mt-24 flex-wrap items-center gap-x-4 gap-y-3 px-4 py-3.5 sm:px-5">
      <div className="flex min-w-0 flex-1 items-center gap-3">
        <IconTile icon={Tablet} tone="amber" size="sm" />
        <div className="min-w-0">
          <p className="truncate text-sm font-semibold text-gray-900">{device.name}</p>
          <p className="text-xs text-amber-900" title={formatDateTime(device.created_at, tz, lang)}>
            {t('adm.dev.registered', { when: relativeTime(device.created_at, lang) })}
          </p>
        </div>
      </div>
      <div className="relative flex w-full flex-wrap items-center gap-2 sm:w-auto">
        <label htmlFor={selectId} className="sr-only">
          {t('adm.dev.siteFor', { name: device.name })}
        </label>
        <Select id={selectId} value={siteId} onChange={(e) => setSiteId(e.target.value)} className="min-w-0 flex-1 sm:w-52 sm:flex-none">
          <option value="">{t('adm.dev.chooseSite')}</option>
          {sites.map((s) => (
            <option key={s.id} value={s.id}>
              {s.name}
            </option>
          ))}
        </Select>
        <Button onClick={() => approve.mutate()} loading={approve.isPending} disabled={!siteId} aria-label={`${t('dev.approve')} — ${device.name}`}>
          <CheckCircle2 className="h-4 w-4" aria-hidden="true" />
          {t('dev.approve')}
        </Button>
        <Button variant="ghost" onClick={onReject} aria-label={`${t('adm.dev.reject')} — ${device.name}`} className="text-gray-600">
          {t('adm.dev.reject')}
        </Button>
      </div>
    </li>
  )
}

function HowTo() {
  const { t } = useI18n()
  const steps = [
    { title: t('adm.dev.how1'), body: t('adm.dev.how1Body') },
    { title: t('adm.dev.how2'), body: t('adm.dev.how2Body') },
    { title: t('adm.dev.how3'), body: t('adm.dev.how3Body') },
  ]
  return (
    <Card title={t('adm.dev.howTitle')} description={t('adm.dev.howDesc')}>
      <ol className="grid gap-4 sm:grid-cols-3">
        {steps.map((s, i) => (
          <li key={s.title} className="flex gap-3">
            <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-primary-600 text-xs font-semibold text-white">{i + 1}</span>
            <div className="min-w-0">
              <p className="text-sm font-semibold text-gray-900">{s.title}</p>
              <p className="mt-0.5 text-sm leading-relaxed text-gray-600">{s.body}</p>
            </div>
          </li>
        ))}
      </ol>
    </Card>
  )
}

export function DevicesTab() {
  const { t, lang } = useI18n()
  const { sites, site } = useSite()
  const tz = site?.timezone ?? 'Asia/Jakarta'
  const qc = useQueryClient()
  const toast = useToast()
  const onError = useErrorToast()
  const [editing, setEditing] = useState<Device | null>(null)
  const [revoking, setRevoking] = useState<Device | null>(null)
  const [healthOf, setHealthOf] = useState<Device | null>(null)

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['devices'],
    queryFn: () => apiGet<Device[]>('/devices'),
    refetchInterval: 30_000,
  })
  // Tablet accounts (shared cache with the accounts card) to name the account in the health drawer.
  const accounts = useQuery({ queryKey: ['device-accounts'], queryFn: () => apiGet<DeviceAccount[]>('/device-accounts') })

  const revoke = useMutation({
    mutationFn: (d: Device) => apiClient.put(`/devices/${d.id}`, { name: d.name, site_id: d.site_id ?? null, status: 'revoked' }),
    onSuccess: (_r, d) => {
      toast.success(d.status === 'pending' ? t('adm.dev.rejected') : t('dev.revoked'))
      qc.invalidateQueries({ queryKey: ['devices'] })
      setRevoking(null)
    },
    onError,
  })

  const siteName = (id?: string) => (id ? sites.find((s) => s.id === id)?.name ?? '—' : '—')
  const all = data ?? []
  const pending = all.filter((d) => d.status === 'pending').sort(byName)
  const active = all.filter((d) => d.status === 'active').sort((a, b) => siteName(a.site_id).localeCompare(siteName(b.site_id)) || byName(a, b))
  const revoked = all.filter((d) => d.status === 'revoked').sort(byName)
  const now = Date.now()

  return (
    <>
      <SectionHeader title={t('adm.set.devices')} description={t('adm.dev.intro')} />

      {isLoading ? (
        <TableSkeleton />
      ) : error ? (
        <ErrorState error={error} onRetry={() => refetch()} />
      ) : all.length === 0 ? (
        <div className="space-y-6">
          <EmptyState icon={Tablet} title={t('adm.dev.emptyTitle')} message={t('adm.dev.empty')} />
          <HowTo />
          <DeviceAccounts />
        </div>
      ) : (
        <div className="space-y-6">
          {pending.length > 0 && (
            <section aria-labelledby="pending-devices" className="overflow-hidden rounded-2xl border border-amber-200 bg-amber-50/60 shadow-card">
              <div className="flex items-start gap-3 border-b border-amber-200/70 px-4 py-3.5 sm:px-5">
                <TriangleAlert className="mt-0.5 h-5 w-5 shrink-0 text-amber-600" aria-hidden="true" />
                <div className="min-w-0">
                  <h3 id="pending-devices" className="text-[15px] font-semibold text-amber-950">
                    {t('adm.dev.pendingTitle', { n: pending.length })}
                  </h3>
                  <p className="text-sm text-amber-900/80">{t('adm.dev.pendingHint')}</p>
                </div>
              </div>
              <ul className="divide-y divide-amber-200/60 bg-white/60">
                {pending.map((d) => (
                  <PendingRow key={d.id} device={d} onReject={() => setRevoking(d)} />
                ))}
              </ul>
            </section>
          )}

          <Card title={t('adm.dev.activeTitle', { n: active.length })} description={t('adm.dev.activeDesc')} flush>
            {active.length === 0 ? (
              <p className="px-5 py-6 text-sm text-gray-500">{t('adm.dev.noActive')}</p>
            ) : (
              <ul className="divide-y divide-gray-100">
                {active.map((d) => {
                  const online = isOnline(d.last_seen_at, now)
                  return (
                    <li key={d.id} id={`device-${d.id}`} className="flex scroll-mt-24 flex-wrap items-center gap-x-4 gap-y-2 px-4 py-3 sm:px-5">
                      <div className="flex min-w-0 flex-1 items-center gap-3">
                        <IconTile icon={Tablet} tone={online ? 'green' : 'gray'} size="sm" />
                        <div className="min-w-0">
                          <p className="truncate text-sm font-medium text-gray-900">{d.name}</p>
                          <p className="flex flex-wrap items-center gap-x-3 gap-y-0.5 text-xs text-gray-500">
                            <span className="inline-flex items-center gap-1">
                              <MapPin className="h-3 w-3" aria-hidden="true" />
                              {siteName(d.site_id)}
                            </span>
                            <span className="inline-flex items-center gap-1" title={formatDateTime(d.last_seen_at, tz, lang)}>
                              <Clock className="h-3 w-3" aria-hidden="true" />
                              {d.last_seen_at ? t('adm.dev.lastSeen', { when: relativeTime(d.last_seen_at, lang, now) }) : t('adm.dev.neverSeen')}
                            </span>
                          </p>
                        </div>
                      </div>
                      <div className="flex flex-wrap items-center gap-2">
                        <HealthBadge device={d} onOpen={() => setHealthOf(d)} now={now} />
                        {online ? (
                          <Badge tone="green">
                            <Dot tone="green" pulse />
                            {t('adm.dev.online')}
                          </Badge>
                        ) : (
                          <Badge tone="gray" dot>
                            {t('adm.dev.offline')}
                          </Badge>
                        )}
                        <Button size="sm" variant="secondary" onClick={() => setEditing(d)} aria-label={`${t('common.edit')} — ${d.name}`}>
                          <Pencil className="h-3.5 w-3.5" aria-hidden="true" />
                          <span className="hidden sm:inline">{t('common.edit')}</span>
                        </Button>
                        <Button size="sm" variant="ghost" className="text-red-700 hover:bg-red-50 hover:text-red-800" onClick={() => setRevoking(d)} aria-label={`${t('dev.revoke')} — ${d.name}`}>
                          <Ban className="h-3.5 w-3.5" aria-hidden="true" />
                          <span className="hidden sm:inline">{t('dev.revoke')}</span>
                        </Button>
                      </div>
                    </li>
                  )
                })}
              </ul>
            )}
          </Card>

          {revoked.length > 0 && (
            <Card title={t('adm.dev.revokedTitle', { n: revoked.length })} description={t('adm.dev.revokedDesc')} flush>
              <ul className="divide-y divide-gray-100">
                {revoked.map((d) => (
                  <li key={d.id} id={`device-${d.id}`} className="flex scroll-mt-24 flex-wrap items-center gap-x-4 gap-y-2 px-4 py-3 sm:px-5">
                    <div className="flex min-w-0 flex-1 items-center gap-3 opacity-70">
                      <IconTile icon={Tablet} tone="gray" size="sm" />
                      <div className="min-w-0">
                        <p className="truncate text-sm font-medium text-gray-700 line-through decoration-gray-300">{d.name}</p>
                        <p className="text-xs text-gray-500">{d.last_seen_at ? t('adm.dev.lastSeen', { when: relativeTime(d.last_seen_at, lang, now) }) : t('adm.dev.neverSeen')}</p>
                      </div>
                    </div>
                    <Button size="sm" variant="ghost" onClick={() => setEditing(d)} aria-label={`${t('adm.dev.reactivate')} — ${d.name}`}>
                      <RotateCcw className="h-3.5 w-3.5" aria-hidden="true" />
                      {t('adm.dev.reactivate')}
                    </Button>
                  </li>
                ))}
              </ul>
            </Card>
          )}

          <DeviceAccounts />
          <HowTo />
        </div>
      )}

      {healthOf && (
        <DeviceHealthDrawer
          device={healthOf}
          siteName={siteName(healthOf.site_id)}
          tz={tz}
          account={(accounts.data ?? []).find((a) => a.id === healthOf.keycloak_user_id)}
          onClose={() => setHealthOf(null)}
        />
      )}
      {editing && <DeviceModal device={editing} onClose={() => setEditing(null)} />}
      <ConfirmDialog
        open={!!revoking}
        title={revoking?.status === 'pending' ? t('adm.dev.rejectTitle', { name: revoking?.name ?? '' }) : t('adm.dev.revokeTitle', { name: revoking?.name ?? '' })}
        message={revoking?.status === 'pending' ? t('adm.dev.rejectBody') : t('adm.dev.revokeBody')}
        confirmLabel={revoking?.status === 'pending' ? t('adm.dev.reject') : t('dev.revoke')}
        loading={revoke.isPending}
        onConfirm={() => revoking && revoke.mutate(revoking)}
        onClose={() => setRevoking(null)}
      />
    </>
  )
}
