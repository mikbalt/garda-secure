import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CheckCircle2, Ban, MapPin } from 'lucide-react'
import apiClient, { apiGet } from '../../api/client'
import { StatusBadge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Field, FormError, Input, Select } from '../../components/ui/Form'
import { Modal } from '../../components/ui/Modal'
import { Notice } from '../../components/ui/States'
import { Table, type Column } from '../../components/ui/Table'
import { useErrorToast, useToast } from '../../components/toast/context'
import { useSite } from '../../context/site'
import { useI18n } from '../../i18n/context'
import { errorText } from '../../i18n/errors'
import { formatDateTime } from '../../lib/time'
import type { Device, DeviceStatus } from '../../types'

const ORDER: Record<DeviceStatus, number> = { pending: 0, active: 1, revoked: 2 }

function ApproveModal({ device, onClose }: { device: Device; onClose: () => void }) {
  const { t, lang } = useI18n()
  const { sites, siteId: currentSite } = useSite()
  const qc = useQueryClient()
  const toast = useToast()
  const [name, setName] = useState(device.name)
  const [siteId, setSiteId] = useState(device.site_id ?? currentSite)
  const save = useMutation({
    mutationFn: () => apiClient.put(`/devices/${device.id}`, { name: name.trim(), site_id: siteId, status: 'active' }),
    onSuccess: () => {
      toast.success(t('dev.approved'))
      qc.invalidateQueries({ queryKey: ['devices'] })
      onClose()
    },
  })
  return (
    <Modal
      open
      onClose={onClose}
      title={t('dev.approveTitle', { name: device.name })}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button onClick={() => save.mutate()} loading={save.isPending} disabled={!siteId}>
            {t('dev.approve')}
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <FormError message={save.error ? errorText(save.error, lang) : null} />
        <Field label={t('common.name')}>
          <Input value={name} onChange={(e) => setName(e.target.value)} maxLength={100} />
        </Field>
        <Field label={t('common.site')} required>
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

export function DevicesTab() {
  const { t, lang } = useI18n()
  const { sites, site } = useSite()
  const qc = useQueryClient()
  const toast = useToast()
  const onError = useErrorToast()
  const [approving, setApproving] = useState<Device | null>(null)
  const tz = site?.timezone ?? 'Asia/Jakarta'

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['devices'],
    queryFn: () => apiGet<Device[]>('/devices'),
    refetchInterval: 30_000,
  })

  const revoke = useMutation({
    mutationFn: (d: Device) => apiClient.put(`/devices/${d.id}`, { name: d.name, site_id: d.site_id ?? null, status: 'revoked' }),
    onSuccess: () => {
      toast.success(t('dev.revoked'))
      qc.invalidateQueries({ queryKey: ['devices'] })
    },
    onError,
  })

  const siteName = (id?: string) => (id ? sites.find((s) => s.id === id)?.name ?? '—' : '—')
  const rows = [...(data ?? [])].sort((a, b) => ORDER[a.status] - ORDER[b.status] || a.name.localeCompare(b.name))

  const columns: Column<Device>[] = [
    { key: 'name', header: t('common.name'), cell: (d) => <span className="font-medium">{d.name}</span> },
    { key: 'status', header: t('common.status'), cell: (d) => <StatusBadge status={d.status} /> },
    { key: 'site', header: t('common.site'), cell: (d) => siteName(d.site_id) },
    { key: 'seen', header: t('dev.lastSeen'), cell: (d) => formatDateTime(d.last_seen_at, tz, lang) },
    { key: 'created', header: t('common.created'), cell: (d) => formatDateTime(d.created_at, tz, lang) },
    {
      key: 'actions',
      header: <span className="sr-only">{t('common.actions')}</span>,
      cell: (d) => (
        <span className="flex gap-1">
          {d.status !== 'active' ? (
            <Button size="sm" onClick={() => setApproving(d)} aria-label={`${t('dev.approve')} — ${d.name}`}>
              <CheckCircle2 className="h-3.5 w-3.5" aria-hidden="true" />
              {t('dev.approve')}
            </Button>
          ) : (
            <Button size="sm" variant="secondary" onClick={() => setApproving(d)} aria-label={`${t('dev.reassign')} — ${d.name}`}>
              <MapPin className="h-3.5 w-3.5" aria-hidden="true" />
              {t('dev.reassign')}
            </Button>
          )}
          {d.status !== 'revoked' && (
            <Button
              size="sm"
              variant="danger"
              loading={revoke.isPending && revoke.variables?.id === d.id}
              onClick={() => window.confirm(t('dev.confirmRevoke')) && revoke.mutate(d)}
              aria-label={`${t('dev.revoke')} — ${d.name}`}
            >
              <Ban className="h-3.5 w-3.5" aria-hidden="true" />
              {t('dev.revoke')}
            </Button>
          )}
        </span>
      ),
    },
  ]

  return (
    <>
      <Notice>{t('dev.hint')}</Notice>
      <Table
        columns={columns}
        rows={rows}
        rowKey={(d) => d.id}
        isLoading={isLoading}
        error={error}
        onRetry={() => refetch()}
        empty={t('dev.empty')}
        rowClassName={(d) => (d.status === 'pending' ? 'bg-yellow-50' : '')}
      />
      {approving && <ApproveModal device={approving} onClose={() => setApproving(null)} />}
    </>
  )
}
