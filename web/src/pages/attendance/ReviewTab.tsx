import { useState } from 'react'
import { useMutation, useQueryClient, type UseQueryResult } from '@tanstack/react-query'
import { CheckCheck } from 'lucide-react'
import apiClient from '../../api/client'
import { StatusBadge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { MediaPhotoButton } from '../../components/MediaPhoto'
import { Field, FormError, Textarea } from '../../components/ui/Form'
import { Modal } from '../../components/ui/Modal'
import { Notice } from '../../components/ui/States'
import { Table, type Column } from '../../components/ui/Table'
import { useToast } from '../../components/toast/context'
import { useI18n } from '../../i18n/context'
import { errorText } from '../../i18n/errors'
import { formatDateTime } from '../../lib/time'
import type { AttendanceRecord, Site } from '../../types'

export function ReviewTab({ site, query }: { site: Site; query: UseQueryResult<AttendanceRecord[]> }) {
  const { t, lang } = useI18n()
  const toast = useToast()
  const qc = useQueryClient()
  const tz = site.timezone
  const [target, setTarget] = useState<AttendanceRecord | null>(null)
  const [note, setNote] = useState('')

  const mark = useMutation({
    mutationFn: (rec: AttendanceRecord) => apiClient.post(`/attendance/${rec.id}/review`, note.trim() ? { note: note.trim() } : {}),
    onSuccess: () => {
      toast.success(t('att.reviewedDone'))
      qc.invalidateQueries({ queryKey: ['attendance-review', site.id] })
      qc.invalidateQueries({ queryKey: ['attendance-daily', site.id] })
      qc.invalidateQueries({ queryKey: ['dashboard', site.id] })
      setTarget(null)
      setNote('')
    },
  })

  const columns: Column<AttendanceRecord>[] = [
    { key: 'guard', header: t('common.guard'), cell: (r) => <span className="font-medium">{(r.guard_name || '—')}</span> },
    { key: 'status', header: t('common.status'), cell: (r) => <StatusBadge status={r.status} /> },
    { key: 'in', header: t('att.clockIn'), cell: (r) => formatDateTime(r.clock_in_at, tz, lang) },
    { key: 'out', header: t('att.clockOut'), cell: (r) => formatDateTime(r.clock_out_at, tz, lang) },
    { key: 'late', header: t('att.lateMin'), numeric: true, cell: (r) => r.late_minutes || '—' },
    { key: 'early', header: t('att.earlyOutMin'), numeric: true, cell: (r) => r.early_out_minutes || '—' },
    {
      key: 'selfie',
      header: t('att.selfie'),
      cell: (r) => <MediaPhotoButton objectKey={r.selfie_object_key} title={`${t('att.selfie')}: ${r.guard_name ?? ''}`} />,
    },
    { key: 'created', header: t('att.recorded'), cell: (r) => formatDateTime(r.created_at, tz, lang) },
    {
      key: 'actions',
      header: <span className="sr-only">{t('common.actions')}</span>,
      cell: (r) => (
        <Button size="sm" variant="secondary" onClick={() => setTarget(r)}>
          <CheckCheck className="h-3.5 w-3.5" aria-hidden="true" />
          {t('att.markReviewed')}
        </Button>
      ),
    },
  ]

  return (
    <>
      <Notice>{t('att.reviewHint')}</Notice>
      <Table
        columns={columns}
        rows={query.data}
        rowKey={(r) => r.id}
        isLoading={query.isLoading}
        error={query.error}
        onRetry={() => query.refetch()}
        empty={t('att.reviewEmpty')}
      />
      {target && (
        <Modal
          open
          onClose={() => setTarget(null)}
          title={`${t('att.markReviewed')} — ${(target.guard_name || '—')}`}
          footer={
            <>
              <Button variant="secondary" onClick={() => setTarget(null)}>
                {t('common.cancel')}
              </Button>
              <Button loading={mark.isPending} onClick={() => mark.mutate(target)}>
                {t('att.markReviewed')}
              </Button>
            </>
          }
        >
          <div className="space-y-4">
            <FormError message={mark.error ? errorText(mark.error, lang) : null} />
            <Field label={t('att.reviewNote')} optional>
              <Textarea value={note} onChange={(e) => setNote(e.target.value)} maxLength={500} />
            </Field>
          </div>
        </Modal>
      )}
    </>
  )
}
