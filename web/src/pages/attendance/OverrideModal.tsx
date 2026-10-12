import { useState, type FormEvent } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ShieldCheck } from 'lucide-react'
import apiClient from '../../api/client'
import { Badge, StatusBadge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Field, FormError, Input, Textarea } from '../../components/ui/Form'
import { Modal } from '../../components/ui/Modal'
import { useToast } from '../../components/toast/context'
import { useI18n } from '../../i18n/context'
import { errorText } from '../../i18n/errors'
import { formatTime, isoToLocalInput, localDateOf, localInputToIso } from '../../lib/time'
import { Avatar } from '../guards/PeopleKit'
import { formatLongDay } from '../guards/peopleUtil'
import type { AttendanceStatus, DailyAttendanceRow, Site } from '../../types'

interface Props {
  site: Site
  row: DailyAttendanceRow
  onClose: () => void
}

const OPTIONS: { value: AttendanceStatus; ring: string; dot: string }[] = [
  { value: 'present', ring: 'peer-checked:border-emerald-500 peer-checked:bg-emerald-50 peer-checked:ring-emerald-500/20', dot: 'bg-emerald-500' },
  { value: 'late', ring: 'peer-checked:border-amber-500 peer-checked:bg-amber-50 peer-checked:ring-amber-500/20', dot: 'bg-amber-500' },
  { value: 'absent', ring: 'peer-checked:border-red-500 peer-checked:bg-red-50 peer-checked:ring-red-500/20', dot: 'bg-red-500' },
]

/** Admin override of a shift's attendance (reason required, audited). */
export function OverrideModal({ site, row, onClose }: Props) {
  const { t, lang } = useI18n()
  const toast = useToast()
  const qc = useQueryClient()
  const tz = site.timezone
  const [status, setStatus] = useState<AttendanceStatus>(row.status === 'pending' ? 'present' : row.status)
  const [reason, setReason] = useState('')
  const [clockIn, setClockIn] = useState(isoToLocalInput(row.clock_in_at, tz))
  const [clockOut, setClockOut] = useState(isoToLocalInput(row.clock_out_at, tz))
  const [reasonError, setReasonError] = useState('')

  const save = useMutation({
    mutationFn: () =>
      apiClient.put(`/attendance/shifts/${row.shift_id}/override`, {
        status,
        reason: reason.trim(),
        clock_in_at: status === 'absent' ? undefined : localInputToIso(clockIn, tz),
        clock_out_at: status === 'absent' ? undefined : localInputToIso(clockOut, tz),
      }),
    onSuccess: () => {
      toast.success(t('att.overrideDone'))
      qc.invalidateQueries({ queryKey: ['attendance-daily', site.id] })
      qc.invalidateQueries({ queryKey: ['attendance-review', site.id] })
      qc.invalidateQueries({ queryKey: ['dashboard', site.id] })
      onClose()
    },
  })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!reason.trim()) {
      setReasonError(t('ppl.att.reasonRequired'))
      return
    }
    setReasonError('')
    save.mutate()
  }

  return (
    <Modal
      open
      onClose={onClose}
      title={t('ppl.att.overrideTitle')}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form="override-form" loading={save.isPending}>
            {t('ppl.att.saveOverride')}
          </Button>
        </>
      }
    >
      <form id="override-form" onSubmit={submit} className="space-y-5" noValidate>
        {/* Which shift */}
        <div className="flex items-center gap-3 rounded-xl border border-gray-200 bg-gray-50/70 p-3">
          <Avatar name={row.guard_name} />
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-semibold text-gray-900">{row.guard_name}</p>
            <p className="text-xs text-gray-500">
              {formatLongDay(localDateOf(row.shift_start, tz), lang)} · {formatTime(row.shift_start, tz)}–{formatTime(row.shift_end, tz)}
            </p>
          </div>
          <div className="text-right">
            <p className="mb-0.5 text-[11px] font-medium uppercase tracking-wider text-gray-500">{t('ppl.att.currently')}</p>
            {row.status === 'pending' ? <Badge dot>{t('ppl.att.notIn')}</Badge> : <StatusBadge status={row.status} />}
          </div>
        </div>

        <div className="flex items-start gap-2.5 text-sm text-gray-600">
          <ShieldCheck className="mt-0.5 h-4 w-4 shrink-0 text-primary-600" aria-hidden="true" />
          <p>{t('ppl.att.overrideIntro')}</p>
        </div>

        <FormError message={save.error ? errorText(save.error, lang) : null} />

        <fieldset>
          <legend className="mb-1.5 block text-sm font-medium text-gray-700">
            {t('ppl.att.newStatus')}{' '}
            <span className="text-red-700" aria-hidden="true">
              *
            </span>
          </legend>
          <div className="grid grid-cols-3 gap-2">
            {OPTIONS.map((o) => (
              <label key={o.value} className="relative cursor-pointer">
                <input
                  type="radio"
                  name="override-status"
                  value={o.value}
                  checked={status === o.value}
                  onChange={() => setStatus(o.value)}
                  className="peer sr-only"
                />
                <span
                  className={`flex items-center justify-center gap-2 rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm font-medium text-gray-800 ring-4 ring-transparent transition-colors hover:bg-gray-50 peer-focus-visible:outline peer-focus-visible:outline-2 peer-focus-visible:outline-primary-500 ${o.ring}`}
                >
                  <span className={`h-2 w-2 rounded-full ${o.dot}`} aria-hidden="true" />
                  {t(`status.${o.value}`)}
                </span>
              </label>
            ))}
          </div>
        </fieldset>

        {status !== 'absent' && (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <Field label={t('att.clockIn')} optional hint={t('ppl.att.tzNote', { tz })}>
              <Input type="datetime-local" value={clockIn} onChange={(e) => setClockIn(e.target.value)} />
            </Field>
            <Field label={t('att.clockOut')} optional>
              <Input type="datetime-local" value={clockOut} onChange={(e) => setClockOut(e.target.value)} />
            </Field>
          </div>
        )}
        <Field label={t('common.reason')} required hint={t('att.overrideReasonHint')} error={reasonError}>
          <Textarea value={reason} onChange={(e) => setReason(e.target.value)} maxLength={500} placeholder={t('ppl.att.reasonPlaceholder')} />
        </Field>
      </form>
    </Modal>
  )
}
