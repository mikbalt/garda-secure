import { useState, type FormEvent } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import apiClient from '../../api/client'
import { Button } from '../../components/ui/Button'
import { Field, FormError, Input, Select, Textarea } from '../../components/ui/Form'
import { Modal } from '../../components/ui/Modal'
import { useToast } from '../../components/toast/context'
import { useI18n } from '../../i18n/context'
import { errorText } from '../../i18n/errors'
import { isoToLocalInput, localInputToIso } from '../../lib/time'
import type { AttendanceStatus, DailyAttendanceRow, Site } from '../../types'

interface Props {
  site: Site
  row: DailyAttendanceRow
  onClose: () => void
}

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
      setReasonError(t('common.required'))
      return
    }
    setReasonError('')
    save.mutate()
  }

  return (
    <Modal
      open
      onClose={onClose}
      title={t('att.overrideTitle', { guard: row.guard_name })}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form="override-form" loading={save.isPending}>
            {t('common.save')}
          </Button>
        </>
      }
    >
      <form id="override-form" onSubmit={submit} className="space-y-4" noValidate>
        <FormError message={save.error ? errorText(save.error, lang) : null} />
        <Field label={t('common.status')} required>
          <Select value={status} onChange={(e) => setStatus(e.target.value as AttendanceStatus)}>
            <option value="present">{t('status.present')}</option>
            <option value="late">{t('status.late')}</option>
            <option value="absent">{t('status.absent')}</option>
          </Select>
        </Field>
        {status !== 'absent' && (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <Field label={t('att.clockIn')} optional hint={tz}>
              <Input type="datetime-local" value={clockIn} onChange={(e) => setClockIn(e.target.value)} />
            </Field>
            <Field label={t('att.clockOut')} optional hint={tz}>
              <Input type="datetime-local" value={clockOut} onChange={(e) => setClockOut(e.target.value)} />
            </Field>
          </div>
        )}
        <Field label={t('common.reason')} required hint={t('att.overrideReasonHint')} error={reasonError}>
          <Textarea value={reason} onChange={(e) => setReason(e.target.value)} maxLength={500} />
        </Field>
      </form>
    </Modal>
  )
}
