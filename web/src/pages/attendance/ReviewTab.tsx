import { useState } from 'react'
import { useMutation, useQueryClient, type UseQueryResult } from '@tanstack/react-query'
import { CalendarSearch, CheckCheck, CircleCheckBig, CloudOff } from 'lucide-react'
import apiClient from '../../api/client'
import { TableSkeleton } from '../../components/LoadingSkeleton'
import { FlagBadges } from '../../components/FlagBadges'
import { MediaPhotoButton } from '../../components/MediaPhoto'
import { Badge, StatusBadge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Field, FormError, Textarea } from '../../components/ui/Form'
import { Modal } from '../../components/ui/Modal'
import { EmptyState, ErrorState } from '../../components/ui/States'
import { Table, type Column } from '../../components/ui/Table'
import { useToast } from '../../components/toast/context'
import { useI18n } from '../../i18n/context'
import { errorText } from '../../i18n/errors'
import { formatDateTime, formatTime, localDateOf } from '../../lib/time'
import { Avatar, PersonCell } from '../guards/PeopleKit'
import { formatLongDay } from '../guards/peopleUtil'
import type { AttendanceRecord, Site } from '../../types'

interface Props {
  site: Site
  query: UseQueryResult<AttendanceRecord[]>
  /** Jump to the daily view of a date (to correct the record there). */
  onOpenDay: (date: string) => void
}

export function ReviewTab({ site, query, onOpenDay }: Props) {
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

  const close = () => {
    setTarget(null)
    setNote('')
    mark.reset()
  }

  const dayOf = (r: AttendanceRecord) => localDateOf(r.clock_in_at ?? r.created_at, tz)

  const columns: Column<AttendanceRecord>[] = [
    { key: 'guard', header: t('common.guard'), cell: (r) => <PersonCell name={r.guard_name || '—'} sub={formatLongDay(dayOf(r), lang)} /> },
    {
      key: 'status',
      header: t('ppl.att.recordedAs'),
      cell: (r) => (
        <span className="flex flex-wrap items-center gap-1.5">
          <StatusBadge status={r.status} />
          <FlagBadges flags={r.flags} />
        </span>
      ),
    },
    {
      key: 'in',
      header: t('att.clockIn'),
      cell: (r) => (
        <span className="inline-flex items-center gap-1.5 whitespace-nowrap tabular-nums">
          <span className="font-medium text-gray-900">{formatTime(r.clock_in_at, tz)}</span>
          {r.late_minutes > 0 && <Badge tone="yellow">{t('ppl.att.lateBy', { n: r.late_minutes })}</Badge>}
        </span>
      ),
    },
    { key: 'out', header: t('att.clockOut'), cell: (r) => <span className="whitespace-nowrap tabular-nums">{formatTime(r.clock_out_at, tz)}</span> },
    {
      key: 'received',
      header: t('ppl.att.received'),
      cell: (r) => <span className="whitespace-nowrap text-gray-600">{formatDateTime(r.received_at ?? r.created_at, tz, lang)}</span>,
    },
    {
      key: 'selfie',
      header: t('att.selfie'),
      className: '[&_span]:whitespace-nowrap',
      cell: (r) => <MediaPhotoButton objectKey={r.selfie_object_key} title={`${t('att.selfie')}: ${r.guard_name ?? ''}`} />,
    },
    {
      key: 'actions',
      header: <span className="sr-only">{t('common.actions')}</span>,
      className: 'text-right',
      cell: (r) => (
        <span className="flex justify-end gap-1.5">
          <Button
            size="sm"
            variant="ghost"
            onClick={() => onOpenDay(dayOf(r))}
            title={t('ppl.att.openDay')}
            aria-label={`${t('ppl.att.openDay')} — ${r.guard_name ?? ''}`}
          >
            <CalendarSearch className="h-4 w-4" aria-hidden="true" />
          </Button>
          <Button size="sm" variant="secondary" onClick={() => setTarget(r)} aria-label={`${t('att.markReviewed')} — ${r.guard_name ?? ''}`}>
            <CheckCheck className="h-3.5 w-3.5" aria-hidden="true" />
            {t('ppl.att.markReviewedShort')}
          </Button>
        </span>
      ),
    },
  ]

  let body
  if (query.isLoading) body = <TableSkeleton />
  else if (query.error) body = <ErrorState error={query.error} onRetry={() => query.refetch()} />
  else if (!query.data?.length) body = <EmptyState icon={CircleCheckBig} title={t('ppl.att.reviewEmptyTitle')} message={t('ppl.att.reviewEmptyBody')} />
  else
    body = (
      // `relative` + clip keeps screen-reader-only text inside the scrolling table from widening the page.
      <div className="relative -m-1 overflow-hidden p-1">
        <Table columns={columns} rows={query.data} rowKey={(r) => r.id} caption={t('ppl.att.tabReview')} />
      </div>
    )

  return (
    <>
      <div className="mb-4 flex items-start gap-3 rounded-2xl border border-violet-200 bg-violet-50/60 px-4 py-3.5 text-sm text-violet-950">
        <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-white text-violet-600 ring-1 ring-violet-200">
          <CloudOff className="h-4 w-4" aria-hidden="true" />
        </span>
        <div className="min-w-0">
          <p className="font-semibold">{t('ppl.att.reviewWhy')}</p>
          <p className="mt-0.5 text-violet-900">{t('ppl.att.reviewWhyBody')}</p>
          <p className="mt-1 text-violet-900">{t('flag.reviewWhy')}</p>
        </div>
      </div>
      {body}
      {target && (
        <Modal
          open
          onClose={close}
          title={t('att.markReviewed')}
          footer={
            <>
              <Button variant="secondary" onClick={close}>
                {t('common.cancel')}
              </Button>
              <Button loading={mark.isPending} onClick={() => mark.mutate(target)}>
                <CheckCheck className="h-4 w-4" aria-hidden="true" />
                {t('att.markReviewed')}
              </Button>
            </>
          }
        >
          <div className="space-y-4">
            <div className="flex items-center gap-3 rounded-xl border border-gray-200 bg-gray-50/70 p-3">
              <Avatar name={target.guard_name} />
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-semibold text-gray-900">{target.guard_name || '—'}</p>
                <p className="text-xs text-gray-500">
                  {t('att.clockIn')} {formatDateTime(target.clock_in_at, tz, lang)} · {t('ppl.att.received')}{' '}
                  {formatDateTime(target.received_at ?? target.created_at, tz, lang)}
                </p>
              </div>
              <StatusBadge status={target.status} />
            </div>
            <p className="text-sm text-gray-600">{t('ppl.att.reviewModalBody')}</p>
            <FormError message={mark.error ? errorText(mark.error, lang) : null} />
            <Field label={t('att.reviewNote')} optional>
              <Textarea value={note} onChange={(e) => setNote(e.target.value)} maxLength={500} placeholder={t('ppl.att.reviewNotePlaceholder')} />
            </Field>
          </div>
        </Modal>
      )}
    </>
  )
}
