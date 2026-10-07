import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import apiClient, { apiGet } from '../../api/client'
import { MediaPhotoButton } from '../../components/MediaPhoto'
import { Field, Select } from '../../components/ui/Form'
import { Toolbar } from '../../components/ui/States'
import { Table, type Column } from '../../components/ui/Table'
import { useErrorToast, useToast } from '../../components/toast/context'
import { useI18n } from '../../i18n/context'
import { formatDateTime } from '../../lib/time'
import type { DefectReport, Site, WorkflowStatus } from '../../types'

const STATUSES: WorkflowStatus[] = ['open', 'in_progress', 'resolved']

export function DefectsTab({ site }: { site: Site }) {
  const { t, lang } = useI18n()
  const tz = site.timezone
  const qc = useQueryClient()
  const toast = useToast()
  const onError = useErrorToast()
  const [status, setStatus] = useState('')

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['defects', site.id, status],
    queryFn: () => apiGet<DefectReport[]>('/defects', { site_id: site.id, status }),
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

  const columns: Column<DefectReport>[] = [
    { key: 'created', header: t('common.created'), cell: (r) => <span className="whitespace-nowrap">{formatDateTime(r.created_at, tz, lang)}</span> },
    { key: 'type', header: t('def.type'), cell: (r) => <span className="font-medium">{r.defect_type ?? '—'}</span> },
    { key: 'location', header: t('common.location'), cell: (r) => r.location ?? '—' },
    { key: 'desc', header: t('common.description'), cell: (r) => <span className="line-clamp-2 max-w-xs">{r.description ?? '—'}</span> },
    { key: 'guard', header: t('common.guard'), cell: (r) => (r.guard_name || '—') },
    { key: 'photo', header: t('common.photo'), cell: (r) => <MediaPhotoButton objectKey={r.photo_object_key} title={`${t('def.type')}: ${r.defect_type ?? ''}`} /> },
    {
      key: 'status',
      header: t('common.status'),
      cell: (r) => (
        <>
          <label htmlFor={`defect-${r.id}`} className="sr-only">
            {t('inc.changeStatus')} — {r.defect_type}
          </label>
          <Select
            id={`defect-${r.id}`}
            value={r.status}
            className="!w-36 !py-1"
            disabled={update.isPending && update.variables?.id === r.id}
            onChange={(e) => update.mutate({ id: r.id, s: e.target.value as WorkflowStatus })}
          >
            {STATUSES.map((s) => (
              <option key={s} value={s}>
                {t(`status.${s}`)}
              </option>
            ))}
          </Select>
        </>
      ),
    },
  ]

  return (
    <>
      <Toolbar>
        <Field label={t('common.status')}>
          <Select value={status} onChange={(e) => setStatus(e.target.value)}>
            <option value="">{t('common.all')}</option>
            {STATUSES.map((s) => (
              <option key={s} value={s}>
                {t(`status.${s}`)}
              </option>
            ))}
          </Select>
        </Field>
      </Toolbar>
      <Table columns={columns} rows={data} rowKey={(r) => r.id} isLoading={isLoading} error={error} onRetry={() => refetch()} empty={t('def.empty')} />
    </>
  )
}
